-- 0001_init — Paddock baseline.
--
-- Tenancy convention (ADR-0008): every tenant-owned table carries an `org_id`
-- and is protected by an RLS policy that reads the per-transaction GUC
--
--     SET LOCAL app.current_org = '<org-uuid>';
--
-- The ONLY sanctioned way to set that GUC is the postgres.WithOrg helper, which
-- opens a transaction, sets it, and runs the caller's work inside it. When the
-- GUC is unset, tenant-scoped policies evaluate to false and queries see no
-- rows — a missing scope fails closed, never leaking across tenants.
--
-- This baseline migration intentionally creates no tenant tables yet (Phase 1
-- introduces organizations and the first RLS policies). It establishes the
-- extensions every later migration relies on. River owns its own tables via its
-- migrator (see `make migrate`); they are not duplicated here.

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- gen_random_uuid(), digest()
CREATE EXTENSION IF NOT EXISTS citext;   -- case-insensitive email identifiers
