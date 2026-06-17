# ADR-0002: Server-Rendered UI — Go html/template + HTMX + tus-js-client

**Status**: Accepted
**Date**: 2026-06-16

## Context

Paddock needs two distinct UI surfaces:

1. **Admin/portal** — press-officer event setup, accreditation import, curation, embargo controls, billing; structured forms and workflows.
2. **Media portal** — branded per-event gallery for journalists, sponsors, and team comms; paginated image grids with lazy loading; asset download with licence acceptance.

A key product constraint is that a press officer must run their first event on the same day they sign up. The demo path is the selling path; the UI must be demonstrable before backend features are complete. The team writes Go and the codebase enforces a Go/TDD house style throughout.

The web upload flow must support resumable uploads (large raw files, unreliable trackside connectivity) without a native app.

## Decision

We will use **Go `html/template`** for server-side rendering with **HTMX** for in-page interactions and **tus-js-client** for resumable browser uploads. There will be no separate JavaScript framework, no SPA, and no frontend build pipeline.

Specific patterns:

- Pages are rendered by `cmd/web` Go handlers using `html/template`; the browser receives complete HTML.
- HTMX attributes on elements trigger partial-page updates (gallery pagination, live form validation, status polling) without full-page reloads and without a client-side routing layer.
- The tus protocol client (`tus-js-client`) handles resumable multi-file uploads directly in the browser; the server implements a tus endpoint in Go.
- Galleries are server-paginated; images lazy-load as the user scrolls. No client-side state store is required.

## Alternatives Considered

### React/TypeScript SPA

**Pros:**
- Rich interactivity without page reloads.
- Large ecosystem of UI component libraries.
- Familiar to many frontend engineers.

**Cons:**
- Requires a separate build pipeline (webpack/vite, node_modules, bundler config).
- Produces a second deploy artifact that must be versioned, hosted, and cache-invalidated independently of the Go binary.
- Introduces a non-Go test stack (Jest/Vitest/Playwright) alongside the Go test suite, fragmenting the CI pipeline.
- The API contract between the SPA and the Go backend must be formally versioned; any type mismatch surfaces only at runtime.
- Slower time to a demo-able product: the press-officer setup flow, which is the core demo, cannot be shown until both the API and the frontend are working.

**Why rejected**: The extra build/deploy complexity and second test stack contradict the house style and the "live in an afternoon" positioning. The incremental interactivity HTMX provides covers the MVP UI requirements without those costs.

### API-only backend (no server-rendered UI)

**Pros:**
- Maximum flexibility: any frontend (SPA, mobile, third-party) can consume the API.
- Clean separation of concerns.

**Cons:**
- The self-serve setup flow — onboarding, event creation from template, accreditation import, embargo configuration — is the core demo that sells the product. With an API-only backend there is nothing to show until a separate frontend exists.
- Press officers are not developers; a headless API is not a substitute for a working product UI.

**Why rejected**: The press-officer self-serve flow is simultaneously the adoption driver and the demo artefact. An API without a UI ships nothing demonstrable.

## Consequences

### Positive

- There is one deploy artifact (`cmd/web`) and one test suite (Go); no node_modules, no bundler config.
- Templates are rendered on the server and can be shown in a browser immediately; the first gallery page can be demoed before any JS interactivity is wired.
- HTMX's hypermedia model means UI interactions are expressed as HTTP exchanges; Go handlers return HTML fragments, which are fully testable with `net/http/httptest` and `html.Parse`.
- The tus server endpoint is a standard Go HTTP handler; resumable upload behaviour is tested in Go alongside all other handler behaviour.
- Gallery pagination and image lazy-loading are handled by the server and standard browser APIs respectively; no client-side routing or state management layer is needed.

### Negative

- Rich, stateful UI interactions (real-time collaborative editing, optimistic updates) require careful HTMX SSE/WebSocket patterns or are deferred to Y1+. The media portal is read-heavy and pagination-driven, which suits the server-render model well; the admin UI has more form complexity where this constraint will occasionally be felt.
- Engineers unfamiliar with the hypermedia/HTMX model may reach for JavaScript patterns that conflict with it; the pattern requires some onboarding.
- Template logic in Go's `html/template` is intentionally limited; complex rendering logic must live in Go functions passed to the template, which is the correct layering but requires discipline.

### Neutral

- When a public API is needed (Y1 CMS embeds, headless asset API), it is added as additional routes on `cmd/web` alongside the HTML routes; the server-render decision does not preclude an API.
