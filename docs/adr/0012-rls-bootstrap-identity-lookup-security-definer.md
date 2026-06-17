# ADR-0012: RLS-Bootstrap Identity Lookup via a SECURITY DEFINER Function

**Status**: Accepted
**Date**: 2026-06-17

## Context

ADR-0008 makes every tenant-owned table — including `organizations` and `users` — invisible unless the per-transaction GUC `app.current_org` is set, via the `postgres.WithOrg` helper. This is exactly what we want for every request *after* we know the tenant.

But authentication has a chicken-and-egg problem. At the start of a login the user types only their work email. We do not yet know which organization they belong to, so we cannot call `WithOrg(orgID, …)` — and without a scope, an RLS-protected `SELECT … FROM users WHERE email = $1` returns zero rows. The email → (user, org) resolution must happen *before* any tenant scope exists. This is the "RLS bootstrap" problem: exactly one lookup, by design, has to cross tenant boundaries.

The resolution must be: minimal (leak only what login needs), auditable (a named, greppable bypass — not an ambient capability), and testable under the two-role integration harness (ADR-0009) where the application connects as the non-superuser `paddock_app`.

## Decision

Resolve email → identity through a single Postgres `SECURITY DEFINER` function, owned by the migration role (`paddock_migrate`, which is `BYPASSRLS`), returning only the four columns login needs:

```sql
CREATE FUNCTION identity_lookup(p_email citext)
RETURNS TABLE (user_id uuid, org_id uuid, role user_role, status user_status)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp
AS $$ SELECT id, org_id, role, status FROM users WHERE email = p_email; $$;

REVOKE ALL ON FUNCTION identity_lookup(citext) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION identity_lookup(citext) TO paddock_app;
```

Because the function runs with the privileges of its owner (`paddock_migrate`), it sees across tenants by definition, while `EXECUTE` is granted only to `paddock_app`. `SET search_path` pins resolution so the definer-rights function cannot be hijacked by a caller-controlled search path. In Go, `Repository.Lookup` calls it on the bare pool (`pool.SQL().QueryRowContext`), explicitly **not** inside `WithOrg` — the one sanctioned place a query is unscoped. After redemption the `org_id` is carried in the magic-link token (ADR-0013), so every subsequent query is normally scoped via `WithOrg`.

**Email is globally unique** (`UNIQUE (email)` on `users`, not per-org) so the lookup resolves to exactly one organization deterministically — there is no "which org did you mean?" ambiguity at the passwordless front door.

**Anti-enumeration lives in the application, not the function.** The login handler always returns the same "if that email exists, a link is on its way" response with the same status and roughly constant time, whether or not the function returned a row, and refuses to issue links to non-`active` users. The function may freely return zero rows; the handler must never branch its visible response on existence.

## Alternatives Considered

### A global directory table (email → user_id, org_id) outside RLS

**Pros:**
- A plain unscoped `SELECT` resolves the login with no special database object.
- Conceptually simple.

**Cons:**
- Duplicates the email→org mapping that already lives in `users`, creating a second source of truth that must be kept in sync on every user insert/update/delete (triggers or application code).
- Drift between the directory and `users` is a place isolation can silently rot.

**Why rejected**: more write surface and a synchronization invariant, for no benefit the function does not already provide.

### A permissive RLS policy on `users` allowing a narrow email-keyed SELECT

**Pros:**
- No separate function or table.

**Cons:**
- Any unscoped `SELECT` on `users` would then return rows; getting the policy predicate to permit *exactly* an email-keyed single-row read and nothing else is fragile.
- A bug in that predicate widens directly into a full cross-tenant enumeration oracle on the most sensitive table.

**Why rejected**: it turns the user table into an ambient, always-on bypass whose blast radius on a mistake is the entire user base. The function is a single, explicit, `EXECUTE`-gated door instead.

## Consequences

### Positive

- The cross-tenant bypass is named, owned, column-limited, and `EXECUTE`-gated — easy to find (`grep identity_lookup`), easy to review, easy to test.
- Testable under the two-role harness: called as `paddock_app` with **no** org GUC, the function resolves a user in either tenant, while a direct unscoped `SELECT FROM users` in the same connection returns zero rows — proving only the function bypasses.
- No second source of truth; `users` remains canonical.

### Negative

- `SECURITY DEFINER` is a sharp tool; it must keep `SET search_path` and a minimal projection, and any change to it is a security-relevant change requiring review.
- One email maps to one organization. A human who is an admin in two orgs is not modelled in MVP; supporting it later is an additive change (the function returns a set → an org-picker screen), not a rewrite.

### Neutral

- The function is owned by `paddock_migrate` and therefore created/dropped by migrations, alongside the tables it reads.
