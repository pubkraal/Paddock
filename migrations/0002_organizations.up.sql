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

CREATE POLICY org_self ON organizations
    USING (id = current_setting('app.current_org')::uuid)
    WITH CHECK (id = current_setting('app.current_org')::uuid);
