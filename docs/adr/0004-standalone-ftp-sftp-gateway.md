# ADR-0004: Standalone FTP/SFTP Ingest Gateway

**Status**: Accepted
**Date**: 2026-06-16

## Context

Camera-native FTP/SFTP is the primary ingest path for accredited photographers. Canon R1/R3, Nikon Z9/Z8, and Sony A1/A9 III bodies transmit raw files directly to an FTP endpoint via Photo Mechanic or built-in camera FTP. This is the muscle memory of working press photographers; any deviation from it is an adoption barrier.

The ingest path has its own requirements that differ sharply from the web-serving and job-processing paths:

- **Per-photographer authentication**: each FTP session authenticates a specific accredited photographer; credentials map to an identity and thus to auto-routing rules (event, session, copyright).
- **High-volume, bursty**: at session end, several photographers may be transmitting simultaneously; the ingest path must never block or back-pressure the serving path.
- **Availability independence**: a gateway restart or deployment must not affect journalists browsing galleries or sponsors downloading assets.
- **Auto-routing**: the gateway must read IPTC metadata and capture-time to route each file to the correct event and session without photographer intervention.
- **Blast-radius isolation**: a bug or crash in the FTP server must not take down the web or worker tiers.

## Decision

We will build `cmd/ftp-gateway` as a standalone deployable: a separate binary that runs an FTP/SFTP server, authenticates per-photographer, streams uploads directly to the EU object-storage bucket, and enqueues an ingest job on River (via Postgres) for each received file. It never communicates with `cmd/web` or `cmd/worker` at runtime; all coordination flows through the shared database and object store.

Auto-routing logic (photographer identity → event + session mapping; IPTC parse; capture-time window matching) lives in `internal/ingest` and is shared with the worker's ingest processing path.

## Alternatives Considered

### Embed the FTP server in the worker binary

**Pros:**
- One fewer binary to build, deploy, and operate.
- FTP authentication can share the worker's database connection pool.

**Cons:**
- Couples ingest availability to worker deployments. A worker restart to deploy a new rendition pipeline or fix a job-processing bug takes down the FTP server; photographers in the middle of transmitting 300 files lose their session.
- The worker tier is designed to be scaled horizontally; multiple worker instances each running an FTP listener on the same port is not viable without a separate load balancer and sticky-session routing.
- The FTP protocol's connection model (long-lived control + data connections) is poorly suited to sharing a process with a short-burst job consumer.

**Why rejected**: Ingest availability must be independent of worker deployments. The two have different scaling requirements and different failure modes. Coupling them in a single binary creates an availability dependency that violates the brief's degradation requirement ("ingest never blocks").

### Managed SFTP (e.g., AWS Transfer Family)

**Pros:**
- No FTP server to operate; the provider manages availability, scaling, and protocol compliance.
- Connects directly to S3.

**Cons:**
- AWS Transfer Family pricing is per-endpoint-hour plus per-GB transferred; at media scale (hundreds of GBs per event) this is a significant cost relative to a self-managed binary.
- Per-photographer authentication and routing logic must be implemented as Lambda functions or custom identity providers, which moves core product logic outside the Go codebase and into a vendor-specific compute layer.
- The auto-routing logic (IPTC parse, capture-time window matching, photographer-to-event mapping) is rich domain logic that must live in the application, not in a serverless shim.
- Weakens the EU/portability story: AWS Transfer Family is AWS-specific and would couple the ingest path to AWS even if the object storage is provider-agnostic.
- Weak per-photographer routing control: Transfer Family's identity provider model does not natively expose the IPTC-based routing context the gateway needs.

**Why rejected**: Cost at media scale is prohibitive, and the per-photographer routing and authentication logic is core product behavior that must live in the Go codebase, not in a vendor-specific compute shim.

## Consequences

### Positive

- Ingest availability is decoupled from web and worker deployments; photographers can transmit files while either of the other tiers is being updated or restarted.
- The FTP gateway can be scaled independently (multiple replicas behind a TCP load balancer) if ingest volume grows, without scaling web or worker capacity.
- A crash or resource exhaustion in the gateway does not affect the serving path; journalists and sponsors see no degradation.
- All gateway logic is in Go, tested with the same `net/http/httptest`-equivalent patterns, and subject to the same lint and coverage gates.
- The gateway's only runtime dependencies are the Postgres database (for auth and job enqueue) and the object store (for file upload); it has no HTTP dependency on the web tier.

### Negative

- Three deployables require three deployment configurations (container images, service definitions, health checks). This is modest operational overhead but is overhead nonetheless.
- The FTP/SFTP server implementation must be maintained in-tree; bugs in the protocol handling are the team's responsibility. Libraries exist (e.g., `pkg/sftp`, `goftp/server`) but require integration and testing.
- Long-lived FTP control connections mean the gateway cannot be replaced in a rolling deployment without disconnecting active upload sessions. Graceful shutdown (drain active sessions, stop accepting new connections) must be implemented.

### Neutral

- The gateway enqueues River jobs via Postgres in the same transaction as recording the received file; this is the same transactional-enqueue pattern used throughout the platform (see ADR-0005).
