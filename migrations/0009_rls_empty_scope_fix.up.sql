-- 0009_rls_empty_scope_fix — harden the RLS scope predicate against an empty
-- (not NULL) app.current_org.
--
-- A custom GUC set with SET LOCAL inside a transaction does NOT revert to NULL
-- after commit on a pooled connection — it reverts to the EMPTY STRING. So a
-- query that runs WITHOUT WithOrg on a connection that previously served a
-- scoped transaction sees current_setting('app.current_org', true) = '' rather
-- than NULL, and the bare ''::uuid cast raises "invalid input syntax for type
-- uuid" instead of failing closed to zero rows.
--
-- The fix is to coalesce '' to NULL before the cast: NULLIF(current_setting(
-- 'app.current_org', true), '')::uuid. NULL then makes the predicate NULL → the
-- row is filtered → zero rows, gracefully. This recreates the organizations and
-- users policies from 0002/0003 in the hardened form; the Phase 2 tables
-- (0005-0008) already use it. No data leak existed before (the cast errored
-- rather than returning other tenants' rows) — this restores the graceful
-- fail-closed behaviour ADR-0008 promises.

DROP POLICY IF EXISTS org_self_select ON organizations;
DROP POLICY IF EXISTS org_self_insert ON organizations;
DROP POLICY IF EXISTS org_self_update ON organizations;

CREATE POLICY org_self_select ON organizations
    FOR SELECT USING (id = NULLIF(current_setting('app.current_org', true), '')::uuid);
CREATE POLICY org_self_insert ON organizations
    FOR INSERT WITH CHECK (id = NULLIF(current_setting('app.current_org', true), '')::uuid);
CREATE POLICY org_self_update ON organizations
    FOR UPDATE USING (id = NULLIF(current_setting('app.current_org', true), '')::uuid)
    WITH CHECK (id = NULLIF(current_setting('app.current_org', true), '')::uuid);

DROP POLICY IF EXISTS org_isolation_select ON users;
DROP POLICY IF EXISTS org_isolation_insert ON users;
DROP POLICY IF EXISTS org_isolation_update ON users;

CREATE POLICY org_isolation_select ON users
    FOR SELECT USING (org_id = NULLIF(current_setting('app.current_org', true), '')::uuid);
CREATE POLICY org_isolation_insert ON users
    FOR INSERT WITH CHECK (org_id = NULLIF(current_setting('app.current_org', true), '')::uuid);
CREATE POLICY org_isolation_update ON users
    FOR UPDATE USING (org_id = NULLIF(current_setting('app.current_org', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('app.current_org', true), '')::uuid);
