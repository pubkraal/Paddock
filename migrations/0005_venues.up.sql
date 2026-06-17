-- 0005_venues — circuits/venues, the FK target events reference.
--
-- Zones (corner / marshal-post zones, PLAN §5) are stored as jsonb and are
-- data-only in this phase: captured now, surfaced for tagging in a later phase.
-- Like every tenant table the venue is org-scoped under RLS with the per-command
-- policy pattern established in 0002/0003 (FORCE RLS house default, no DELETE
-- policy → least privilege; current_setting(..., true) fails closed when unset).

CREATE TABLE venues (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text NOT NULL,
    circuit_map_ref text,
    zones           jsonb NOT NULL DEFAULT '[]',
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX venues_org_id_idx ON venues (org_id);

ALTER TABLE venues ENABLE ROW LEVEL SECURITY;
ALTER TABLE venues FORCE ROW LEVEL SECURITY;

CREATE POLICY venues_org_isolation_select ON venues
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);

CREATE POLICY venues_org_isolation_insert ON venues
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);

CREATE POLICY venues_org_isolation_update ON venues
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
