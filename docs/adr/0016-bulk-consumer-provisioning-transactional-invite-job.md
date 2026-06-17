# ADR-0016: Bulk Consumer Provisioning via a Transactional River Invite Job

**Status**: Accepted
**Date**: 2026-06-17

## Context

Importing a 120-person accreditation roster (Phase 2) must do two things per valid row: create a `consumer` user account scoped to the organization, and get that person an emailed magic-link invite so they can reach their tier-appropriate portal. The acceptance demo enqueues 120 invites from inside the setup wizard without the operator leaving the page (PLAN §6).

Two forces shape the design:

1. **Email is slow and external.** Sending 120 emails synchronously inside the HTTP request would block the wizard for many seconds and couple the import's success to the SMTP provider's availability. ADR-0005 already establishes River (Postgres-backed) as the async job queue and Phase 0 booted a no-op worker; this is the first real job type.
2. **An invite must never outlive — or precede — its row.** If we insert the accreditation/user rows and the transaction later rolls back, any already-sent invite is an orphan pointing at a user that does not exist. Conversely, if we send after commit via a separate channel, a crash between commit and send loses the invite. ADR-0009's transactional-enqueue guarantee (enqueue the job inside the very `*sql.Tx` that writes the rows) closes both gaps.

## Decision

Accreditation import provisions and invites through a **transactionally-enqueued River job**.

For each valid roster row, inside the same `WithOrg` transaction that writes the accreditation and upserts the consumer user, enqueue one job:

```go
type AccreditationInviteArgs struct {
    UserID string
    OrgID  string
    Email  string
}

func (AccreditationInviteArgs) Kind() string { return "accreditation_invite" }
```

The job is inserted via the River `*sql.Tx` insert client (`InsertTx`) using the same transaction handle as the row writes, so the job becomes visible to workers if and only if the row write commits — no orphaned invites, no invites for rolled-back rows.

The worker (`InviteWorker`, registered in the `cmd/worker` registry alongside the Phase 0 no-op) handles the job by issuing a **consumer-grant** magic-link token (the same Redis-backed, single-use, opaque token from ADR-0013, with `consumer_grant` purpose), building the link from the configured `Auth.BaseURL`, and sending it through the `Mailer` (ADR-0007). The worker is constructed with injected token-issuer and link-sender interfaces, so it is unit-tested with mocks (no Redis, no SMTP); send/issue failures return an error so River retries with backoff.

Because the worker now sends mail, **`cmd/worker` gains Redis, Mailer, and Auth configuration** — the first time the worker tier touches those subsystems.

**Idempotency.** Re-running an import (a common operator action after fixing a few rows) must not double-provision or double-invite. The consumer upsert is `INSERT … ON CONFLICT (email) DO NOTHING` against the globally-unique `users.email`, and accreditations are `UNIQUE (event_id, email)`. A row that already exists produces no new job, so re-import is safe.

## Alternatives Considered

### Send emails synchronously during the import request

**Pros:**
- No job type, no worker involvement; simplest control flow.

**Cons:**
- Blocks the wizard for the duration of 120 SMTP round-trips and fails the whole import if the mail provider hiccups; partial sends on timeout are ambiguous to recover.

**Why rejected**: violates the responsive same-day-onboarding UX and couples a DB write to an external service's uptime.

### Enqueue after commit (application-level, outside the transaction)

**Pros:**
- Conceptually simple; no shared transaction handle.

**Cons:**
- A crash between `COMMIT` and enqueue silently drops invites; a rollback after a pre-emptive enqueue sends invites for rows that never persisted. This is exactly the dual-write problem ADR-0009 exists to avoid.

**Why rejected**: only transactional enqueue gives exactly-once-relative-to-the-write semantics.

## Consequences

### Positive

- The import returns immediately; 120 invites drain asynchronously and survive worker restarts (jobs are durable Postgres rows).
- No orphaned or lost invites: the job and the rows commit atomically.
- Re-import is idempotent; operators can safely re-run after corrections.
- River gains its first real job type and worker, exercising the transactional-enqueue path that Phase 0 only constructed.

### Negative

- The worker tier's configuration and dependency surface grows (Redis + Mailer + Auth); it is no longer a pure Postgres consumer.
- Invite delivery is eventually-consistent: a row is committed before its email is sent. This is correct for invitations (the magic link is issued at send time, with its own TTL) but means "imported" ≠ "emailed" instantaneously.

### Neutral

- The consumer-grant token is issued by the worker at send time (not at import time), so the invite's TTL starts when the email goes out, consistent with ADR-0013.
