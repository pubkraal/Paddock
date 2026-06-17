# PADDOCK — Product & Application Brief
**Motorsport-native media operations platform**
Working title: *Paddock* (codename; trademark search pending)
Version 1.0 — June 2026 — CaaS Consultancy / Confidential

---

## 1. One-line definition

Paddock is the operational layer between the camera shutter and every downstream consumer of motorsport imagery — media, sponsors, teams, federations, and DAMs. It owns the race weekend: ingest at trackside speed, enrichment from official timing data, rights enforcement tied to accreditation, and instant distribution. It starts where Bynder, PhotoShelter, and Fotoware end.

## 2. Problem statement

Series organizers, promoters, circuits, and teams below the Getty-contract tier run race-weekend media operations on duct tape: WeTransfer links, SmugMug galleries, Dropbox folders, e-mail embargo notices, and a press officer manually forwarding ZIPs to 50 sponsors at 23:00 on Sunday. The consequences are measurable: content arrives after its commercial half-life expires, embargoes are enforced by hope, sponsor entitlements are honoured from memory, photographer copyright terms live in unread PDFs, and nobody can answer "who downloaded what under which licence."

Generic marketing DAMs (Bynder is the proof point — Sauber F1 uses it as a race-weekend "media hub") solve the *library* problem but not the *event* problem: no camera-FTP ingest, no timing-data awareness, no accreditation-tied access, no embargo automation, no licensing engine, seat-based pricing hostile to large external audiences, and AI (face recognition) that fails the moment a driver puts a helmet on.

## 3. Positioning

- **Category:** Event Media Operations Platform (EMOP) — deliberately not "DAM." We feed DAMs.
- **Against Bynder:** Bynder manages finished brand content for internal teams. Paddock runs live editorial operations for external audiences. Bynder is a downstream sync target, not a competitor we displace on day one — we coexist, then absorb budget.
- **Against PhotoShelter for Brands / Fotoware:** Closest functional neighbours (FTP ingest, galleries, media delivery). We beat them on motorsport-specific enrichment (timing correlation, car/number recognition), accreditation-driven entitlements, embargo automation, and per-event commercial model.
- **Against Orange Logic (Cortex):** We are not competing at IOC/FIFA scale in Y1–Y2. Cortex is a seven-figure, 9-month-implementation platform. Paddock is live in one afternoon. By Y3 we meet them at national-federation level from below.
- **Against Motorsport Images / agency model:** Complementary in principle (agencies need delivery rails too), competitive in practice where an organizer's agency contract bundles hosting. Treat as channel and threat simultaneously (see G2M brief).
- **Against the status quo (SmugMug + WeTransfer + e-mail):** The real competitor in 80% of deals. We win on time-to-publish, embargo enforcement, sponsor self-service, and audit trail.

## 4. Personas

| Persona | Role in product | Buying influence |
|---|---|---|
| **Press Officer / Media Delegate** (series or promoter) | Primary admin. Sets up events, manages accreditation tiers, curates selects, lifts embargoes, answers to the championship director. | Champion & daily user |
| **Accredited Photographer** | Supply side. Transmits via camera FTP / Photo Mechanic. Wants metadata respected, copyright clear, fast pipeline, ideally revenue. | Adoption gatekeeper & referral channel |
| **Sponsor / Partnerships Manager** (series and team side) | Self-service consumer. Wants "our branding, this weekend, commercial-cleared, now." | Renewal driver — sponsor delight is the renewal story |
| **Journalist / Editor** | Pull consumer. Wants selects before the press conference, hi-res, editorial licence, no account friction. | Usage volume, indirectly drives organizer NPS |
| **Team Comms Manager** | Both contributor and consumer. Mirror of the press officer at team scale. | Secondary buyer segment (Sauber pattern) |
| **Federation / ASN Archive Manager** | Long-tail consumer. Season archive, historic licensing, preservation. | Y2–Y3 expansion buyer |
| **Compliance / Legal** (organizer) | Reviews biometrics, data residency, processor terms. | Veto holder — neutralized by our compliance-by-design posture |

## 5. Domain model (the core IP)

The data model is the differentiator. Generic DAMs model folders and tags; Paddock models motorsport.

```
Organization (series / promoter / circuit / team / ASN)
 └─ Championship
     └─ Season
         └─ Event (e.g., "24H Nürburgring 2027")
             ├─ Sessions (FP1, Quali, Top-30 Shootout, Race, Podium, Paddock/Atmosphere)
             ├─ Entry List (Car № → Team → Driver lineup → Class → Livery refs)
             ├─ Accreditations (person → tier → validity window → credential ref)
             └─ Venue (circuit map, marshal-post / corner zones for location tagging)

Asset (photo, later video)
 ├─ Immutable source metadata (EXIF + photographer IPTC/XMP — never destroyed)
 ├─ Enrichment layer (session, car, drivers-on-stint, class, lap, corner, podium flag)
 ├─ Rights record (copyright holder, licence-back terms, usage class)
 └─ State (raw / select / published / embargoed / archived / killed)

Entitlement Tier (per organization, templated)
 ├─ Media-Editorial · Sponsor-Commercial(category) · Team · Internal · Public-Watermarked
 ├─ Resolution ceiling, watermark policy, licence text, embargo behaviour
 └─ Bound to accreditation records or named accounts

Embargo
 ├─ Manual, scheduled, or session-state-triggered ("lift at chequered flag + 15 min")
 └─ Per asset, per gallery, per tier (media may see embargoed; sponsors may not)

Licence Event (audit primitive)
 └─ Who downloaded which rendition, when, under which licence text version, from which IP
```

**Why this matters:** every Y1+ feature (timing correlation, auto-captioning, sponsor-category enforcement, embargo automation) is a function over this model. Get it right in MVP even where the UI doesn't yet expose it.

## 6. Existing-landscape integration map

Paddock must slot into what photographers and organizers already run. Integration is not a feature category — it is the adoption strategy.

**Upstream (ingest):**
| System | Integration | Phase |
|---|---|---|
| Camera-native FTP/FTPS/SFTP (Canon R1/R3, Nikon Z9/Z8, Sony A1/A9 III) | Per-photographer FTP endpoint with auto-routing to event/session | MVP |
| **Photo Mechanic** (Camera Bits) | FTP target preset; preserve IPTC stationery; publish per-event **code-replacement files** (entry list → `\car5\` → "№5 Phoenix Racing Audi R8 LMS GT3 evo II, Driver A/Driver B") generated automatically from the entry list | MVP (FTP) / Y1 (code-replacement generation) |
| Capture One / Lightroom Classic | FTP/hot-folder publish recipes, documented presets | MVP (docs) |
| Web uploader (drag-drop, resumable, tus-style) | For team comms and casual contributors | MVP |
| Watch-folder desktop agent | For media-centre ingest stations (the AP headless-Mac pattern) | Y1 |
| Mobile capture app (paddock/podium phone content) | Direct-to-event upload with auto session tagging | Y2 |
| Agency feeds (Getty delivery, Motorsport Images, DPPI, Panoramic) | Hot-folder / API ingest with source attribution | Y2 |

**Enrichment (the moat):**
| System | Integration | Phase |
|---|---|---|
| **Al Kamel Systems** (WEC, ELMS, GT World Challenge timing) | Live + post-session timing/classification ingest → session windows, car/driver stint table | Y1 |
| **TSL Timing** (UK: British GT, BTCC support series) | Same | Y1 |
| **MYLAPS Orbits / Speedhive** (club, karting, national) | Results + transponder data | Y1 |
| **RaceResult** | Results API | Y1 |
| **wige / DTM-style feeds, NLS timing (VLN)** | Per-deal adapters | Y2 |
| GPS/track-position feeds (where series provide) | Photographer-position + car-position correlation for corner-level auto-tagging | Y2–Y3 |

**Downstream (distribution & sync):**
| System | Integration | Phase |
|---|---|---|
| Expiring/branded share links, ZIP delivery | Native | MVP |
| **Bynder** | One-way publish connector (selects → Bynder collection with mapped metadata). Strategic: "keep Bynder for brand, Paddock runs the weekend" | Y1 |
| Adobe Experience Manager Assets / SharePoint / Google Drive | Publish connectors | Y1 |
| **PhotoShelter / Fotoware** | Publish connectors (migration on-ramps in disguise) | Y1–Y2 |
| Social presets (Meta Graph API, X, LinkedIn) — crops, watermarks, alt-text from captions | Push from selects queue | Y1 |
| CMS (WordPress, Contentful, Drupal) | Headless asset API + embed codes | Y1 (API) |
| **Greenfly-style driver/influencer push** | Direct-to-athlete delivery of cleared, branded assets | Y2 |
| Wire-style syndication feed (FTP-out per subscriber, the format newspapers already consume) | Outbound FTP per media subscriber | Y2 |
| **Accredit Solutions** and comparable accreditation platforms; FIA/ASN accreditation exports | Credential → entitlement provisioning/revocation; CSV import in MVP, API sync in Y1 | MVP (CSV) / Y1 (API) |
| SSO/SCIM (Entra ID, Google Workspace, Okta) | Organizer-side identity | Y1 |

## 7. Feature roadmap — MVP → Y1 → Y2 → Y3

Sales constraint applied throughout: **a press officer must run their first event the same day they sign up.** Every phase ships with onboarding templates (pre-built championship structures, entitlement-tier presets, licence-text library, embargo presets) so the product is demonstrably usable in a pilot without services.

### MVP (Months 0–6) — "Run one race weekend better than anyone"

**Scope (must-have, demo-able, billable):**

1. **Org/championship/event/session setup** from templates (sprint weekend, endurance, rally placeholder structure). Entry-list import (CSV/Excel — the format every series secretary already has).
2. **Ingest:** per-photographer FTP/SFTP endpoints with auto-routing rules (filename/IPTC/session-time based); resumable web upload; full IPTC/XMP preservation; duplicate detection; original always retained.
3. **Galleries & curation:** session-structured galleries; selects flagging; batch metadata templates (event/session/copyright applied on ingest); kill switch (asset recall with link invalidation).
4. **Entitlement tiers:** five presets (Media-Editorial, Sponsor-Commercial, Team, Internal, Public-Watermarked); per-tier resolution ceiling, watermark, licence text; accreditation import via CSV → bulk account provisioning with magic-link access (no password friction for journalists).
5. **Embargo engine v1:** manual + scheduled embargoes per gallery/tier; embargoed-but-visible mode for media tier.
6. **Watermarking:** per-tier overlay incl. sponsor-logo watermarks on public tier.
7. **Delivery:** branded media portal per event; expiring links; multi-asset ZIP; per-download **licence acceptance** with versioned licence text.
8. **Audit & compliance core:** complete licence-event log (who/what/when/which licence/IP); GDPR-clean processor posture; EU data residency; DPA template; retention rules.
9. **Search v1:** structured filters (event/session/car/team/driver/photographer/tag) + free text over captions.
10. **Admin & billing:** per-event and per-season billing, self-serve checkout for single-event pilots.

**Explicitly out of MVP:** timing integration, CV recognition, video, sales/marketplace, SSO, DAM connectors. Discipline here is the difference between shipping in 6 months and dying in 14.

**MVP acceptance test ("the Zandvoort Saturday test"):** a press officer with no training sets up an event from template, imports a 40-car entry list and a 120-person accreditation CSV, three photographers transmit 800 images via camera FTP during a 60-minute session, selects are published to media tier within 10 minutes of session end, a sponsor downloads category-cleared images self-service, embargo on podium shots lifts on schedule, and the audit log answers "who downloaded asset X" — all in one afternoon, on paddock-grade connectivity.

### Year 1 (Months 6–18) — "The metadata writes itself"

1. **Timing-feed enrichment v1:** Al Kamel, TSL, MYLAPS Orbits, RaceResult adapters. Session windows auto-create; entry-list sync; **timestamp correlation** assigns session and probable car/stint context to every frame; classification import enables "podium / class winner" auto-tags.
2. **CV recognition v1 — car-first, not face-first:** race-number and livery recognition trained per-event from entry-list reference images; confidence-scored, human-confirm queue. (Deliberate GDPR posture: no biometrics in the default pipeline.)
3. **Photo Mechanic deep support:** auto-generated code-replacement files per event; ingest profiles; documented "wire-style" workflow so existing pro muscle memory transfers in minutes.
4. **Accreditation API sync:** Accredit Solutions (and one ASN system per launch market) — credential approved → access provisioned; revoked → access killed. CSV remains the universal fallback.
5. **Live galleries:** session-live publishing (image visible to media tier ≤60 s after FTP receipt), pit-lane curation on tablet.
6. **Distribution connectors:** Bynder, AEM, SharePoint/Drive, PhotoShelter, Fotoware publish; social push presets; headless asset API + embeds.
7. **SSO/SCIM** for organizer staff; granular admin roles (press officer ≠ season admin ≠ finance).
8. **Analytics v1:** downloads by tier/sponsor/outlet, embargo compliance report, sponsor-value report (the renewal artefact: "your logo'd assets were downloaded 1,240 times by 38 outlets").
9. **Multi-photographer assignment:** shot-list briefs, coverage matrix (which cars/sponsors lack coverage — drives Sunday-morning assignments).

**Y1 acceptance test ("the Spa 24 test"):** 25,000 frames over four days, six photographers, timing feed live; ≥85% of frames carry correct session + car attribution with zero manual captioning; sponsor portal serves 60 partners self-service; Bynder connector syncs 300 selects to the organizer's existing brand DAM nightly; full audit trail; no ingest backlog exceeding 5 minutes at peak.

### Year 2 (Months 18–30) — "Money and video"

1. **Driver-stint auto-captioning:** timing stint table × timestamp → driver names in captions for endurance (the feature endurance press officers will evangelize unprompted).
2. **Photographer commerce:** optional sales module — print/digital sales to fans and participants, syndication pricing for media, automated rev-share and payout, photographer copyright retained with licence-back to organizer (contract templates included). Brings Sportpxl/GeoSnapShot-style economics to the professional tier and locks in the supply side.
3. **Video (MAM-lite):** clip ingest (broadcast turnaround clips, onboard exports), proxy playback, same entitlement/embargo engine, social-format rendering. Not a broadcast MAM — a rights-aware clip locker.
4. **Wire syndication out:** per-subscriber outbound FTP with IPTC mapping — newspapers receive Paddock content exactly like an agency feed.
5. **White-label portals & multi-org:** promoter runs N championships under one tenancy; circuit edition (Zandvoort/Nürburgring pattern: every event at the venue, trackday to GP support).
6. **Sponsor-category exclusivity engine:** category-tagged assets + conflict rules (a tyre sponsor never self-serves images featuring a competitor's branding prominence above threshold — CV-assisted, human-reviewed).
7. **Agency ingest:** Getty/Motorsport Images delivery intake with source-segregated rights handling.
8. **Mobile capture app**; **edge ingest node** pilot (on-site cache for circuits with hostile uplinks; store-and-forward).
9. **Compliance milestone:** ISO 27001 certification of the platform org (CaaS eats its own cooking), SOC 2 Type II underway, public trust centre. This is a *sales feature* at federation level.

### Year 3 (Months 30–42) — "From tool to infrastructure"

1. **Face/helmet recognition as opt-in module** with full consent management (driver/personnel consent registry, DPIA template, per-organizer enablement) — paddock and podium coverage completion, done lawfully. GDPR posture becomes the differentiator instead of the obstacle.
2. **Natural-language search** ("Verstappen-style: №31, podium, rain, 2027, sponsor-cleared") over the enriched index.
3. **Archive & preservation tier:** season/historic archive product for ASNs and federations — fixity, format migration policy, rights inheritance, public archive storefront. The Orange Logic flank attack from below.
4. **Position-correlated tagging:** GPS/track-feed × photographer-position → corner-level auto-tags ("Eau Rouge", "Karussell") where data partnerships permit.
5. **Discipline expansion:** rally mode (stage-based model, road-section assets, Rallye-specific timing — e.g., results from rally timing providers), bikes, historic events.
6. **Network features:** cross-organizer photographer profiles and availability marketplace (the photographer who shoots NLS also shoots GT Masters — one identity, N organizers); media-outlet identity across series (one journalist account, every accredited championship).
7. **Federation/Enterprise tier:** ASN-wide deployments (every national championship under the federation), procurement-grade security pack, data-residency options, 24/7 event support SLAs.

## 8. Non-functional requirements (sales-relevant subset)

- **Event-window availability:** 99.95% during declared event windows; status page; degradation mode = ingest never blocks (queue locally, publish later).
- **Burst targets:** MVP 1,000 images/hr/event sustained; Y1 10,000/hr; Y2 25,000/hr with ≤5-min publish backlog at P95.
- **Time-to-visible:** ≤60 s from FTP receipt to media-tier visibility (Y1 live galleries).
- **Residency & compliance:** EU hosting default; GDPR processor terms standard; biometrics off by default until Y3 module; ISO 27001 (Y2), SOC 2 Type II (Y2–Y3); full audit immutability.
- **Zero-friction access:** magic links for accredited media; no app install required for any consumer persona.

## 9. Verification & feature-completeness framework

Every feature ships with: (a) written acceptance criteria in the backlog item, (b) automated test coverage where applicable, (c) inclusion in the **Virtual Race Weekend harness** — a replayable simulation that injects a real anonymized event dataset (entry list, timing feed recording, 25k-image corpus with EXIF/IPTC) and asserts end-to-end outcomes: routing accuracy, enrichment precision/recall (car-attribution ≥85% precision Y1, ≥95% Y2), embargo enforcement (zero leaks across 10k simulated downloads), licence-event completeness (100%), publish-latency P95. Phase gates (MVP/Y1/Y2) are passed only when the harness run is green **and** a named pilot customer has completed the corresponding real-world acceptance test (Zandvoort Saturday / Spa 24). Feature-completeness per phase is therefore binary and externally verifiable, not a roadmap opinion.

## 10. Packaging (product view; commercials in G2M brief)

- **Event** — single event, self-serve, all MVP features, capped photographers/assets. The pilot SKU.
- **Championship** — per season: unlimited events in series, timing enrichment, connectors, analytics, unlimited external users (the anti-Bynder pricing statement).
- **Promoter/Circuit** — multi-championship tenancy, white-label, priority event support.
- **Federation/Enterprise** (Y2+) — ASN-wide, SSO/SCIM, archive tier, SLAs, security pack.
- **Add-ons:** commerce module (rev-share based), video, recognition modules, edge node.

## 11. Open product risks

1. **Timing-data access:** Al Kamel/TSL relationships are partnership-dependent; mitigation = results-file parsing (public formats) as fallback, partnership track in G2M.
2. **CV training data:** per-event livery training needs reference imagery; mitigation = entry-list photo requirement in event setup flow + transfer learning across seasons.
3. **Photographer trust:** any perception of rights-grab kills supply-side adoption; mitigation = copyright-retention defaults, public photographer terms, advisory group of working shooters from day one.
4. **Bynder reaction:** they could buy a niche player or build ingest; mitigation = speed, domain depth (timing integrations they won't prioritize), and supply-side lock-in via the photographer network.
