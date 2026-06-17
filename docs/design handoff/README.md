# Handoff: Paddock — Event Media Operations Platform

## Overview
Paddock is a motorsport-native media operations platform. A press officer sets up a race weekend, photographers push frames trackside, the press office curates and applies embargoes, and accredited journalists / sponsors / teams browse a branded per-event gallery and download assets under a licence.

Two surfaces:
1. **Admin / portal** — press-officer event setup, accreditation import, curation, embargo controls, entitlements, audit, billing. Structured forms + operational workflows.
2. **Media portal** — branded per-event gallery for journalists, sponsors, teams. Paginated image grids with lazy loading; licence-gated download.

**Guiding constraint:** the demo path *is* the selling path. A press officer must be able to run their first event the same day they sign up, and the UI must be demonstrable before backend features are complete. Favour same-day, self-serve, template-driven setup and graceful degradation (ingest must never block).

## About the design files
The files in this bundle are **design references created as HTML** (Design Components) — they show intended look and behaviour, not production code to ship. The task is to **recreate these designs in the target codebase** using its established patterns. The team writes **Go** with a **TDD house style**; expect the UI to be served by a Go backend (templating + a JS sprinkle, or a Go-served SPA — your call to fit the existing stack). Treat the HTML/JS here as a precise visual + interaction spec, not a code drop.

## Fidelity
**High-fidelity.** Final colours, typography, spacing, status language, and interactions are specified below and should be reproduced faithfully. The interactive prototype (`Paddock Prototype`) is the behavioural source of truth for the core ingest→curate→publish loop and the resumable uploader.

---

## Design tokens

### Colour — dark "Graphite" scheme (primary, used across admin + media)
| Token | Hex | Use |
|---|---|---|
| App background | `#0b0d10` | Outermost surface, portal background |
| Card surface | `#0e1013` | Panels, cards, toolbars |
| Border (strong) | `#20242b` | Card outlines |
| Border (mid) | `#1c2026` | Inner dividers, inputs |
| Border (hairline) | `#181c22` / `#131619` | Row separators |
| Text primary | `#e9e7e2` | Headings, key values (warm white — NOT pure white) |
| Text secondary | `#cfd3d9` | Body, table cells |
| Text muted | `#8b9099` | Supporting copy |
| Text faint | `#6b6e74` | Mono labels, captions |
| Text dim | `#5f656d` | Placeholders |
| Board ink | `#16181c` | Dark text on the light design-board background |

### Colour — light "Paper" scheme (optional, for daylight media-centre stations / editorial media portal)
| Token | Hex |
|---|---|
| Background | `#f4f2ec` |
| Card | `#fbfaf6` |
| Border | `#d6d2c7` / `#e4e1d8` |
| Text primary | `#16181c` |
Photo tiles stay dark in both schemes; only the chrome flips. Offer dark admin + (optionally) light media portal.

### Accent
| Token | Hex |
|---|---|
| Paddock red (primary accent) | `#e0452f` |
Used sparingly: primary buttons, live indicators, active nav, the single brand pop. One accent only — do not introduce additional brand colours.

### Status language — "the racing-flag system" (CRITICAL — this is the product's semantic spine)
| Status | Flag | Hex | Meaning |
|---|---|---|---|
| Published | Green | `#43b97a` | Live, cleared, downloadable |
| Select / Review | Yellow | `#e0a23f` | Curated, pending confirm |
| Embargo / Kill | Red | `#e0452f` | Held back or recalled |
| Sponsor | Blue | `#4d8fe0` | Commercial tier, category-cleared |
| Raw / unreviewed | Neutral | `#9aa0a8` | Ingested, awaiting a select |
| Session closed | Chequered | `#cfd3d9` | Complete / archived |
Badges render as a pill: status dot (6px) + uppercase mono label, on a 14–18% tint of the status colour. Reuse everywhere a state is shown (tiles, tables, inspectors).

### Typography
- **Archivo** (weights 400/500/600/700/800/900) — all UI text. Headings & big counts 700–800 with `letter-spacing:-.01em` to `-.02em`. Body 400–600.
- **JetBrains Mono** (400–700) — ALL data: filenames, car numbers (`№31`), timecodes, IDs, lap data, chunk counts, mono labels (uppercase, `letter-spacing:.06em–.12em`, 10–11px, colour `#6b6e74`).
- Scale: page/event titles 21–30px; section heads 16–19px; body 12.5–14px; mono labels 10–11px. Never rely on emoji.

### Radii, shadows, spacing
- Radius: cards `11–14px`, controls/buttons `7–9px`, tiles `6–9px`, pills `999px`, chunk cells `1.5px`.
- Card shadow: `0 24px 50px -24px rgba(0,0,0,.45)` (panels), `0 30px 60px -26px rgba(0,0,0,.5)` (hero screens).
- Layout: design-board frames are `1440px` wide; the interactive app shell is `1280×820`. Generous 18–34px padding inside cards; `gap`-based flex/grid throughout (no margin-based spacing).
- Image placeholders use a diagonal hatch: `repeating-linear-gradient(125deg,#14181e 0,#14181e 8px,#171c23 8px,#171c23 16px)` with `inset 0 0 50px rgba(0,0,0,.55)`. Replace with real renditions in production.

---

## Identity model (drives every entry point)
| Actor | Auth | Notes |
|---|---|---|
| **Press officer** | Email / SSO **seat** | The only real account. Owns events, curation, embargo, billing. |
| **Photographer** | Per-event **FTP credentials** + magic upload link | No seat. Issued at accreditation. Three ways to ingest (below). |
| **Journalist / Sponsor / Team** | **Magic link**, scoped to tier/category | No password — the link *is* the entitlement. |
| **Paddock (system)** | — | Auto enrichment, routing, embargo timers, audit. No human. |

## How a photographer ingests (three paths, one endpoint)
1. **In-camera FTP push** — pro bodies (R3/Z9/A1) push over trackside wifi/5G to the event FTP. JPEG previews live, raws follow.
2. **Laptop FTP (Photo Mechanic / any FTPS client)** — card reader → existing client → per-event hot-folder. Zero new software.
3. **Browser resumable upload** — drag RAWs into the portal; chunked + checksum-verified, auto-resumes after dropouts. The no-app path.

All converge on the ingest endpoint → checksum + dedupe → auto-route by session time + IPTC → car № matched from entry list → lands in the curation queue as RAW, enriched, in ≤60s.

**Entry points to the uploader (this was a key review point — make the "way in" visible):**
- Photographer lands on their dashboard → primary **↥ Upload photos** button in the header.
- Press officer → **Ingest** item in the left rail (accented) or **↥ Add photos** button in the dashboard header.

---

## Screens / Views
Two artefacts contain the screens:
- **`Paddock.dc.html`** — the full static design board: 31 screens grouped Foundations · Admin · Media · Access · Dashboards · Distribute, plus a clickable Contents index at the top.
- **`Paddock Prototype.dc.html`** — the interactive core loop (behavioural spec).

### Core admin screens
**Ingest & Curation (two approved layouts — build both as view modes, default A):**
- **Layout A · Triptych** — left: live ingest feed (FTP×3 + web, per-file status + progress); centre: 4-up curation grid with status-bordered tiles; right: inspector (large preview, metadata, Select/Publish/Embargo/Kill actions). Dense control-room default for methodical session review.
- **Layout C · Lightbox rapid-cull** — one large preview + EXIF strip, keyboard-first keep/cull (`P` publish, `S` select, `X` reject, `←/→` navigate), thin incoming filmstrip on the right. Fastest against a press-conference deadline.

**Event setup** — start from an Endurance/Sprint/Rally template (auto-generates 6 sessions, tiers, licence text, embargo presets). Import entry list (xlsx) + accreditation (csv, 4 tiers) with column mapping. "Go live" in ~6 min, same-day.

**Resumable upload** — see prototype spec below. Two routes in: browser drop (Select files / Select folder) + per-event FTP credentials panel (host/port/user/password/folder, "copy"/"reveal").

**Embargo control** — table of embargoes: manual / scheduled / session-triggered (e.g. "Chequered + 15m"). Tier behaviour per embargo. Key rule: **embargoed-but-visible** — media tier previews held frames with a red badge; sponsor & public tiers can't see them at all.

**Audit log** — licence-event log answering "who downloaded asset X": time, user, org, asset id, rendition, tier, licence version, IP. Stat strip (downloads, distinct outlets, embargo leaks=0, licence completeness=100%). CSV export.

Plus: Entry list, Entitlement tiers, Accreditation, Billing, and role dashboards (Press officer, Photographer, Sponsor, Team comms, Federation/Archive).

### Core media screens
**Media gallery (journalist)** — branded per-event header, filter bar (session/class/car/team/search), 4-up grid with lazy loading ("Showing 60 of 2,418 · loading more on scroll"). Tiles show car №, sponsor badge, hi-res label, download affordance. Embargoed tiles render blurred with "EMBARGOED · lifts <time>".

**Sponsor portal** — category-cleared assets only; competitor-prominent frames hidden by conflict rules; commercial watermark on every download; category-locked overlay where applicable.

**Asset detail + licence download** — rendition picker (Hi-res / Web), licence text (versioned, e.g. Media-Editorial v2.3), explicit accept checkbox, "Accept & download". On accept, write the audit event (who · what · when · which licence text · IP).

---

## Interactions & Behaviour — interactive prototype spec
`Paddock Prototype.dc.html`. App shell `1280×820`, dark scheme. Top bar: View-as toggle (Photographer / Press officer) + Reset. Shared state across roles.

### Navigation / state
- `role`: `photographer | press`
- Photographer screens: `dash | upload`
- Press screens: `dash | upload | curate | gallery` (left-rail nav)
- Switching role or Reset returns to that role's dashboard and clears the upload queue.
- Shared state is real: files uploaded as photographer raise the press officer's "Frames in"; publishing in curation populates the gallery instantly.

### Resumable uploader (behavioural source of truth)
- **Start:** Select files / Select folder enqueues files. Drop zone always visible.
- **Chunk model:** each file = N chunks (≈1 per MB; 28–80 in the demo). **1 MB chunks, up to 3 in parallel, checksum per chunk.**
- **Per-chunk visualisation:** a wrapping grid of 7px cells per file — verified=green `#43b97a`, in-flight=amber `#e0a23f` (next 3), pending=`#1c2026`. Counter reads "47 / 63 chunks".
- **States:** `UPLOADING` (amber) → `✓ INGESTED` (green). `❚❚ PAUSED` (grey) when paused. `● RECONNECTING` (red) on a connection blip.
- **Pause/Resume:** per-file button + "Pause all / Resume all". Paused files hold verified chunks and resume from the last verified chunk — never re-upload.
- **Reconnect blip:** auto-triggers mid-transfer (~45%) for one file, shows RECONNECTING for a few ticks, then resumes from last verified chunk. This demonstrates the trackside-dropout requirement — make it a first-class, demoable behaviour.
- **Completion:** success banner "All N files ingested · enriched and routed to curation in <60s"; CTA → curation (press) or dashboard (photographer). Transmitted counter increments per completed file.
- **Degradation:** ingest must never block — queue locally, publish later.

### Curation → publish
- Grid of assets; click a tile to toggle SELECT (published tiles are locked, not clickable). Status border + badge follow the racing-flag system.
- "Publish N selects" → flips selected to PUBLISHED, fires a confirmation toast, and they appear in the media gallery immediately ("what journalists see right now").

### Motion
Keep transitions subtle (pulse on live dots ~1.6s; chunk-cell fills tween ~0.2s). Avoid entrance animations that start at opacity 0 on large containers.

---

## State management (production)
- **Event** → Sessions, Entry list (car № → team/class/drivers), Accreditation (people → tier), Entitlement tiers, Licence texts (versioned), Embargoes.
- **Asset** → id, file, photographer (© retained), session, car №, drivers-on-stint, IPTC/XMP (preserved), EXIF, rendition set (hi-res/web), status (raw/select/published/embargoed/killed), tier visibility.
- **Upload** → file → chunks[] (offset, size, checksum, verified). Resume = server reports verified chunks; client sends only the gaps.
- **LicenceEvent (audit)** → user, org, asset, rendition, tier, licence version, timestamp, IP. Append-only.
- **Embargo** → scope, trigger (manual/scheduled/session), lift time, per-tier behaviour.

## Build notes
- **Go / TDD:** model the racing-flag status as a typed enum with explicit transitions (raw→select→published; published→killed/recall; embargo as an overlay with a lift trigger) — it's the spine, test the state machine hard.
- **Resumable upload:** implement a chunked protocol (e.g. tus or equivalent) — server tracks verified chunks by checksum; client resumes the gaps. The browser path must need no native app.
- **Identity:** magic links as scoped, signed, expiring tokens carrying tier/category — "the link is the entitlement." Photographer FTP creds are per-event and expire at event close.
- **Demo-path-first:** templates ship sessions/tiers/licence/embargo presets so a new event is live in minutes with no services configured. Every consumer-visible state must be demoable with seed data before the backend is complete.
- **Embargoed-but-visible** and **audit completeness** are differentiators — don't treat them as afterthoughts.

## Assets
No real imagery is included — tiles use a diagonal-hatch placeholder. Source real per-event photography for production; keep IPTC/XMP and © intact end-to-end. Logo is a simple chequered-glyph mark (recreate or replace with the final brand mark). Fonts: Archivo + JetBrains Mono (Google Fonts).

## Files
- `Paddock.dc.html` — full static design board (31 screens + Contents index). Design reference.
- `Paddock Prototype.dc.html` — interactive core loop (ingest → curate → publish; resumable uploader). Behavioural source of truth.
- `Paddock Prototype.html` — self-contained, offline-openable bundle of the prototype (same as the shared link).
- `support.js` — runtime for the `.dc.html` Design Components (needed to open the `.dc.html` files locally).

To view the `.dc.html` files, keep `support.js` alongside them and open in a browser. The `.html` bundle opens standalone.
