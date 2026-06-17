-- Revert organizations and users policies to the pre-0009 bare-cast form.

DROP POLICY IF EXISTS org_self_select ON organizations;
DROP POLICY IF EXISTS org_self_insert ON organizations;
DROP POLICY IF EXISTS org_self_update ON organizations;

CREATE POLICY org_self_select ON organizations
    FOR SELECT USING (id = current_setting('app.current_org', true)::uuid);
CREATE POLICY org_self_insert ON organizations
    FOR INSERT WITH CHECK (id = current_setting('app.current_org', true)::uuid);
CREATE POLICY org_self_update ON organizations
    FOR UPDATE USING (id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (id = current_setting('app.current_org', true)::uuid);

DROP POLICY IF EXISTS org_isolation_select ON users;
DROP POLICY IF EXISTS org_isolation_insert ON users;
DROP POLICY IF EXISTS org_isolation_update ON users;

CREATE POLICY org_isolation_select ON users
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY org_isolation_insert ON users
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY org_isolation_update ON users
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
