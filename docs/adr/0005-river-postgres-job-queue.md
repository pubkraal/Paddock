# ADR-0005: River (Postgres-Backed) for the Async Job Queue; Redis Is Ephemeral-Only

**Status**: Accepted
**Date**: 2026-06-16

## Context

Paddock's worker tier must reliably execute a wide variety of asynchronous jobs: ingest processing (IPTC parse, duplicate detection, rendition scheduling), thumbnail and preview generation, watermark compositing, embargo lifts at scheduled times, ZIP bundle assembly, and email sends. These jobs are enqueued by all three binaries (`cmd/web`, `cmd/worker` itself for chained jobs, and `cmd/ftp-gateway`).

A media ingest pipeline has a specific consistency requirement that generic queues can miss: when a file arrives and is written to the database (an asset row is created), a processing job must be enqueued for that file. If the job is lost — due to a crash between the DB write and the enqueue, or a queue-side failure — the asset sits in the catalog forever unprocessed. Detecting and recovering orphaned assets requires a reconciliation process that adds complexity and latency.

The platform already requires Postgres (for the domain model, multitenancy via RLS, and River itself) and Redis (for session storage, magic-link tokens, and rate limits). The question is which of those two is the right substrate for the job queue.

## Decision

We will use **River** (`riverqueue.com/river`) as the async job queue, backed by Postgres. Jobs are enqueued in the **same database transaction** as the asset row or state change that triggered them. River handles retries, backoff, scheduling (embargo lifts at a future time), and dead-letter queuing.

**Redis is used only for ephemeral, non-durable state**: HTTP sessions, magic-link tokens (short TTL), and rate-limit counters. Redis durability (AOF/RDB) will not be required or relied upon; the platform must tolerate a full Redis restart with no job or data loss.

## Alternatives Considered

### asynq (Redis-backed queue)

**Pros:**
- Simple API; widely used in Go projects.
- Redis is already present in the stack for sessions and rate limiting.
- Good tooling (asynq web UI, inspector).

**Cons:**
- Enqueue is a separate Redis operation, not atomic with the Postgres DB write. A process crash between `INSERT asset` and `ENQUEUE job` produces an orphaned asset row with no processing job. Detection requires a reconciliation scan; recovery requires manual or automated re-enqueue logic.
- Relying on asynq for durability makes Redis a durability-critical dependency; AOF persistence, backup, and Redis Sentinel/Cluster become operational requirements, adding significant complexity to the infrastructure.
- At-least-once delivery requires idempotent handlers regardless of queue choice, but the orphan-asset problem specifically requires the enqueue to be atomic with the DB change — something Redis cannot provide.

**Why rejected**: The non-transactional enqueue is a structural weakness for an ingest pipeline where orphaned assets are a product correctness failure. Making Redis durability-critical when it is already in the stack as an ephemeral store adds unnecessary operational burden.

### Hand-rolled SKIP LOCKED queue (Postgres)

**Pros:**
- Uses Postgres already in the stack; no new dependency.
- `SELECT ... FOR UPDATE SKIP LOCKED` is a well-understood pattern for Postgres-backed queues.
- Zero external library risk.

**Cons:**
- Requires implementing retry logic with exponential backoff from scratch.
- Requires implementing scheduled jobs (future embargo lifts) from scratch, typically via a separate polling loop.
- Requires implementing dead-letter queuing (permanently failed jobs that need human inspection) from scratch.
- Requires implementing job uniqueness/deduplication, worker heartbeats, and stuck-job detection from scratch.
- All of this is solved infrastructure that River already provides; building it again is not a business differentiator.

**Why rejected**: River provides exactly the retry, backoff, scheduling, dead-letter, and observability features that a hand-rolled queue would need to implement. Rebuilding them without tests covering edge cases (network partitions, clock skew in scheduled jobs) is high-risk reinvention.

## Consequences

### Positive

- Transactional enqueue eliminates the orphaned-asset class of bug entirely. If the transaction commits, the job exists. If the transaction rolls back, neither the asset row nor the job exists.
- River's scheduled-job support (enqueue a job to run at a future timestamp) is the natural implementation for embargo lifts: `InsertTx(ctx, tx, EmbargoLiftArgs{...}, &river.InsertOpts{ScheduledAt: &embargoTime})`.
- Redis does not need to be durable; it can be treated as a lossy cache, simplifying the infrastructure (no AOF, no Sentinel required in MVP).
- River's dead-letter queue surfaces permanently failed jobs in a queryable Postgres table; operators can inspect, retry, or discard them via SQL or a UI without a separate tool.
- One fewer stateful, durability-critical piece of infrastructure: Postgres is already the single source of truth.

### Negative

- River is a newer library with a smaller community than asynq or Sidekiq-style queues. API stability and long-term maintenance must be monitored.
- All three binaries (`cmd/web`, `cmd/worker`, `cmd/ftp-gateway`) must have access to the Postgres connection in order to enqueue jobs transactionally. This is already required for the domain model; it is not a new dependency, but it does mean the FTP gateway must hold a Postgres connection.
- High job throughput at burst (thousands of ingest jobs per hour at Y1 targets) exercises Postgres more than a Redis-backed queue would; table bloat and VACUUM behavior on the River job tables must be monitored.

### Neutral

- River workers are Go functions registered in `cmd/worker`; they use the same dependency injection and interface patterns as the rest of the codebase and are tested with the same Go test tooling.
