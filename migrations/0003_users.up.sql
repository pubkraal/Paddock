-- 0003_users — admin and consumer accounts, org-scoped under RLS.
--
-- Roles: the three admin roles (press_officer, season_admin, finance) plus
-- consumer (accredited media granted scoped, single-use magic-link access).
-- There are no passwords (ADR-0013): authentication is magic-link only.
--
-- Email is GLOBALLY unique, not per-org: at login the user types only their
-- email and we must resolve a single org deterministically before any tenant
-- scope exists (ADR-0012). Multi-org-per-email is a future additive change.

CREATE TYPE user_role AS ENUM ('press_officer', 'season_admin', 'finance', 'consumer');
CREATE TYPE user_status AS ENUM ('active', 'disabled');

CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email      citext NOT NULL UNIQUE,
    role       user_role NOT NULL,
    status     user_status NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX users_org_id_idx ON users (org_id);

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

-- Split per command for least privilege (see 0002): read/insert/update only
-- within the caller's org; no FOR DELETE policy, so the app role cannot delete
-- users. current_setting(..., true) yields NULL when unset → zero rows, fails
-- closed.
CREATE POLICY org_isolation_select ON users
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);

CREATE POLICY org_isolation_insert ON users
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);

CREATE POLICY org_isolation_update ON users
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
