# ADR-0007: Mailer Interface with One Pluggable EU Default Provider

**Status**: Accepted
**Date**: 2026-06-16

## Context

Paddock sends transactional email at several points in the user journey:

- Magic links for accredited media access (no-password journalist and sponsor login)
- Embargo-lift notifications (media tier: "your content is now available")
- Share-link delivery (expiring gallery links sent to named recipients)
- Licence receipts (confirmation of download under a specific licence version)
- Admin notifications (accreditation bulk-import results, billing events)

Email is not a primary user-facing feature; it is infrastructure. However, the choice of how it is wired into the application affects testability, vendor flexibility, and EU data-residency compliance (transactional email content may carry personal data — recipient name, asset descriptions).

## Decision

We will define a narrow `Mailer` interface in `internal/platform` and ship:

1. One EU-friendly production adapter for MVP — either a reputable SMTP relay with EU infrastructure or Postmark with an EU data-residency region. The specific provider is selected at deployment time via environment configuration.
2. A no-op/log adapter (`LogMailer`) that records email sends to the application log without transmitting anything, used in development and unit tests.
3. A **Mailpit** container in the local docker-compose stack and CI, providing a real SMTP sink with a web UI for inspecting outbound email during development.

The interface will be narrow — the minimum surface needed for the use cases above:

```go
type Mailer interface {
    Send(ctx context.Context, msg Message) error
}
```

`Message` carries recipient, subject, and both plain-text and HTML bodies; template rendering is the caller's responsibility.

## Alternatives Considered

### Call a vendor SDK directly at each send site

**Pros:**
- Minimal abstraction; straightforward to understand at the call site.
- No interface boilerplate.

**Cons:**
- Couples every send site to one specific vendor's SDK and its method signatures. Changing providers requires finding and updating every call site.
- Unit-testing email behavior requires either a live email account or mocking a vendor-specific SDK — both are fragile and slow.
- If the vendor is not EU-resident, personal data in email content may leave the EU on every send. Provider migration requires application code changes, not only configuration changes.

**Why rejected**: Directly contradicts the house style of accepting interfaces and injecting dependencies. Hard to unit-test and hard to migrate.

### Use only the standard library `net/smtp` package directly

**Pros:**
- No third-party dependency for the sending layer.
- `net/smtp` is stable and well-understood.

**Cons:**
- `net/smtp` has no built-in delivery features: no retry on transient failure, no bounce handling, no delivery tracking.
- Connecting directly to port 25 from a cloud host is widely blocked by cloud providers to prevent spam; a relay is required in practice.
- The code still ends up wrapped in a struct to be injected; using `net/smtp` directly at call sites has the same coupling problem as using a vendor SDK directly.

**Why rejected**: `net/smtp` alone is insufficient for production use without building delivery features that a transactional mail relay already provides. And it still needs to be wrapped to be testable, so the interface pattern remains necessary regardless.

## Consequences

### Positive

- Call sites depend only on the `Mailer` interface; the production adapter, the log adapter, and the Mailpit-in-CI adapter are interchangeable without touching call sites.
- Unit tests inject `LogMailer` or a hand-written mock that captures sent messages; tests are fast, offline, and deterministic.
- The EU data-residency posture is enforced by provider selection (environment configuration), not by application code. Swapping to a different EU-resident provider requires only a new adapter implementation and a configuration change, with no call-site changes.
- Mailpit in the docker-compose stack gives engineers a real SMTP sink with a web UI for inspecting magic-link and embargo-notification emails during local development.

### Negative

- Adding a new email use case requires populating a `Message` struct and calling `Send`; HTML template rendering must be done by the caller before calling `Send`. This is the correct layering but adds a step compared to a higher-level "send magic link" method. A thin service layer on top of `Mailer` (e.g., `AuthMailer`, `EmbargoMailer`) should be added per domain when the template rendering logic is non-trivial.
- The specific EU production provider is not fixed in the codebase; engineers must consult deployment documentation to know which adapter is active. This is intentional but requires discipline in keeping configuration documentation current.

### Neutral

- The production adapter for MVP is a thin wrapper around an SMTP or HTTP API call. Future adapters (e.g., a different provider, or a stub for acceptance testing) are new structs implementing `Mailer`; no existing code changes.
