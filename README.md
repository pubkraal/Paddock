# Paddock

Motorsport-native media operations platform. A press officer sets up a race
weekend, photographers push frames trackside, the press office curates and
applies embargoes, and accredited journalists / sponsors / teams browse a
branded per-event gallery and download assets under a licence.

## Documentation

- [`BRIEF.md`](./BRIEF.md) — what we're building (MVP scope, NFRs, verification).
- [`PLAN.md`](./PLAN.md) — the phased implementation plan (architecture, phases, design system).
- [`docs/adr/`](./docs/adr/README.md) — architecture decision records.
- [`docs/design handoff/`](./docs/design%20handoff/) — visual + interaction design spec.

## Stack

Modular monolith, three deployables (`cmd/web`, `cmd/worker`, `cmd/ftp-gateway`)
sharing one Go module. Server-rendered `html/template` + HTMX. Postgres with
row-level-security multitenancy, River job queue, S3-compatible EU object
storage. See `PLAN.md` §1.

## Development

```bash
make up        # start backing services (postgres, redis, minio, mailpit, sftp)
make migrate   # apply schema (golang-migrate) + River's own migrations
make run       # run cmd/web
make test      # unit tier (no Docker)
make test-int  # integration tier (testcontainers)
make lint      # golangci-lint
```

Requires Go 1.26+ and Docker.
