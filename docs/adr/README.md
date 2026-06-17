# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) documenting significant architectural choices made for the Paddock project. Each ADR captures the context, the decision, the alternatives considered and why they were rejected, and the consequences (positive and negative) of the chosen approach.

ADRs are permanent. When a decision is superseded by a later one, the original ADR is marked as superseded and a new ADR is created; the original is never deleted.

## Index

| ADR | Title | Status | Date |
|---|---|---|---|
| [ADR-0001](0001-modular-monolith-three-deployables.md) | Modular Monolith, Three Deployables Sharing One Codebase | Accepted | 2026-06-16 |
| [ADR-0002](0002-server-rendered-ui-htmx.md) | Server-Rendered UI — Go html/template + HTMX + tus-js-client | Accepted | 2026-06-16 |
| [ADR-0003](0003-s3-compatible-object-storage.md) | S3-Compatible Object Storage in an EU Region | Accepted | 2026-06-16 |
| [ADR-0004](0004-standalone-ftp-sftp-gateway.md) | Standalone FTP/SFTP Ingest Gateway | Accepted | 2026-06-16 |
| [ADR-0005](0005-river-postgres-job-queue.md) | River (Postgres-Backed) for the Async Job Queue; Redis Is Ephemeral-Only | Accepted | 2026-06-16 |
| [ADR-0006](0006-imagemagick-image-processing.md) | ImageMagick (Shell-Out) for Image Processing on the Worker Tier | Accepted | 2026-06-16 |
| [ADR-0007](0007-mailer-interface-pluggable-provider.md) | Mailer Interface with One Pluggable EU Default Provider | Accepted | 2026-06-16 |
| [ADR-0008](0008-multitenancy-postgres-rls.md) | Multitenancy via Postgres Row-Level Security from Day One | Accepted | 2026-06-16 |
| [ADR-0009](0009-data-access-pgx-hand-written-sql-two-tier-testing.md) | Data Access — pgx via database/sql Adapter, Hand-Written SQL, Two-Tier Testing | Accepted | 2026-06-16 |
| [ADR-0010](0010-delivery-presigned-urls-licence-event-at-issue.md) | Delivery via Short-Lived Presigned S3 URLs; Licence Event Logged at Issue Time | Accepted | 2026-06-16 |
| [ADR-0011](0011-local-dev-ci-docker-compose-testcontainers.md) | Local Dev and CI via docker-compose and testcontainers-go | Accepted | 2026-06-16 |

## Tags

| Tag | ADRs |
|---|---|
| architecture | ADR-0001, ADR-0002 |
| storage | ADR-0003, ADR-0010 |
| ingest | ADR-0004, ADR-0006 |
| jobs / async | ADR-0005 |
| image-processing | ADR-0006 |
| email | ADR-0007 |
| multitenancy | ADR-0008, ADR-0009 |
| data-access | ADR-0009 |
| delivery | ADR-0010 |
| testing | ADR-0009, ADR-0011 |
| developer-experience | ADR-0011 |
| eu-residency | ADR-0003, ADR-0007 |
| security | ADR-0008, ADR-0010 |
