# ADR-0009: Data Access — pgx via database/sql Adapter, Hand-Written SQL, Two-Tier Testing

**Status**: Accepted
**Date**: 2026-06-16

## Context

The data access layer must reconcile three constraints:

1. **CLAUDE.md house style**: unit tests use `go-sqlmock`; 100% coverage is required; the standard `database/sql` interface is the assumed database abstraction.
2. **RLS correctness (ADR-0008)**: Row-Level Security policies can only be proven to work against a real Postgres engine. `go-sqlmock` regex-matches SQL strings; it has no concept of RLS policies, GUCs, or tenant isolation. A unit test cannot verify that a query issued under `org_id = A` cannot see rows belonging to `org_id = B`.
3. **Query expressiveness**: the domain model (session-window queries, IPTC metadata, timestamp correlation, entitlement lookups, embargo-state filtering) requires SQL that is expressive enough to be written and read by engineers without being obscured by a code-generation layer.

These constraints point in different directions: sqlmock wants `database/sql`-compatible code; RLS needs real Postgres; expressiveness favors hand-written SQL.

## Decision

We will use **pgx** (`github.com/jackc/pgx/v5`) as the Postgres driver, configured via its **`database/sql`-compatible adapter** (`pgx/v5/stdlib`). Queries are **hand-written SQL** — no ORM, no code generation. Schema migrations and RLS policy DDL are managed by **golang-migrate**.

Testing is split into two tiers:

**Tier 1 — Unit tests (go-sqlmock):**
Handler and service logic that exercises the repository interface is unit-tested with `go-sqlmock`. These tests cover query construction, result mapping, and error handling at speed, without a real database.

**Tier 2 — Integration tests (testcontainers-go):**
Repository implementations are tested against a real Postgres container (via `testcontainers-go`) spun up per test package. These tests verify:
- RLS policies actually isolate tenants (a query under org A cannot return org B's rows, even without a WHERE clause).
- The `SET LOCAL app.current_org` GUC is correctly applied and respected.
- Queries produce correct results against the real schema, including constraints and indexes.
- Migrations apply cleanly and idempotently.

The `database/sql` interface boundary means the same repository struct works against both `go-sqlmock` (in unit tests) and a real `*sql.DB` backed by pgx (in integration tests).

## Alternatives Considered

### sqlc (SQL-to-Go code generation)

**Pros:**
- Type-safe Go functions generated from annotated SQL; no runtime query string construction.
- SQL is written by the engineer and reviewed, but executed via generated code with correct types.
- Good developer experience for greenfield schema.

**Cons:**
- Generated code does not fit the `go-sqlmock` unit-test pattern: sqlc generates specific function signatures that do not implement a hand-written repository interface, making it harder to swap in a mock.
- The generated code is not under the engineer's direct control; subtle behavior (null handling, array scanning) is determined by the generator version.
- Adding RLS-aware transaction wrapping requires either patching the generated code or adding a layer on top, which partially defeats the purpose of code generation.

**Why rejected**: Does not fit the sqlmock unit-test house style. The interface-mock pattern requires explicit repository interfaces that hand-written code fits naturally; code-generated functions do not.

### go-sqlmock only (no integration tests)

**Pros:**
- Consistent with the existing unit-test pattern; no testcontainers dependency.
- Fast: no container startup overhead.
- 100% coverage achievable without a real database.

**Cons:**
- sqlmock regex-matches SQL; it does not execute Postgres query planning, constraints, or — critically — RLS policies. A test that passes against sqlmock cannot prove that `SET LOCAL app.current_org = 'org-A'` actually prevents reading org-B rows.
- Tenant isolation correctness is the single most important security invariant of the platform (ADR-0008). Leaving it effectively untested is not acceptable.
- Schema migrations and index behavior are invisible to sqlmock; a query that is correct in syntax but wrong for the schema (wrong column name, missing index causing full-table scans) passes unit tests but fails or degrades in production.

**Why rejected**: RLS correctness cannot be verified by sqlmock. The isolation invariant is too important to leave to only unit tests.

### An ORM (GORM, ent)

**Pros:**
- Reduces boilerplate for common CRUD operations.
- Schema migrations can be driven from the ORM model.

**Cons:**
- ORMs abstract SQL in ways that make complex queries (session-window lookups, multi-join enrichment queries, embargo-state filtering with per-tier visibility rules) harder to write correctly and review.
- Generated SQL from an ORM may not use indexes optimally; debugging requires understanding both the ORM's query construction and the underlying SQL.
- The `database/sql` interface compatibility varies by ORM; GORM uses its own `*gorm.DB` type, which is not interchangeable with `*sql.DB` in tests.
- RLS GUC injection requires hooking into the ORM's transaction lifecycle, which is ORM-specific and fragile.

**Why rejected**: The expressiveness cost is too high for the complex queries the domain requires. Hand-written SQL is readable, reviewable, and directly portable between engineers.

## Consequences

### Positive

- The `database/sql` adapter for pgx means all repository code works against both `go-sqlmock` (unit tests) and a real Postgres connection (integration tests) without any branching.
- Hand-written SQL is explicit, reviewable, and can be optimized with `EXPLAIN ANALYZE` directly. Queries are not hidden behind a code-generation or abstraction layer.
- Integration tests with testcontainers-go verify RLS isolation with real Postgres policies; this is the only way to prove the isolation invariant holds.
- golang-migrate manages schema versioning; migration files are plain SQL, reviewable by anyone with Postgres knowledge, and applied deterministically in CI.
- pgx provides excellent Postgres-specific type support (UUID, JSONB, arrays, `pgtype` package) while still exposing the standard `database/sql` interface when needed.

### Negative

- Two test tiers mean two test environments. Integration tests are slower than unit tests (container startup: typically 2–5 seconds per package in CI). The test suite must be structured so unit tests can be run without Docker for fast local iteration.
- testcontainers-go requires Docker in CI. CI runners must have Docker available (standard for most CI providers but an explicit dependency).
- Hand-written SQL is more verbose than ORM-generated CRUD; repository layer code will be larger than an ORM equivalent. This is accepted as the cost of expressiveness and review clarity.
- Engineers must be proficient in SQL and understand the RLS/GUC pattern; there is no ORM to abstract these concerns.

### Neutral

- The split between unit and integration tests reflects the split between "does the application logic work?" (unit) and "does the database enforce isolation correctly?" (integration). This distinction is healthy and clarifies what each test tier is actually verifying.
