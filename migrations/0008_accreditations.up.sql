-- 0008_accreditations — accredited people per event (PLAN §5: person → tier →
-- validity window → credential ref) and the bridge to their consumer account.
--
-- Importing an accreditation roster provisions a consumer user per person and
-- enqueues a magic-link invite (ADR-0016). user_id links the accreditation to
-- that account; ON DELETE SET NULL keeps the historical accreditation if the
-- account is later removed. tier is the design's four bands. email is unique per
-- event so re-import dedupes and does not double-invite.
--
-- Org-scoped under RLS with the per-command policy pattern (FORCE, no DELETE,
-- fail-closed) from 0002/0003.

CREATE TYPE accreditation_tier AS ENUM ('media', 'sponsor', 'team', 'internal');

CREATE TABLE accreditations (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id       uuid NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    user_id        uuid REFERENCES users (id) ON DELETE SET NULL,
    person_name    text NOT NULL,
    email          citext NOT NULL,
    tier           accreditation_tier NOT NULL,
    valid_from     date,
    valid_to       date,
    credential_ref text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_id, email)
);

CREATE INDEX accreditations_org_id_idx ON accreditations (org_id);
CREATE INDEX accreditations_event_id_idx ON accreditations (event_id);
CREATE INDEX accreditations_user_id_idx ON accreditations (user_id);

ALTER TABLE accreditations ENABLE ROW LEVEL SECURITY;
ALTER TABLE accreditations FORCE ROW LEVEL SECURITY;

CREATE POLICY accreditations_org_isolation_select ON accreditations
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY accreditations_org_isolation_insert ON accreditations
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY accreditations_org_isolation_update ON accreditations
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
