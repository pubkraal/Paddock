-- 0007_entry_list — the imported race entry list (PLAN §5: Car № → Team →
-- Driver lineup → Class → livery refs).
--
-- One entry_list per import (carrying the source filename for the audit trail);
-- entries hang off it. Drivers and livery refs are text[] so a single-table
-- import stays cheap and the wizard preview is a flat read — the multi-driver
-- lineup is captured without a join table this phase. car_no is unique within
-- an entry_list so re-import dedupes on it.
--
-- Org-scoped under RLS with the per-command policy pattern (FORCE, no DELETE,
-- fail-closed) from 0002/0003.

CREATE TABLE entry_lists (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id        uuid NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    source_filename text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX entry_lists_org_id_idx ON entry_lists (org_id);
CREATE INDEX entry_lists_event_id_idx ON entry_lists (event_id);

CREATE TABLE entries (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    entry_list_id uuid NOT NULL REFERENCES entry_lists (id) ON DELETE CASCADE,
    car_no        text NOT NULL,
    team          text,
    class         text,
    drivers       text[] NOT NULL DEFAULT '{}',
    livery_refs   text[] NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entry_list_id, car_no)
);

CREATE INDEX entries_org_id_idx ON entries (org_id);
CREATE INDEX entries_entry_list_id_idx ON entries (entry_list_id);

ALTER TABLE entry_lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_lists FORCE ROW LEVEL SECURITY;
ALTER TABLE entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE entries FORCE ROW LEVEL SECURITY;

CREATE POLICY entry_lists_org_isolation_select ON entry_lists
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY entry_lists_org_isolation_insert ON entry_lists
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY entry_lists_org_isolation_update ON entry_lists
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);

CREATE POLICY entries_org_isolation_select ON entries
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY entries_org_isolation_insert ON entries
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY entries_org_isolation_update ON entries
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
