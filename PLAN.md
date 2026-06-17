# PADDOCK — MVP Implementation Plan

**Audience:** the coding agent that will build the Paddock MVP.
**Source of truth for *what*:** [`BRIEF.md`](./BRIEF.md) (§7 MVP scope, §8 NFRs, §9 verification).
**Source of truth for *how* (architecture):** [`docs/adr/`](./docs/adr/README.md).
**Source of truth for *visual + interaction design*:** [`docs/design handoff/`](./docs/design%20handoff/README.md) — design tokens, components, the 31-screen board (`Paddock.dc.html`), and the `Paddock Prototype` behavioural spec. The design contributes *design only*; this plan and the ADRs govern architecture and scope. See §4.
**Source of truth for *style*:** the global `CLAUDE.md` development guidelines (Go, strict TDD, 100% behavioural coverage, `_test` packages, stdlib testing, `golangci-lint`).

> **Governing constraint (read this first):** *low barrier to entry and speed is everything.* Two meanings, both binding:
> 1. **Product:** a press officer with no training must run their first race weekend the same day they sign up. Every phase ships with onboarding templates and self-serve flows.
> 2. **Operations:** media ingest is bursty and heavy and must **never** degrade the serving path. Media work is asynchronous, on a separately-scaled worker tier.

---

## 0. How to use this plan

- **Build in phase order.** Each phase is a vertical slice that leaves the tree green (builds, 100% coverage, lint clean) and is independently demonstrable. Do not start a phase until the previous one's **Definition of Done** is met.
- **TDD is non-negotiable.** No production line exists without a failing test demanding it (red → green → refactor). See §3 for the two-tier testing model RLS forces on us.
- **Every phase lists the ADRs it implements.** If an implementation detail is not covered by an ADR or this plan, it is an *open decision* — see §7 — and must be confirmed with the product owner before being finalized, not chosen unilaterally.
- **Domain model fidelity (BRIEF §5):** model the full motorsport hierarchy and the Asset/Entitlement/Embargo/Licence-Event primitives correctly **even where the MVP UI does not yet expose them**. Every Y1+ feature is a function over this model; getting it wrong now is the expensive mistake.

### Definition of Done (applies to every phase, every PR)

Mirror the CLAUDE.md pre-commit gates exactly:

```bash
gofumpt -w . && gofmt -w .            # zero diff
go mod tidy                           # zero diff
go build ./...                        # compiles
make test                             # unit tier — 100% coverage of changed behaviour
make test-int                         # integration tier (Docker) — RLS + repos + pipeline
golangci-lint run -c build/ci/golangci.yml --new-from-rev=main
```

A phase is done only when all gates are green **and** its acceptance criteria below are demonstrably met.

---

## 1. Locked architecture (the stack)

| Area | Decision | ADR |
|---|---|---|
| Topology | Modular monolith, **three deployables** sharing one codebase: `cmd/web`, `cmd/worker`, `cmd/ftp-gateway` | [0001](./docs/adr/0001-modular-monolith-three-deployables.md) |
| UI | Server-rendered Go `html/template` + HTMX + `tus-js-client` (resumable upload). No SPA. Dark "Graphite" everywhere + optional light "Paper" theme for the media portal — design system in §4. | [0002](./docs/adr/0002-server-rendered-ui-htmx.md) |
| Object storage | S3-compatible, EU region, via `aws-sdk-go-v2`; MinIO in dev/CI | [0003](./docs/adr/0003-s3-compatible-object-storage.md) |
| Ingest | Standalone FTP/SFTP gateway → streams to bucket → enqueues ingest job; never on the serving path | [0004](./docs/adr/0004-standalone-ftp-sftp-gateway.md) |
| Async jobs | **River** (Postgres-backed, transactional enqueue); Redis is ephemeral-only | [0005](./docs/adr/0005-river-postgres-job-queue.md) |
| Image processing | ImageMagick shell-out on the worker tier, behind an `ImageProcessor` interface | [0006](./docs/adr/0006-imagemagick-image-processing.md) |
| Email | `Mailer` interface + one pluggable EU default; Mailpit in dev | [0007](./docs/adr/0007-mailer-interface-pluggable-provider.md) |
| Tenancy | Postgres **RLS from day one** (`SET LOCAL app.current_org`) | [0008](./docs/adr/0008-multitenancy-postgres-rls.md) |
| Data layer | pgx via `database/sql` adapter, hand-written SQL; sqlmock units + testcontainers integration; golang-migrate | [0009](./docs/adr/0009-data-access-pgx-hand-written-sql-two-tier-testing.md) |
| Delivery | Short-lived presigned S3 URLs; Licence Event logged at issue time | [0010](./docs/adr/0010-delivery-presigned-urls-licence-event-at-issue.md) |
| Dev/CI | docker-compose (postgres/redis/minio/mailpit) + Makefile + testcontainers-go | [0011](./docs/adr/0011-local-dev-ci-docker-compose-testcontainers.md) |

**Design system & UI fidelity:** the visual + interaction layer (tokens, racing-flag status spine, components, two themes, screen→phase map) is specified in **§4**, sourced from `docs/design handoff/`. It is high-fidelity and reproduced faithfully, but it does not alter the architecture or scope locked above.

### Data flow (the three tiers)

```
   Photographer (Camera FTP / Photo Mechanic / Capture One)
        │  SFTP/FTPS, per-photographer credentials
        ▼
  ┌─────────────────┐   stream bytes    ┌──────────────────┐
  │  cmd/ftp-gateway │ ────────────────▶ │  EU S3 bucket     │
  └─────────────────┘                   └──────────────────┘
        │ enqueue ingest job (River, in txn with Asset row)
        ▼
  ┌──────────────────────────────────────────────────────────┐
  │  Postgres (catalog, rights, audit, RLS, River job tables) │
  └──────────────────────────────────────────────────────────┘
        ▲                                   ▲
        │ drains jobs                       │ reads/writes (org-scoped, RLS)
  ┌─────────────┐                     ┌─────────────┐
  │ cmd/worker  │  ImageMagick →      │  cmd/web    │  html/template + HTMX
  │ renditions, │  S3 renditions      │  admin +    │  presigned URLs out
  │ watermarks, │                     │  portals    │
  │ ZIPs, email,│                     └─────────────┘
  │ embargo lift│                            │
  └─────────────┘                            ▼
                                     Consumers (journalist/sponsor/team)
                                     magic-link, no app install
```

---

## 2. Repository layout

```
paddock/
├── cmd/
│   ├── web/            # HTTP serving: admin UI + portals + delivery endpoints
│   ├── worker/         # River job consumer: ingest, renditions, watermarks, ZIP, email, embargo
│   └── ftp-gateway/    # SFTP/FTPS server; streams to S3, enqueues ingest jobs
├── internal/
│   ├── platform/       # cross-cutting infra (no domain logic)
│   │   ├── config/     # 12-factor env config (see twelve-factor skill)
│   │   ├── postgres/   # pgx pool, txn helper that sets `app.current_org` GUC
│   │   ├── redis/      # ephemeral store: sessions, magic tokens, rate limits
│   │   ├── objectstore/# S3 abstraction (aws-sdk-go-v2); MinIO-compatible
│   │   ├── queue/      # River client + worker registration
│   │   ├── mailer/     # Mailer interface + adapters (SMTP/Postmark-EU, noop, mailpit)
│   │   ├── imageproc/  # ImageProcessor interface + ImageMagick adapter (worker only)
│   │   └── httpx/      # router, middleware (auth, org-scope, request-id, recovery)
│   ├── identity/       # Organization, User, accounts, sessions, magic-link
│   ├── catalog/        # Championship→Season→Event→Session, Venue, EntryList, Asset, galleries, search
│   ├── ingest/         # routing rules, IPTC/XMP parse, dedupe, ingest job handlers
│   ├── rights/         # Entitlement tiers, accreditation binding, embargo, watermark policy
│   └── delivery/       # share links, presigned issuance, ZIP, Licence Event / audit
├── web/                # server-rendered UI (design system §4)
│   ├── templates/      # html/template — layouts, pages, HTMX partials
│   ├── components/     # reusable partials: racing-flag badge, image-tile, nav rail, table, inspector, uploader
│   └── static/
│       ├── css/        # token layer — graphite (dark) + paper (light) themes
│       ├── fonts/      # Archivo + JetBrains Mono (self-hosted)
│       └── js/         # htmx, tus-js-client + bespoke uploader (chunk-grid, pause/resume, reconnect)
├── migrations/         # golang-migrate SQL (schema + RLS policies)
├── build/
│   ├── ci/golangci.yml
│   └── package/        # Dockerfiles per deployable (web, worker, ftp-gateway)
├── deploy/
│   └── docker-compose.yml
├── test/
│   ├── integration/    # testcontainers-go helpers (real Postgres/MinIO)
│   └── harness/        # Virtual Race Weekend harness + fixtures (Phase 10)
├── Makefile
└── docs/adr/
```

Interfaces are defined **where they are consumed** (CLAUDE.md), so e.g. `catalog` declares the `ObjectStore` interface it needs; `platform/objectstore` provides the concrete struct. Accept interfaces, return structs.

---

## 3. Cross-cutting conventions (establish in Phase 0, obey everywhere)

- **Tenancy invariant (ADR-0008):** every tenant-owned table has `org_id`; every DB access runs inside a transaction that first executes `SET LOCAL app.current_org = $org`. RLS policies do the enforcing. A `postgres.WithOrg(ctx, orgID, fn)` helper is the *only* sanctioned way to touch tenant data — no raw queries outside it.
- **Two-tier tests (ADR-0009):**
  - **Unit tier** (`make test`): handlers, services, pure logic. go-sqlmock for DB expectations, miniredis for Redis, hand-written interface mocks. Fast, no Docker. This is where 100% behavioural coverage is driven.
  - **Integration tier** (`make test-int`, `//go:build integration`): real Postgres + MinIO via testcontainers-go. **Mandatory for:** every RLS policy (prove tenant A cannot read/write tenant B), repository SQL correctness, the ingest→rendition pipeline, and presigned-URL round-trips. sqlmock *cannot* prove RLS — integration tests are how isolation is verified.
- **Two DB roles (deploy requirement, ADR-0008/0009):** the golang-migrate connection uses a **superuser/BYPASSRLS** role; the application connection uses a non-superuser role that RLS applies to. These are distinct connection strings in config. The integration suite must connect as the *application* role so RLS is actually exercised.
- **Config:** 12-factor, env-driven (`twelve-factor` skill). One typed `Config` struct per deployable, validated at boot; fail fast on missing required vars. No global state; inject dependencies via constructors.
- **Errors:** sentinel errors (`var errFoo = errors.New(...)`), wrap with `%w`, check with `errors.Is/As`. Guard clauses, max 2 levels nesting.
- **Graceful shutdown / disposability:** every deployable handles SIGTERM, drains in-flight work (web: connections; worker: current job; gateway: active transfers), bounded timeout.
- **Ingest never blocks (BRIEF §8 degradation mode):** the gateway accepts and persists bytes even when the worker is backed up; processing catches up from the queue.

---

## 4. Design system & UI fidelity

**Source:** `docs/design handoff/` — README (token + behaviour spec), `Paddock.dc.html` (31-screen static board), `Paddock Prototype.dc.html` (behavioural source of truth for ingest→curate→publish + the resumable uploader). Fidelity is **high**: colours, typography, spacing, status language, and interactions are reproduced faithfully.

**Phasing rule (confirmed):** UI is built **inside each backend slice — there is no separate design phase.** Phase 0 establishes the *design-system primitives only* (tokens, fonts, base components, both theme scaffolds) as shared infrastructure. Every *feature* screen is then built in the phase that owns its backend, wired to that phase's real data, with seed/fixtures making the slice demoable as it lands.

### Themes (confirmed: dark admin + light media portal)

- **Dark "Graphite"** — primary, used across admin, curation, and portals. App bg `#0b0d10`, card `#0e1013`, borders `#20242b / #1c2026 / #181c22`, text `#e9e7e2` (primary) → `#5f656d` (dim).
- **Light "Paper"** — built as an option for the **media / sponsor portal** (daylight press-centre): bg `#f4f2ec`, card `#fbfaf6`, text `#16181c`. The chrome flips; **photo tiles stay dark in both schemes.** Admin/curation stays dark-only.
- **One accent:** Paddock red `#e0452f` — primary buttons, live dots, active nav. No other brand colour.

### Typography (Google Fonts, self-hosted)

- **Archivo** (400–900) — all UI text; headings/big counts 700–800, `letter-spacing −.01/−.02em`; body 400–600.
- **JetBrains Mono** (400–700) — **all data**: filenames, car № (`№31`), timecodes, IDs, chunk counts, mono labels (uppercase, `.06–.12em`, 10–11px, `#6b6e74`). Never rely on emoji.

### Racing-flag status system — the semantic spine

The design's flag colours are the **visual projection of the typed Asset state enum** (built in Phase 5). One source of truth: the enum drives the badge — they are never modelled twice.

| Asset state (enum) | Flag | Hex | Badge label |
|---|---|---|---|
| `raw` | Neutral | `#9aa0a8` | RAW |
| `select` | Yellow | `#e0a23f` | SELECT |
| `published` | Green | `#43b97a` | PUBLISHED |
| `embargoed` | Red | `#e0452f` | EMBARGO |
| `killed` | Red | `#e0452f` | KILLED |
| `archived` | Chequered | `#cfd3d9` | CLOSED |

`Sponsor` blue `#4d8fe0` is a **tier / visibility** badge (Phase 6), **not** an asset state. **Badge component:** pill = 6px status dot + uppercase mono label on a 14–18% tint of the status colour; reused on tiles, tables, and inspectors.

### Tokens — radii / shadow / spacing

Radius: cards 11–14px, controls 7–9px, tiles 6–9px, pills 999px, chunk cells 1.5px. Card shadow `0 24px 50px -24px rgba(0,0,0,.45)`. Gap-based flex/grid (no margin spacing); 18–34px card padding; app shell `1280×820`. Image placeholders use the diagonal-hatch pattern until real renditions exist.

### Screen → phase map

| Design screen group | Realised in |
|---|---|
| Foundations — tokens, badge, nav, base components | Phase 0 (primitives only) |
| Access — login, magic-link landing | Phase 1 |
| Admin — event setup wizard, entry list, accreditation; role **dashboards** (press officer, photographer, sponsor, team-comms, federation/archive) | Phase 2 |
| Resumable upload — chunk-grid uploader + FTP credentials panel | Phase 4 |
| Ingest & Curation — **Layout A · Triptych** (default) **+ Layout C · Lightbox** | Phase 5 |
| Entitlement tiers; **Sponsor portal** (category-lock / conflict overlay) | Phase 6 |
| Embargo control — embargoed-but-visible | Phase 7 |
| Media gallery, Asset detail + licence download, Audit log | Phase 8 |
| Billing | Phase 9 |
| Distribute / Virtual Race Weekend | Phase 10 |

---

## 5. Domain model checklist (BRIEF §5 — build correctly even if UI-hidden)

| Aggregate | MVP fields that must exist | Phase |
|---|---|---|
| Organization | type (series/promoter/circuit/team/ASN), residency region | 1 |
| Championship → Season → Event → Session | full hierarchy; Session types (FP/Quali/Race/Podium/Paddock) | 2 |
| Venue | circuit map ref, corner/marshal-post zones (data only, tagging later) | 2 |
| Entry List | Car № → Team → Driver lineup → Class → livery refs | 2 |
| Accreditation | person → tier → validity window → credential ref | 2 |
| Asset | immutable source (EXIF+IPTC/XMP), enrichment layer (session/car/etc, nullable now), rights record, **state** (`raw/select/published/embargoed/archived/killed`) | 4 |
| Entitlement Tier | resolution ceiling, watermark policy, licence text, embargo behaviour; bound to accreditation or named account | 6 |
| Embargo | manual / scheduled / session-state-triggered (state hook stubbed for MVP); per asset / gallery / tier | 7 |
| Licence Event | who / which rendition / when / licence-text-version / IP (at issue, per ADR-0010) | 8 |

---

## 6. Phases

Each phase: **Goal · Deliverables · Migrations · Tests · Acceptance · ADRs · Depends on.**

### Phase 0 — Foundation & walking skeleton
- **Goal:** an empty-but-real system that builds, tests, lints, and boots all three deployables doing nothing useful yet.
- **Deliverables:** repo layout (§2); `Makefile` (`run`, `migrate`, `test`, `test-int`, `lint`, `fmt`); `build/ci/golangci.yml`; `deploy/docker-compose.yml` (postgres, redis, minio, mailpit, an SFTP target); typed `Config` per deployable; `platform/postgres` pool + `WithOrg` txn helper (GUC wiring); `platform/redis`; `platform/objectstore` (S3/MinIO ping); `platform/queue` (River bootstrap, empty registry); `platform/httpx` (router, request-id/recovery/logging middleware); `/healthz` + `/readyz` on `cmd/web`; skeleton `cmd/worker` (River consumer loop) and `cmd/ftp-gateway` (listens, rejects all) that start and shut down gracefully; golang-migrate wired with the two-role convention.
- **Design-system primitives (§4) — primitives only, NO feature screens:** self-hosted Archivo + JetBrains Mono; the CSS token layer for both themes (Graphite dark + Paper light scaffold); `web/components` base set — buttons, inputs, cards, nav rail, table, inspector shell, image-tile (hatch placeholder), and the racing-flag **badge** component driven by a status token; a base `html/template` layout + HTMX wiring. A throwaway `/_styleguide` route renders the component set for visual QA.
- **Migrations:** `0001_init` — extensions, `app.current_org` GUC convention documented, River's own tables, application role.
- **Tests:** config validation (unit); `WithOrg` sets/clears the GUC (integration); health endpoints (unit); graceful shutdown (unit); badge component renders the correct flag class/label per status token (unit, template render).
- **Acceptance:** `make run` brings the stack up; all three binaries boot and stop cleanly; CI green on a no-op build; `/_styleguide` renders the token themes and badge states faithfully (§4).
- **ADRs:** 0001, 0005, 0008, 0009, 0011.
- **Depends on:** —

### Phase 1 — Identity, tenancy & access
- **Goal:** organizations exist, isolation is enforced and *proven*, admins log in, consumers get magic-link access.
- **Deliverables:** `identity` package — Organization, User (admin accounts: press officer / season admin / finance roles), session auth (Redis-backed sessions), **magic-link** issuance + redemption (Redis token, single-use, TTL) for accredited consumers (no password). RLS policies for the first tenant tables. Auth + org-scope middleware that resolves the caller's org and feeds `WithOrg`. Login + magic-link landing pages (HTMX), per the **Access** screens (§4), in the Graphite theme.
- **Migrations:** organizations, users, sessions metadata; RLS enabled + policies on every tenant table introduced.
- **Tests:** **integration (mandatory):** tenant A cannot SELECT/INSERT/UPDATE tenant B's rows under RLS; magic-link single-use + expiry; session lifecycle. Unit: handler auth flows (sqlmock + miniredis), role gating.
- **Acceptance:** seed two orgs; prove cross-tenant reads return zero rows at the DB level; an admin logs in; a magic link grants scoped consumer access then cannot be reused.
- **ADRs:** 0008, 0009, 0007 (magic-link email), 0002.
- **Depends on:** 0.

### Phase 2 — Domain setup from templates (the same-day onboarding)
- **Goal:** *the* barrier-to-entry feature — a press officer sets up a full event from a template in minutes, with entry list and accreditations imported.
- **Deliverables:** `catalog` hierarchy (Championship→Season→Event→Session) + Venue; **onboarding templates** (sprint weekend, endurance, rally placeholder) that scaffold sessions/structure on event creation; **Entry-list import** (CSV/Excel → Car №/Team/Driver/Class/livery refs) with preview + error report; **Accreditation CSV import** (csv, 4 tiers, column mapping) → bulk consumer account provisioning, each emailed a magic link (worker job via Mailer); admin setup UI (HTMX wizard) per the **Event setup** screens — "Go live" in ~6 min; **role dashboards** (press officer, photographer, sponsor, team-comms, federation/archive) per the design board, each surfacing its primary action (e.g. photographer `↥ Upload photos`, press `↥ Add photos` / `Ingest`).
- **Migrations:** championship/season/event/session, venue, entry_list/entries, accreditations; all RLS-scoped.
- **Tests:** template scaffolding produces correct session sets (table-driven); CSV/Excel parsing incl. malformed rows and dedupe (unit, factories); bulk provisioning enqueues N magic-link emails (unit, queue mock); integration: imported rows are org-scoped.
- **Acceptance:** from a template, import a 40-car entry list and a 120-person accreditation CSV; 120 magic-link invites enqueued; event is ready to receive media — all without leaving the wizard.
- **ADRs:** 0002, 0005, 0007, 0009.
- **Depends on:** 1.

### Phase 3 — Storage & async media backbone
- **Goal:** the asynchronous spine that keeps heavy media off the serving path.
- **Deliverables:** finalize `objectstore` (put/get/presign, content-addressed keys, original-retention bucket layout); `imageproc.ImageProcessor` interface + **ImageMagick adapter** with a **bounded concurrency / back-pressure cap** on the worker (ADR-0006); River job type definitions and a registered no-op `RenditionJob` end-to-end (enqueue on web/test → worker generates a derivative → stored in S3); worker concurrency config.
- **Migrations:** assets table stub (id, org, source key, checksum, state) to let jobs reference rows transactionally.
- **Tests:** integration: enqueue→worker→S3 round-trip with MinIO; ImageMagick adapter produces expected dimensions/format from a fixture; concurrency cap holds under N enqueued jobs. Unit: job payload (de)serialization, processor interface with a fake.
- **Acceptance:** a fixture image enqueued as a job is processed by the worker and the derivative lands in MinIO; web tier latency is unaffected while a burst of jobs runs.
- **ADRs:** 0003, 0005, 0006.
- **Depends on:** 0 (independent of 1–2; can proceed in parallel after 0).
- **Deferred from Phase 0 review (PR #1):** Phase 0 only *constructs* River clients (non-nil assertions). The transactional-enqueue semantics — enqueue inside `WithOrg`'s `*sql.Tx`, payload round-trip, dedupe — get real behavioural coverage here. Also pin the **MinIO image tag** in `deploy/docker-compose.yml` (left floating in Phase 0).

### Phase 4 — Ingest (FTP gateway + web upload + pipeline)
- **Goal:** photographers transmit at trackside speed; assets appear with full metadata, deduped, originals retained.
- **Deliverables:** `cmd/ftp-gateway` — SFTP/FTPS server, **per-photographer credentials** (provisioned per event/accreditation), streams uploads straight to S3, then enqueues an ingest job in the same txn as the Asset row; **auto-routing** rules (photographer identity / IPTC / capture-time → event+session); **IPTC/XMP + EXIF preservation** (immutable source layer, never destroyed); **duplicate detection** (checksum); **original always retained**. Resumable **web upload** — `tus` protocol with a **bespoke uploader UI** (the prototype is the behavioural source of truth, §4): per-file **chunk-grid** (7px cells — verified=green `#43b97a`, in-flight=amber `#e0a23f` for the next 3, pending=`#1c2026`), live `47 / 63 chunks` counter, **1 MB chunks, ≤3 in parallel, checksum per chunk**, per-file + "pause all / resume all" controls that **resume from the last verified chunk (never re-upload)**, and the first-class **reconnect-blip** state (`● RECONNECTING` → auto-resume) that demonstrates the trackside-dropout requirement; completion banner "All N files ingested · enriched and routed to curation in <60s". Visible **entry points** (key review point): photographer dashboard `↥ Upload photos` header button; press `Ingest` rail item (accented) + `↥ Add photos`. **FTP credentials panel** in the upload view (host/port/user/password/folder, copy + reveal). Ingest job handler: parse metadata, apply **batch metadata templates** (event/session/copyright stamped on ingest), generate thumbnail + web preview + (later) tier renditions.
- **Migrations:** assets full schema (source metadata, enrichment layer nullable, rights record, state), photographer credentials, ingest routing rules.
- **Tests:** routing resolution table-driven (filename/IPTC/time precedence); dedupe; IPTC/XMP preserved byte-for-byte; integration: SFTP upload → S3 + asset row + job enqueued + renditions produced; tus resumable chunking — a simulated mid-transfer drop resumes from the last verified chunk with no re-upload (the reconnect behaviour), and chunk-cell states render per the spec. Gateway auth: only valid per-photographer creds accepted.
- **Acceptance ("Zandvoort ingest slice"):** three photographers transmit ~800 images over SFTP during a session; each is routed to the correct event/session, deduped, original retained, thumbnail+preview generated; no serving-path impact; ingest queue drains within target.
- **ADRs:** 0004, 0003, 0005, 0006, 0009.
- **Depends on:** 2, 3.
- **Deferred from Phase 0 review (PR #1):** the Phase 0 `cmd/ftp-gateway` reject-all skeleton (`rejectGateway`) was an untested placeholder. The real SFTP/FTPS server built here carries full test coverage — per-photographer auth, routing, stream-to-S3, and resumable chunking.

### Phase 5 — Galleries, curation & search
- **Goal:** the press officer curates and the asset state machine governs visibility.
- **Deliverables:** session-structured **galleries** (server-paginated, lazy-loaded images, HTMX infinite scroll); **two curation view modes** built as a toggle, default **A** (§4): **Layout A · Triptych** — left live ingest feed (FTP×3 + web, per-file status/progress), centre 4-up status-bordered curation grid, right inspector (large preview, metadata, Select/Publish/Embargo/Kill); **Layout C · Lightbox** rapid-cull — one large preview + EXIF strip, keyboard-first (`P` publish, `S` select, `X` reject, `←/→` navigate), thin incoming filmstrip; **selects** flagging (click a tile to toggle SELECT; published tiles locked); **publish flow** — "Publish N selects" flips to PUBLISHED, fires a confirmation toast, appears in the media gallery immediately; **batch metadata templates** applied/edited in bulk; **kill switch** (asset recall → state `killed` → invalidate any outstanding links/renditions); enforce the Asset **state machine** (`raw/select/published/embargoed/archived/killed`) with legal transitions, projected to the racing-flag **badge** (§4 — enum is the single source, badge follows); **Search v1** — structured filters (event/session/car/team/driver/photographer/tag) + free-text over captions (Postgres FTS, no external search engine for MVP).
- **Migrations:** gallery/asset-state indexes, FTS (tsvector) column + GIN index, tags.
- **Tests:** state-machine transitions (table-driven, illegal transitions rejected); badge rendering follows the state enum across all states; Layout C keyboard map (`P/S/X/←/→`) drives the correct transition/navigation; kill switch invalidates links (integration); search filter combinations and FTS ranking; pagination correctness.
- **Acceptance:** curate a session gallery in both view modes (Triptych default + Lightbox keyboard-cull), flag selects, publish (they appear in the media gallery immediately), then kill an asset and confirm its links 404; search returns correct assets by car № and by caption text.
- **ADRs:** 0002, 0009.
- **Depends on:** 4.

### Phase 6 — Entitlement tiers & watermarking
- **Goal:** the five-tier rights model and per-tier renditions/watermarks.
- **Deliverables:** **five tier presets** (Media-Editorial, Sponsor-Commercial(category), Team, Internal, Public-Watermarked); per-tier **resolution ceiling**, **watermark policy** (incl. **sponsor-logo** overlay on public tier), **licence text** (versioned); bind tiers to **accreditation records or named accounts**; worker generates **per-tier renditions** (resolution-capped + watermarked) via ImageMagick; templated tier library so a new org gets all five on creation. **Sponsor portal** UI (§4): category-cleared assets only, competitor-prominent frames hidden by conflict rules, **category-locked overlay** where applicable, commercial watermark on every download; the `Sponsor` blue tier badge.
- **Migrations:** entitlement_tiers, licence_texts (versioned), tier-bindings, per-asset rendition records.
- **Tests:** resolution ceiling enforced per tier; watermark composited (fixture compare); tier→account/accreditation binding resolution; licence-text versioning. Integration: rendition generation per tier lands correct derivatives.
- **Acceptance:** the five presets exist on a new org; a sponsor account resolves to category-cleared, watermarked, resolution-capped renditions; media-editorial resolves to hi-res with editorial licence text.
- **ADRs:** 0006, 0008, 0009.
- **Depends on:** 5.

### Phase 7 — Embargo engine v1
- **Goal:** embargoes enforced by the system, not by hope.
- **Deliverables:** **manual + scheduled** embargoes per gallery/tier; **embargoed-but-visible** mode for the media tier — held frames render **blurred** with a red `EMBARGOED · lifts <time>` badge (§4); sponsor & public tiers cannot see the asset at all; scheduled **lift** via River scheduled jobs; a **session-state-trigger hook** modelled now ("lift at chequered flag + 15 min") but driven manually in MVP (timing integration is Y1); enforcement evaluated at delivery (Phase 8) and in gallery visibility (Phase 5).
- **Migrations:** embargoes (scope, mode, lift-at, state-trigger ref).
- **Tests:** scheduled lift fires at the right time (controllable clock — inject time, no `time.Now()` in logic); per-tier visibility matrix (media-visible vs sponsor-hidden); manual lift; overlapping/precedence rules. Integration: scheduled job lifts embargo and republishes.
- **Acceptance:** podium shots embargoed; media tier sees them flagged, sponsor tier cannot; embargo lifts on schedule and the assets become downloadable to sponsors.
- **ADRs:** 0005, 0008.
- **Depends on:** 6.

### Phase 8 — Delivery, licence acceptance & audit
- **Goal:** branded self-serve delivery with a complete, queryable audit trail.
- **Deliverables:** **branded per-event media gallery** (§4) — branded header, filter bar (session/class/car/team/search), 4-up grid with lazy/infinite scroll and a `Showing 60 of 2,418` counter, tiles showing car №, sponsor badge, hi-res label and download affordance; offered in the **light "Paper" theme** option for the media/sponsor portal (photo tiles stay dark; admin stays Graphite); **Asset detail + licence download** screen — rendition picker (Hi-res / Web), versioned licence text (e.g. Media-Editorial v2.3), explicit **accept checkbox**, `Accept & download`; **expiring share links**; on accept, write the **Licence Event** (who / which rendition / when / licence-version / **IP at issue**) and issue a **short-TTL presigned S3 URL** (ADR-0010); **multi-asset ZIP** built on the worker, delivered via presigned URL; **Audit log** screen for the press officer answering "who downloaded asset X" — table (time, user, org, asset id, rendition, tier, licence version, IP) + stat strip (downloads, distinct outlets, embargo leaks=0, licence completeness=100%) + **CSV export**. Entitlement + embargo checks gate every issuance.
- **Migrations:** share_links (expiring), licence_events (immutable, append-only).
- **Tests:** entitlement + embargo enforced before URL issuance (integration); licence event written exactly once per accept with correct fields; presigned URL TTL + scope; expired share link rejected; ZIP contents match selection; audit query correctness. **Zero-leak property test:** simulated downloads across tiers never issue a URL for an embargoed/unentitled asset.
- **Acceptance ("audit answers the question"):** a sponsor self-serves category-cleared images after accepting the licence; the licence event is recorded with IP and licence version; the press officer queries "who downloaded asset X" and gets a complete answer.
- **ADRs:** 0010, 0003, 0005, 0006, 0008, 0009.
- **Depends on:** 7.
- **Carried product decision:** the audit records authorization intent + the authorization-request IP at issue time, **not** the confirmed S3 byte-fetch — an accepted trade-off for egress economics (ADR-0010). Do not re-litigate in implementation.

### Phase 9 — Admin & billing (self-serve pilot SKU)
- **Goal:** a single-event pilot can be bought self-serve — the commercial on-ramp.
- **Deliverables:** per-event and per-season billing model; **self-serve checkout** for single-event pilots (Billing screen, §4); plan/entitlement gating (capped photographers/assets on the Event SKU per BRIEF §10); invoices/receipts via Mailer.
- **Migrations:** billing accounts, subscriptions/event-purchases, usage caps.
- **Tests:** checkout state machine; cap enforcement (e.g. asset/photographer limits on Event SKU); webhook handling (idempotent); proration/season logic. Integration: purchase provisions an org/event.
- **Acceptance:** a new customer self-serves a single-event purchase and immediately runs Phase 2 setup.
- **ADRs:** 0005 (async receipts), 0007, 0009.
- **Depends on:** 8.
- **⚠ Open decision — confirm before starting:** payment provider (see §7). Build the checkout behind a `PaymentProvider` interface so the choice is swappable and unit-testable; do not hard-wire a vendor before it is confirmed.

### Phase 10 — Virtual Race Weekend harness (the MVP acceptance gate)
- **Goal:** the replayable, externally-verifiable proof of MVP completeness (BRIEF §9).
- **Deliverables:** `test/harness` — injects an anonymized event dataset (entry list + accreditation CSV + an image corpus carrying real EXIF/IPTC) and asserts **end-to-end**: routing accuracy, embargo enforcement (**zero leaks** across simulated downloads), licence-event completeness (**100%**), and publish-latency. Encodes the **"Zandvoort Saturday test"** (BRIEF §7 MVP acceptance) as an automated run: template setup → 40-car entry list + 120-person accreditation import → 3 photographers × ~800 images via FTP in a 60-min session → selects published to media tier within 10 min of session end → sponsor self-serve category-cleared download → scheduled podium embargo lift → audit answers "who downloaded asset X".
- **Tests:** the harness *is* the test; it runs in the integration tier against the real stack (testcontainers + MinIO + the FTP gateway). Add latency/burst assertions against BRIEF §8 (1,000 images/hr sustained).
- **Acceptance (MVP phase gate):** harness run is green **and** the named-pilot real-world Zandvoort Saturday test passes. Per BRIEF §9, completeness is binary and externally verifiable — this phase is what makes it so.
- **ADRs:** all.
- **Depends on:** 9 (exercises every prior phase).

---

## 7. Open decisions — confirm with the product owner before the relevant phase

These surfaced during planning and are **provider/sub-choices not yet locked**. The architecture above does not depend on them, but they must be confirmed (not chosen unilaterally) before the phase that needs them. Each is isolated behind an interface so confirmation is low-cost.

| # | Decision | Needed by | Default leaning (to confirm) |
|---|---|---|---|
| O1 | Exact EU **S3-compatible provider** for prod (Hetzner / Scaleway / Cloudflare R2 EU / AWS S3 eu-*) | Phase 3 deploy | Code is provider-agnostic; pick on cost/egress/residency |
| O2 | Exact **email provider** behind `Mailer` (Postmark-EU / SES-eu / SMTP relay) | Phase 1 (magic links) | SMTP relay for dev-parity, one hosted adapter for prod |
| O3 | **FTP/SFTP server library** for the gateway (e.g. `pkg/sftp`+`crypto/ssh`; FTPS lib choice) | Phase 4 | SFTP via `pkg/sftp`; confirm FTPS scope/lib |
| O4 | **Payment provider** for self-serve checkout (Stripe / Mollie / Paddle) | Phase 9 | Behind `PaymentProvider` interface; Mollie/Stripe EU-friendly |
| O5 | **Excel** parsing scope for entry lists (xlsx only, or xls too?) | Phase 2 | xlsx via a maintained Go lib; CSV is the guaranteed path |

---

## 8. Explicitly out of MVP (BRIEF §7 — do not build)

Timing-feed integration, CV/number/livery recognition, video, sales/marketplace/commerce beyond the pilot SKU, SSO/SCIM, and DAM/distribution connectors (Bynder, AEM, PhotoShelter, social push). The domain model (§5) is built to *accommodate* these (nullable enrichment layer, session-state embargo hook, tier model), but no MVP code implements them. Discipline here is the difference between shipping in 6 months and dying in 14.

---

## 9. Sequencing summary

```
Phase 0  Foundation ──┬─▶ Phase 1 Identity/RLS ──▶ Phase 2 Setup-from-templates ──┐
                      └─▶ Phase 3 Storage/async backbone ───────────────────────┐ │
                                                                                 ▼ ▼
                                                              Phase 4 Ingest (FTP + pipeline)
                                                                          │
                                            Phase 5 Galleries/curation/search
                                                                          │
                                            Phase 6 Entitlement tiers + watermarking
                                                                          │
                                            Phase 7 Embargo engine v1
                                                                          │
                                            Phase 8 Delivery + licence + audit
                                                                          │
                                            Phase 9 Admin & billing (pilot SKU)
                                                                          │
                                            Phase 10 Virtual Race Weekend harness  ← MVP gate
```

Phases 1 and 3 can proceed in parallel after Phase 0; Phase 4 needs both. Everything from 5 onward is a linear chain because each depends on the asset/rights state established before it.
