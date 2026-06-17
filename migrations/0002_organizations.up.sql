-- 0002_organizations — the first tenant table and the first RLS policy.
--
-- An organization IS a tenant. Its own row is therefore self-scoped: a session
-- scoped to org X (app.current_org = X) sees only org X's row. Cross-tenant
-- reads return zero rows at the engine level (ADR-0008), proven by the
-- integration tier.
--
-- FORCE ROW LEVEL SECURITY is the house default (ADR-0008 addendum): the app
-- role is not the table owner so standard RLS already applies, but FORCE keeps
-- the policy in effect even if queries were ever run as the owner.

CREATE TYPE org_type AS ENUM ('series', 'promoter', 'circuit', 'team', 'asn');

CREATE TABLE organizations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    type       org_type NOT NULL,
    region     text NOT NULL, -- data-residency region (PLAN §5)
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;

-- Policies are split per command for least privilege: the app role may read,
-- insert, and update only its own org row, and may NOT delete (no FOR DELETE
-- policy → deletes are denied/affect zero rows). A future phase that needs
-- deletion adds an explicit FOR DELETE policy. current_setting(..., true)
-- returns NULL when the GUC is unset, so an unscoped query sees zero rows rather
-- than erroring — fails closed, gracefully.
CREATE POLICY org_self_select ON organizations
    FOR SELECT USING (id = current_setting('app.current_org', true)::uuid);

CREATE POLICY org_self_insert ON organizations
    FOR INSERT WITH CHECK (id = current_setting('app.current_org', true)::uuid);

CREATE POLICY org_self_update ON organizations
    FOR UPDATE USING (id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (id = current_setting('app.current_org', true)::uuid);
