-- Restore the single-column FKs and drop the composite (id, org_id) keys.

ALTER TABLE accreditations DROP CONSTRAINT accreditations_event_org_fkey;
ALTER TABLE accreditations ADD CONSTRAINT accreditations_event_id_fkey
    FOREIGN KEY (event_id) REFERENCES events (id) ON DELETE CASCADE;

ALTER TABLE entries DROP CONSTRAINT entries_entry_list_org_fkey;
ALTER TABLE entries ADD CONSTRAINT entries_entry_list_id_fkey
    FOREIGN KEY (entry_list_id) REFERENCES entry_lists (id) ON DELETE CASCADE;

ALTER TABLE entry_lists DROP CONSTRAINT entry_lists_event_org_fkey;
ALTER TABLE entry_lists ADD CONSTRAINT entry_lists_event_id_fkey
    FOREIGN KEY (event_id) REFERENCES events (id) ON DELETE CASCADE;

ALTER TABLE sessions DROP CONSTRAINT sessions_event_org_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_event_id_fkey
    FOREIGN KEY (event_id) REFERENCES events (id) ON DELETE CASCADE;

ALTER TABLE events DROP CONSTRAINT events_venue_org_fkey;
ALTER TABLE events ADD CONSTRAINT events_venue_id_fkey
    FOREIGN KEY (venue_id) REFERENCES venues (id) ON DELETE SET NULL;

ALTER TABLE events DROP CONSTRAINT events_season_org_fkey;
ALTER TABLE events ADD CONSTRAINT events_season_id_fkey
    FOREIGN KEY (season_id) REFERENCES seasons (id) ON DELETE CASCADE;

ALTER TABLE seasons DROP CONSTRAINT seasons_championship_org_fkey;
ALTER TABLE seasons ADD CONSTRAINT seasons_championship_id_fkey
    FOREIGN KEY (championship_id) REFERENCES championships (id) ON DELETE CASCADE;

ALTER TABLE entry_lists DROP CONSTRAINT entry_lists_id_org_key;
ALTER TABLE events DROP CONSTRAINT events_id_org_key;
ALTER TABLE venues DROP CONSTRAINT venues_id_org_key;
ALTER TABLE seasons DROP CONSTRAINT seasons_id_org_key;
ALTER TABLE championships DROP CONSTRAINT championships_id_org_key;
