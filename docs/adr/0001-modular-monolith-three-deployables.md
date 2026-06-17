# ADR-0001: Modular Monolith, Three Deployables Sharing One Codebase

**Status**: Accepted
**Date**: 2026-06-16

## Context

Paddock's workload splits cleanly into two very different demand profiles:

- **Serving** — HTTP requests from press officers, journalists, and sponsors; must respond in milliseconds and remain available at 99.95% during declared event windows.
- **Media processing** — ingest of raw camera files, thumbnail/preview/rendition generation, watermark compositing, embargo-lift automation, ZIP builds, email sends; CPU- and I/O-heavy, can spike to thousands of jobs per hour at session end.

Running these profiles in the same OS process means a burst of ingest jobs can exhaust the thread/goroutine pool and degrade web latency exactly when the product must be most reliable. A third, separate concern is camera FTP/SFTP ingest: it binds privileged ports, has its own authentication model (per-photographer credentials), and needs to stay up independently of the job-processing tier.

The team is small and the product must be demonstrably usable in a pilot on day one. Architecture complexity must not delay the first working event.

## Decision

We will build one Go module (`github.com/pubkraal/paddock`) with internal bounded-context packages:

- `internal/ingest` — file receipt, duplicate detection, IPTC/XMP parsing
- `internal/catalog` — asset, gallery, session, event, entry-list domain
- `internal/rights` — entitlement tiers, embargo rules, licence-event log
- `internal/delivery` — presigned-URL generation, ZIP builds, share links
- `internal/platform` — auth, multitenancy, billing, email, background job definitions

Three entrypoint binaries are compiled from this single module:

| Binary | Responsibility |
|---|---|
| `cmd/web` | HTTP server: press-officer admin, branded media portals, API |
| `cmd/worker` | Job queue consumer: ingest processing, rendition generation, watermarking, embargo lifts, ZIP builds, email sends |
| `cmd/ftp-gateway` | Camera FTP/SFTP server: per-photographer authentication, stream-to-bucket, job enqueue |

All three share domain types, business logic, and the database schema via the internal packages. Each is deployed and scaled independently.

## Alternatives Considered

### Single binary (all three roles in one process)

**Pros:**
- Simplest deployment; one artifact to build and ship.
- No inter-process coordination.

**Cons:**
- Media processing goroutines compete directly with HTTP handler goroutines for the Go scheduler and system resources.
- A burst of 1,000 ingest jobs (the MVP burst target) arriving at session end degrades web response times at exactly the moment the product must perform.
- Cannot scale the processing tier without also scaling (and paying for) more web capacity.

**Why rejected**: Resource isolation between the serving path and the media-processing path is a hard non-functional requirement. Sharing a process makes that isolation impossible to enforce in Go's cooperative scheduler.

### Microservices (one repo per domain, separate deployments)

**Pros:**
- True blast-radius isolation per domain.
- Independent technology choices per service.

**Cons:**
- Requires distributed tracing, service discovery, and inter-service contracts from day one.
- Domain types (asset, event, session, entitlement) must be serialized/versioned across service boundaries — a significant ongoing cost.
- Contradicts the "live in an afternoon" positioning: a new engineer must understand distributed systems tooling before writing domain code.
- Testing cross-service behaviour requires either heavy integration infrastructure or extensive mocking that obscures real behaviour.

**Why rejected**: The operational and cognitive overhead of microservices is disproportionate at this stage and team size. The bounded-context package structure inside a monorepo gives us the domain isolation we need without the distribution tax.

## Consequences

### Positive

- Media processing is fully asynchronous on the worker tier, which can be scaled horizontally (more worker replicas) without touching the web tier.
- The FTP gateway can be restarted, upgraded, or scaled independently of both the web server and the job processor.
- All domain logic is written once and tested once; there are no serialization contracts or API versioning concerns between the three binaries.
- A single `go build ./...` produces all three artifacts; CI is straightforward.
- Engineers move between the three binaries without context switching to a different language, framework, or test harness.

### Negative

- The three binaries share a database schema; a schema migration must be backward-compatible with all three simultaneously (or deployments must be coordinated).
- Horizontal scaling of the worker tier requires the job queue (River/Postgres) to be the coordination point — workers must be stateless and idempotent.
- As the codebase grows, package boundary discipline must be maintained actively; Go's `internal/` enforcement helps but does not replace review discipline.

### Neutral

- The module-level boundary is the natural place to introduce a new deployable in future (e.g., a timing-feed ingestion daemon in Y1) without restructuring the codebase.
