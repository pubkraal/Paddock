-- 0010_tenant_composite_fks — DB-level defense-in-depth against cross-tenant
-- references (the IDOR guard).
--
-- A plain FK like entry_lists.event_id -> events(id) is checked by a system RI
-- trigger that runs as the table owner and BYPASSES RLS. So a row with
-- org_id = the caller's own org but event_id = ANOTHER org's event satisfies
-- both the RLS WITH CHECK (org_id matches) and the FK (the foreign event is
-- visible to the bypassing check). The application now also verifies event
-- ownership via a scoped read, but we enforce the invariant in the schema too:
-- a child's org_id must equal its parent's org_id.
--
-- Each parent gets a UNIQUE (id, org_id) so it can be the target of a composite
-- FK, and each child's (parent_id, org_id) references it. A row whose org_id
-- does not match its parent's org_id now fails the FK regardless of RLS.

ALTER TABLE championships ADD CONSTRAINT championships_id_org_key UNIQUE (id, org_id);
ALTER TABLE seasons ADD CONSTRAINT seasons_id_org_key UNIQUE (id, org_id);
ALTER TABLE venues ADD CONSTRAINT venues_id_org_key UNIQUE (id, org_id);
ALTER TABLE events ADD CONSTRAINT events_id_org_key UNIQUE (id, org_id);
ALTER TABLE entry_lists ADD CONSTRAINT entry_lists_id_org_key UNIQUE (id, org_id);

ALTER TABLE seasons DROP CONSTRAINT seasons_championship_id_fkey;
ALTER TABLE seasons ADD CONSTRAINT seasons_championship_org_fkey
    FOREIGN KEY (championship_id, org_id) REFERENCES championships (id, org_id) ON DELETE CASCADE;

ALTER TABLE events DROP CONSTRAINT events_season_id_fkey;
ALTER TABLE events ADD CONSTRAINT events_season_org_fkey
    FOREIGN KEY (season_id, org_id) REFERENCES seasons (id, org_id) ON DELETE CASCADE;

ALTER TABLE events DROP CONSTRAINT events_venue_id_fkey;
ALTER TABLE events ADD CONSTRAINT events_venue_org_fkey
    FOREIGN KEY (venue_id, org_id) REFERENCES venues (id, org_id) ON DELETE SET NULL;

ALTER TABLE sessions DROP CONSTRAINT sessions_event_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_event_org_fkey
    FOREIGN KEY (event_id, org_id) REFERENCES events (id, org_id) ON DELETE CASCADE;

ALTER TABLE entry_lists DROP CONSTRAINT entry_lists_event_id_fkey;
ALTER TABLE entry_lists ADD CONSTRAINT entry_lists_event_org_fkey
    FOREIGN KEY (event_id, org_id) REFERENCES events (id, org_id) ON DELETE CASCADE;

ALTER TABLE entries DROP CONSTRAINT entries_entry_list_id_fkey;
ALTER TABLE entries ADD CONSTRAINT entries_entry_list_org_fkey
    FOREIGN KEY (entry_list_id, org_id) REFERENCES entry_lists (id, org_id) ON DELETE CASCADE;

ALTER TABLE accreditations DROP CONSTRAINT accreditations_event_id_fkey;
ALTER TABLE accreditations ADD CONSTRAINT accreditations_event_org_fkey
    FOREIGN KEY (event_id, org_id) REFERENCES events (id, org_id) ON DELETE CASCADE;
