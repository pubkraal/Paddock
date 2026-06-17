-- 0006_catalog — the Championship → Season → Event → Session hierarchy (PLAN §5).
--
-- An event is created from an onboarding template (ADR-0014): the template
-- scaffolds the sessions row set. session_type is the typed kind that later
-- drives session badges/filters. status moves draft → live on "Go live".
--
-- Every table is org-scoped under RLS with the per-command policy pattern from
-- 0002/0003 (FORCE RLS, no DELETE policy, fail-closed current_setting). FKs
-- cascade down the hierarchy; org_id is denormalised onto every level so RLS is
-- a single-column predicate rather than a join.

CREATE TYPE session_type AS ENUM (
    'practice', 'qualifying', 'race', 'warmup', 'podium', 'paddock'
);

CREATE TABLE championships (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX championships_org_id_idx ON championships (org_id);

CREATE TABLE seasons (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    championship_id uuid NOT NULL REFERENCES championships (id) ON DELETE CASCADE,
    year            int NOT NULL,
    name            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX seasons_org_id_idx ON seasons (org_id);
CREATE INDEX seasons_championship_id_idx ON seasons (championship_id);

CREATE TABLE events (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    season_id  uuid NOT NULL REFERENCES seasons (id) ON DELETE CASCADE,
    venue_id   uuid REFERENCES venues (id) ON DELETE SET NULL,
    name       text NOT NULL,
    starts_on  date,
    ends_on    date,
    status     text NOT NULL DEFAULT 'draft',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX events_org_id_idx ON events (org_id);
CREATE INDEX events_season_id_idx ON events (season_id);
CREATE INDEX events_venue_id_idx ON events (venue_id);

CREATE TABLE sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_id   uuid NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    type       session_type NOT NULL,
    name       text NOT NULL,
    ordinal    int NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_org_id_idx ON sessions (org_id);
CREATE INDEX sessions_event_id_idx ON sessions (event_id);

ALTER TABLE championships ENABLE ROW LEVEL SECURITY;
ALTER TABLE championships FORCE ROW LEVEL SECURITY;
ALTER TABLE seasons ENABLE ROW LEVEL SECURITY;
ALTER TABLE seasons FORCE ROW LEVEL SECURITY;
ALTER TABLE events ENABLE ROW LEVEL SECURITY;
ALTER TABLE events FORCE ROW LEVEL SECURITY;
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;

CREATE POLICY championships_org_isolation_select ON championships
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY championships_org_isolation_insert ON championships
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY championships_org_isolation_update ON championships
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);

CREATE POLICY seasons_org_isolation_select ON seasons
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY seasons_org_isolation_insert ON seasons
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY seasons_org_isolation_update ON seasons
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);

CREATE POLICY events_org_isolation_select ON events
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY events_org_isolation_insert ON events
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY events_org_isolation_update ON events
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);

CREATE POLICY sessions_org_isolation_select ON sessions
    FOR SELECT USING (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY sessions_org_isolation_insert ON sessions
    FOR INSERT WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
CREATE POLICY sessions_org_isolation_update ON sessions
    FOR UPDATE USING (org_id = current_setting('app.current_org', true)::uuid)
    WITH CHECK (org_id = current_setting('app.current_org', true)::uuid);
