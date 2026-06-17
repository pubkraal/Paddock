# ADR-0008: Multitenancy via Postgres Row-Level Security from Day One

**Status**: Accepted
**Date**: 2026-06-16

## Context

Paddock is a multi-tenant platform. Every organization (series, promoter, circuit, team) is a distinct tenant; their assets, events, entitlements, and licence records must be completely isolated from every other tenant. This is not only a data privacy requirement but a commercial and legal one: a sponsor or journalist from Series A must never see assets belonging to Series B.

Multitenancy isolation must hold even in the presence of application bugs. If a handler accidentally omits a `WHERE org_id = ?` clause, the consequence must not be a cross-tenant data leak.

The team will grow; not every engineer will be intimately familiar with the multitenancy model. The isolation mechanism must not rely solely on every engineer remembering to add a WHERE clause on every query.

## Decision

Every tenant-owned table carries an `org_id` column. **Postgres Row-Level Security (RLS) policies** enforce tenant isolation at the database engine level, driven by a per-transaction session parameter (a GUC — Grand Unified Configuration variable) set at the start of each database transaction:

```sql
SET LOCAL app.current_org = '<org-uuid>';
```

RLS policies on each table reference this GUC:

```sql
CREATE POLICY org_isolation ON assets
    USING (org_id = current_setting('app.current_org')::uuid);
```

Application code sets this GUC in a transaction wrapper before executing any queries. If the GUC is not set, the RLS policy causes the query to return no rows (or fail, depending on policy configuration) rather than returning cross-tenant data.

## Alternatives Considered

### Shared schema, application-layer-only scoping (WHERE org_id = ?)

**Pros:**
- Simpler to implement initially; no Postgres RLS configuration.
- Standard pattern in many web applications.
- Works with all ORM and query libraries without special handling.

**Cons:**
- Isolation depends entirely on every query at every call site including the correct `WHERE org_id = ?` clause. One missed clause leaks cross-tenant data.
- As the codebase grows and engineers are added, the probability of a missed clause increases. Code review cannot catch every instance.
- Application-layer isolation cannot be tested comprehensively without running every query path against a live database with multiple tenant fixtures.
- The failure mode (a missing WHERE clause) is silent: the query succeeds and returns wrong data. There is no error to catch.

**Why rejected**: The failure mode is a silent cross-tenant data leak. For a platform where tenants include competing commercial organizations and personal data is stored, this is an unacceptable risk. Defense in depth requires the isolation to be enforced at a layer the application cannot accidentally bypass.

### Schema-per-tenant (one Postgres schema per organization)

**Pros:**
- Isolation is structural: cross-tenant queries are impossible because the schemas are separate objects.
- Postgres's `search_path` is the isolation mechanism; no per-query clause needed.

**Cons:**
- Schema migrations must be applied to every tenant schema: `N tenants × M migrations = N×M operations`. At scale (hundreds of pilot events with single-event orgs) this fan-out becomes a significant operational burden.
- Connection routing must map each request to the correct schema, adding a layer of middleware that must be correct for every request.
- Tenant provisioning requires DDL execution (CREATE SCHEMA, CREATE TABLE × N); this is slower and more error-prone than inserting a row.
- The "run your first event this afternoon" self-serve model requires near-instant tenant provisioning; DDL fan-out does not meet that bar.
- Connection pooling (PgBouncer) does not play well with session-level `search_path` settings; transaction-mode pooling breaks schema-per-tenant patterns.

**Why rejected**: Migration fan-out and connection-routing complexity are disproportionate. The self-serve, single-event pilot model requires that creating a new organization be as fast as inserting a row, not as slow as creating a schema and running every migration.

## Consequences

### Positive

- Cross-tenant isolation is enforced by the Postgres engine on every query, regardless of whether the application code remembers to scope it. A missing `WHERE org_id` clause returns empty results, not cross-tenant rows.
- The isolation mechanism is testable at the database layer: integration tests can verify that a query issued with `org_id = A` cannot see rows belonging to `org_id = B`, even if the query has no explicit WHERE clause.
- Tenant provisioning is an `INSERT` into the organizations table; no DDL required.
- The RLS policies and GUC pattern are documented once and apply uniformly to every tenant-owned table; application code handles isolation via the transaction wrapper, not via per-query clauses.

### Negative

- Every database transaction must begin with `SET LOCAL app.current_org = ?` before issuing queries. The transaction wrapper that does this must be used consistently; a raw `db.Query` call that bypasses the wrapper will see no rows (or will fail if the GUC is unset and the policy is configured to error). This requires discipline and clear documentation.
- `go-sqlmock` (used in unit tests per CLAUDE.md) regex-matches SQL strings; it cannot verify that RLS policies actually isolate tenants, because sqlmock does not execute policies. RLS correctness must be tested in integration tests against real Postgres (see ADR-0009).
- Postgres's RLS adds a small overhead to every query (policy evaluation). At the query volumes expected in MVP this is negligible, but it is not zero.
- Engineers unfamiliar with Postgres RLS need onboarding; the pattern is powerful but non-standard in Go web applications.

### Neutral

- Superuser connections (used in migrations) bypass RLS. The `golang-migrate` connection must use a dedicated migration role, not the application role, to ensure migrations can write to all tenant rows. The application role must have RLS enabled and must never be a superuser.

## Addendum (2026-06-17): FORCE ROW LEVEL SECURITY is the house default

Every tenant-owned table is created with both `ENABLE` and `FORCE ROW LEVEL SECURITY`. Standard RLS already applies to `paddock_app` because it is not the table owner (the migration role owns the tables), so `FORCE` changes nothing today. It is added anyway as belt-and-suspenders: should anyone ever run application queries as the owning role, or grant ownership by accident, `FORCE` keeps the policies in effect. It is free, makes the intent explicit in the migration, and removes a latent foot-gun. The one sanctioned cross-tenant read — the login bootstrap — is handled by an explicit `SECURITY DEFINER` function, not by relaxing RLS (see ADR-0012).

Policies use the `missing_ok` form of `current_setting` — `current_setting('app.current_org', true)` — so an unscoped query (no GUC set) evaluates the predicate against `NULL` and sees **zero rows**, rather than raising `unrecognized configuration parameter`. This makes "fail closed" a graceful empty result on reads and a rejected `WITH CHECK` on writes, which is what the integration tier asserts.

### Update (2026-06-17): coalesce the empty-string reset to NULL

`SET LOCAL` (which `WithOrg` uses via `set_config(..., is_local => true)`) does not revert a *custom* GUC to NULL after the transaction commits — on a pooled connection it reverts to the **empty string**. So a query that runs without `WithOrg` on a connection that previously served a scoped transaction reads `current_setting('app.current_org', true) = ''`, and a bare `''::uuid` cast raises `invalid input syntax for type uuid` instead of failing closed to zero rows. No data ever leaked (the cast errored rather than returning other tenants' rows), but an error is not the *graceful* fail-closed this ADR promises. Every policy therefore wraps the read as `NULLIF(current_setting('app.current_org', true), '')::uuid`: empty or unset both become `NULL` → zero rows. Phase 2 tables (migrations 0005–0008) ship with this form; migration 0009 backfills the Phase 1 `organizations` and `users` policies. The integration tier asserts an unscoped read on a reused connection returns zero rows.
