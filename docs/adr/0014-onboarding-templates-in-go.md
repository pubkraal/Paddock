# ADR-0014: Onboarding Templates Live in Go, Not the Database

**Status**: Accepted
**Date**: 2026-06-17

## Context

Phase 2's barrier-to-entry feature is same-day setup: a press officer picks an onboarding template — *sprint weekend*, *endurance*, or *rally* — and the system scaffolds the event's session structure (Free Practice, Qualifying, Race, Podium, Paddock, …) in one click, so an event goes from nothing to ready-for-media in minutes (PLAN §6).

A template is a named, ordered set of sessions, each of a typed kind, that gets materialised into real `sessions` rows under the chosen event. The question is *where the template definitions live*: as seed rows in a database table (e.g. `session_templates` / `session_template_items`), or as a versioned registry in Go.

The deciding properties: a template is **behaviour** (which sessions, in what order, of what type) that must be covered by tests; it evolves under code review like any other logic; and it must not become a second source of truth that drifts from the code that consumes it.

## Decision

Onboarding templates are an in-code registry in `internal/catalog` — a slice of `OnboardingTemplate{Key, Label, Blurb, Sessions []SessionSpec}` exposed via `Templates()` and `TemplateByKey(key)`. Creating an event from a template reads the registry and inserts the corresponding `sessions` rows (each carrying `org_id`, `event_id`, `type`, `name`, `ordinal`) inside the event-creation transaction.

The three MVP templates:

- **`sprint`** — Free Practice · Qualifying · Race · Race · Podium.
- **`endurance`** — Pre-Qualifying · Top-30 Qualifying · Warm-Up · Race (24h) · Podium · Paddock / Atmosphere.
- **`rally`** — placeholder: a single stage / road-section session, explicitly marked as a stub to be fleshed out when rally is a real target.

Session kinds are the `session_type` enum (`practice`, `qualifying`, `race`, `warmup`, `podium`, `paddock`); the registry maps each template entry to one of these. The registry is pure data + pure functions, so the scaffolding is exhaustively table-test-covered: each template asserts its exact session set, types, and ordinals.

The database stores only the *result* — the materialised `sessions` rows — never the template definitions.

## Alternatives Considered

### Template definitions as seed rows in dedicated tables

**Pros:**
- Editable without a deploy; a future "custom template builder" UI would write to the same tables.
- Templates are queryable like any other data.

**Cons:**
- Creates a second source of truth: the scaffolding logic in code and the template rows in the DB must agree, and a seed migration becomes load-bearing behaviour that ordinary tests don't exercise.
- Template correctness can only be checked with a database in the loop, pushing what is really unit-level behaviour into the integration tier.
- Seed rows drift across environments (dev vs CI vs prod) unless carefully migration-managed, and a template is RLS-tenant-scoped or global — an awkward modelling question for data that is really product configuration.

**Why rejected**: templates are product behaviour, not tenant data. Keeping them in code keeps them under review, fast-tested, and single-sourced. A custom-template-builder is a future, additive concern that can introduce per-org template rows *then*, without retrofitting the built-in set into the database now.

## Consequences

### Positive

- Template scaffolding is verified by fast, table-driven unit tests with no database — each template's exact output is pinned.
- One source of truth: the code that defines a template is the code that materialises it.
- Adding or revising a template is a reviewed code change with a test, not a data migration.

### Negative

- Changing the built-in templates requires a deploy (acceptable: they change rarely and are product decisions).
- Per-org custom templates are not modelled in MVP; supporting them later is an additive change (a tenant-scoped template table consulted alongside the built-in registry), not a rewrite.

### Neutral

- Materialised `sessions` rows are ordinary tenant data under RLS; only the definitions are code.
</content>
