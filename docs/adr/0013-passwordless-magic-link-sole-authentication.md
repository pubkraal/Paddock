# ADR-0013: Passwordless Magic-Link as the Sole Authentication Mechanism

**Status**: Accepted
**Date**: 2026-06-17

## Context

Paddock's governing constraint is *low barrier to entry and speed is everything* (PLAN.md): a press officer with no training must run their first race weekend the same day they sign up, and accredited consumers (journalists, sponsors, teams) must reach their media without installing an app or managing a credential.

Two source-of-truth documents appeared to disagree on how admins authenticate. PLAN.md §6 lists "session auth (Redis-backed sessions)" for admins and "magic-link … (no password)" specifically for consumers — readable as password-based admin login. The design handoff's login screen says, for press officers and org staff, "No password — we send a magic link." BRIEF §8 lists SSO/SCIM as explicitly out of MVP. The contradiction needed a decision before the identity package could be designed.

## Decision

**Magic-link is the sole authentication mechanism for everyone — admins and consumers alike. No passwords, no SSO in MVP.** Everyone signs in by entering their work email and clicking an emailed, single-use, time-limited link.

What differs is what redemption *grants*:

- An **admin** (`press_officer` / `season_admin` / `finance`) redeems to a durable, org-scoped **session** carrying their role. Subsequent requests are authenticated by an opaque session cookie and scoped via `WithOrg`.
- A **consumer** redeems to an event-scoped, limited **session** (kind `consumer`, carrying an opaque scope). The grant is single-use at the link level; the resulting session lasts the access window.

Supporting decisions recorded here:

- **Sessions live only in Redis.** The "sessions metadata" PLAN.md refers to is the JSON value stored in Redis (`{user_id, org_id, role, kind, scope}`), keyed by an opaque session id, TTL-driven. There is no Postgres `sessions` table. Such a table would itself straddle the pre-scope boundary (you need the session to know the org, but the table would be tenant-scoped), and Redis is the designated ephemeral store for sessions and tokens (ADR-0005). If durable session audit is ever required, it is an additive Phase-2+ table, not a Phase-1 dependency.
- **Magic-link email is sent synchronously** in Phase 1. Login is a single, low-volume transactional email; the request renders and sends it inline via the `Mailer` interface (ADR-0007). Bulk, worker-driven magic-link email (120 accreditation invites at once) arrives in Phase 2 over River. The `LinkMailer` bounds the send with a short timeout (a `select` over the send and the deadline), so a slow or stalled SMTP hop cannot widen the response — on timeout the send may complete in the background while the caller's response returns. The primary anti-enumeration defence remains the byte-identical response body (proven in tests); the timeout bounds worst-case latency. A residual *typical-case* timing difference remains — the existing-user path also issues a token and dispatches a send — and is eliminated only when email moves fully off the request path (async send, Phase 2).
- **Tokens are single-use and never stored raw.** The opaque token (≥256-bit) is stored in Redis under `magiclink:<sha256(token)>`; redemption is atomic via `GETDEL`, so a replayed link fails closed. Expiry is the Redis TTL.

## Alternatives Considered

### Email + password for admins, magic-link for consumers (the literal PLAN.md reading)

**Pros:**
- Familiar admin login; no inbox round-trip on every sign-in.
- Matches one literal reading of PLAN.md.

**Cons:**
- Adds a credential-storage surface (argon2id hashing), a password-reset flow, and the associated phishing/breach considerations — for the exact user class the design says should not have a password.
- Diverges from the design handoff's passwordless login screen.
- More code and more security surface against the "speed is everything" goal.

**Why rejected**: it spends complexity to *remove* the lowest-friction path the product explicitly wants. The design handoff is the source of truth for interaction design, and passwordless-for-all is both less code and a better fit for same-day onboarding.

### SSO (Microsoft Entra / Google / Okta), shown in the design board

**Pros:**
- Enterprise-friendly; the 31-screen board includes the buttons.

**Cons:**
- Explicitly out of MVP (BRIEF §8). Rendering non-functional buttons is dead UI.

**Why rejected**: out of scope; the buttons are omitted from the MVP login screen rather than shown disabled.

## Consequences

### Positive

- One authentication path to build, test, and reason about; the front door is an email field for every user class.
- No password storage, no reset flow, no SSO integration in MVP — the smallest credible auth surface.
- Single-use + TTL tokens and Redis-only sessions keep the durable database free of auth state.

### Negative

- Every sign-in requires inbox access; an admin without their email cannot log in. Acceptable for the target users and offset by re-send.
- Deliverability of the magic-link email becomes a first-class operational concern (ADR-0007 / open decision O2 on the production provider).
- Synchronous send couples login latency to SMTP; mitigated by a short timeout and treating a timeout as a logged infra error while still returning the standard confirmation.
- **Late-disable window:** `Redeem` builds the session from the token payload without re-reading the user's current status, so a user disabled *after* a link is issued can still redeem it until the token's TTL elapses (default 15 minutes). This is an accepted Phase-1 tradeoff — disabling is rare and admin-initiated, the window is short, and re-checking status on redeem would add a DB round-trip to the hot path. Phase 2 closes it (status re-check at redeem and/or active-session revocation when a user is disabled).

### Neutral

- The admin-session vs consumer-grant distinction is carried in the session `kind`/`scope`; the consumer scope is opaque in Phase 1 (event domain lands in Phase 2).
