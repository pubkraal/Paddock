# ADR-0011: Local Dev and CI via docker-compose and testcontainers-go

**Status**: Accepted
**Date**: 2026-06-16

## Context

Paddock depends on several stateful services: Postgres (domain data and River job queue), Redis (sessions and rate limits), MinIO (object storage), and Mailpit (SMTP sink for transactional email in dev). The FTP gateway also needs an FTP/SFTP target for local testing.

The team must be able to run the full application stack locally and run all tests (unit and integration) in CI without environment drift. The CLAUDE.md pre-commit gates (`go test ./internal/...`, `golangci-lint`) must be runnable after a fresh `git clone` with no manual service setup.

Integration tests (ADR-0009) require a real Postgres instance with the current schema applied; they cannot use sqlmock. CI must provide this without requiring a pre-provisioned database service.

## Decision

**Local development:** `docker-compose.yml` in the project root defines all backing services:

| Service | Image | Purpose |
|---|---|---|
| `postgres` | `postgres:17` | Domain data, River queue, migrations target |
| `redis` | `redis:7-alpine` | Sessions, magic-link tokens, rate limits |
| `minio` | `minio/minio` | EU-bucket-compatible object storage in dev |
| `mailpit` | `axllent/mailpit` | SMTP sink + web UI for email inspection |
| `sftp` | `atmoz/sftp` or equivalent | FTP/SFTP target for gateway testing |

A `Makefile` provides targets that mirror the CLAUDE.md pre-commit gates:

```
make up          # docker-compose up -d (all services)
make migrate     # golang-migrate up
make test        # go test ./internal/... -count=1
make test-int    # go test ./internal/... -tags integration -count=1
make lint        # golangci-lint run -c build/ci/golangci.yml
make build       # go build ./...
make down        # docker-compose down
```

**CI:** Integration tests use `testcontainers-go` to spin up Postgres (and MinIO where needed) directly inside the test process. CI runners need only Docker; no pre-provisioned services are required. Unit tests (sqlmock) run without Docker.

The `integration` build tag gates integration tests so that `go test ./internal/...` (without the tag) runs only unit tests and is fast; `go test ./internal/... -tags integration` runs both tiers.

## Alternatives Considered

### Dev containers (VS Code devcontainer / GitHub Codespaces)

**Pros:**
- Full environment (editor + services) in one definition file.
- Reproducible across machines and in Codespaces.

**Cons:**
- Couples the development environment to a specific editor (VS Code); engineers using GoLand, Neovim, or other editors do not benefit without additional configuration.
- devcontainer startup is heavier than `docker-compose up`; first-run image build takes several minutes.
- Adds a layer of abstraction (devcontainer spec → docker-compose or Dockerfile) that makes it harder to understand what services are running and how they are configured.
- The Makefile targets must still exist for CI; the devcontainer is an extra layer on top, not a replacement.

**Why rejected**: Editor coupling and added complexity without proportionate benefit. `docker-compose` provides the reproducible environment with less overhead and no editor dependency.

### Plain local service installation (Postgres, Redis, MinIO installed on the host)

**Pros:**
- No Docker dependency; faster service startup.
- Familiar to engineers who prefer native tools.

**Cons:**
- Service versions diverge across machines and over time (Postgres 14 on one laptop, 17 in CI; Redis 6 vs 7).
- Schema state diverges: an engineer who has been running a stale migration is unaware until a test fails in unexpected ways.
- New engineer onboarding requires installing and configuring four separate services with the correct versions and settings.
- MinIO configuration (bucket names, policies, credentials) must be documented and replicated manually.
- "Works on my machine" bugs from version drift are disproportionately expensive at small team size.

**Why rejected**: Environment drift is the direct failure mode this decision addresses. The "live in one afternoon" positioning applies to the development environment as well as the product; `git clone && make up && make migrate && make test` must work on a new machine.

## Consequences

### Positive

- A new engineer can run the full test suite (unit + integration) after `git clone && make up && make migrate` with no additional setup beyond Docker being installed.
- The Makefile targets exactly mirror the CLAUDE.md pre-commit gates; there is no gap between "what I run locally" and "what CI runs."
- testcontainers-go integration tests are hermetic: each test run starts a fresh Postgres container with the current schema; there is no shared state between CI runs.
- The docker-compose stack is the specification of the production dependency surface; adding a new backing service requires updating compose, the Makefile, and the deployment configuration simultaneously, which makes the dependency explicit.
- Mailpit provides a web UI (`http://localhost:8025`) for inspecting outbound emails during local development, which is essential for testing the magic-link and embargo-notification flows.

### Negative

- Docker is a hard dependency for integration tests and local development. Engineers without Docker (rare, but possible on constrained machines) cannot run integration tests locally; they must rely on CI.
- `testcontainers-go` adds startup latency to integration test runs (2–5 seconds per package for Postgres container startup). This is acceptable for an integration tier that is explicitly separated from the unit-test tier by a build tag, but engineers must understand that `make test-int` is slower than `make test`.
- The docker-compose stack is a local-development tool, not a production deployment specification. Engineers must not assume that compose configuration maps directly to production infrastructure; production deployment is a separate concern (see ADR-0001 for deployable structure).

### Neutral

- The `integration` build tag convention means CI can choose to run only unit tests on every push and run the full integration suite on PR merge or on a schedule, if integration test speed becomes a constraint at scale.
