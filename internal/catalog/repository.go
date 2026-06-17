package catalog

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pubkraal/paddock/internal/platform/postgres"
)

// Repository is the catalog data-access layer. Every operation is a tx-level
// method taking a *sql.Tx; callers run them inside WithOrg so RLS applies
// (ADR-0008) and multi-row work (an event and its sessions; an accreditation and
// its invite job) commits atomically (ADR-0016).
type Repository struct {
	pool *postgres.Pool
}

// NewRepository builds a Repository over the given pool.
func NewRepository(pool *postgres.Pool) *Repository {
	return &Repository{pool: pool}
}

// WithOrg runs fn in a transaction scoped to orgID. It is the only entry point
// for the tx-level methods below.
func (r *Repository) WithOrg(ctx context.Context, orgID string, fn func(context.Context, *sql.Tx) error) error {
	return r.pool.WithOrg(ctx, orgID, fn)
}

const insertChampionshipSQL = `
INSERT INTO championships (org_id, name) VALUES ($1, $2) RETURNING id`

// InsertChampionshipTx inserts a championship and returns its id.
func (r *Repository) InsertChampionshipTx(ctx context.Context, tx *sql.Tx, orgID, name string) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, insertChampionshipSQL, orgID, name).Scan(&id); err != nil {
		return "", fmt.Errorf("catalog: insert championship: %w", err)
	}

	return id, nil
}

const insertSeasonSQL = `
INSERT INTO seasons (org_id, championship_id, year, name) VALUES ($1, $2, $3, $4) RETURNING id`

// InsertSeasonTx inserts a season under a championship and returns its id.
func (r *Repository) InsertSeasonTx(
	ctx context.Context, tx *sql.Tx, orgID, championshipID string, year int, name string,
) (string, error) {
	var id string

	err := tx.QueryRowContext(ctx, insertSeasonSQL, orgID, championshipID, year, name).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("catalog: insert season: %w", err)
	}

	return id, nil
}

const insertVenueSQL = `
INSERT INTO venues (org_id, name, circuit_map_ref) VALUES ($1, $2, $3) RETURNING id`

// InsertVenueTx inserts a venue and returns its id.
func (r *Repository) InsertVenueTx(ctx context.Context, tx *sql.Tx, orgID, name, circuitMapRef string) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, insertVenueSQL, orgID, name, circuitMapRef).Scan(&id); err != nil {
		return "", fmt.Errorf("catalog: insert venue: %w", err)
	}

	return id, nil
}

const insertEventSQL = `
INSERT INTO events (org_id, season_id, venue_id, name, starts_on, ends_on, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, org_id, season_id, COALESCE(venue_id::text, ''), name, status, created_at`

// InsertEventTx inserts a draft event and returns the stored row. An empty
// venueID is stored as NULL.
func (r *Repository) InsertEventTx(
	ctx context.Context, tx *sql.Tx, orgID, seasonID, venueID, name string, startsOn, endsOn time.Time,
) (Event, error) {
	var (
		ev      Event
		created time.Time
	)

	err := tx.QueryRowContext(ctx, insertEventSQL,
		orgID, seasonID, nullString(venueID), name, nullDate(startsOn), nullDate(endsOn), string(EventDraft)).
		Scan(&ev.ID, &ev.OrgID, &ev.SeasonID, &ev.VenueID, &ev.Name, &ev.Status, &created)
	if err != nil {
		return Event{}, fmt.Errorf("catalog: insert event: %w", err)
	}

	ev.StartsOn = startsOn
	ev.EndsOn = endsOn
	ev.CreatedAt = created

	return ev, nil
}

const insertSessionSQL = `
INSERT INTO sessions (org_id, event_id, type, name, ordinal) VALUES ($1, $2, $3, $4, $5)`

// InsertSessionsTx inserts the template's sessions for an event, assigning
// ordinals positionally.
func (r *Repository) InsertSessionsTx(
	ctx context.Context, tx *sql.Tx, orgID, eventID string, specs []SessionSpec,
) error {
	for i, spec := range specs {
		_, err := tx.ExecContext(ctx, insertSessionSQL, orgID, eventID, string(spec.Type), spec.Name, i+1)
		if err != nil {
			return fmt.Errorf("catalog: insert session %d: %w", i, err)
		}
	}

	return nil
}

const setEventStatusSQL = `UPDATE events SET status = $2 WHERE id = $1`

// SetEventStatusTx moves an event to a new status (draft → live on Go live).
func (r *Repository) SetEventStatusTx(ctx context.Context, tx *sql.Tx, eventID string, status EventStatus) error {
	res, err := tx.ExecContext(ctx, setEventStatusSQL, eventID, string(status))
	if err != nil {
		return fmt.Errorf("catalog: set event status: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("catalog: set event status rows: %w", err)
	}

	if n == 0 {
		return ErrEventNotFound
	}

	return nil
}

const getEventSQL = `
SELECT id, org_id, season_id, COALESCE(venue_id::text, ''), name, status, created_at
FROM events WHERE id = $1`

// GetEventTx loads one event by id within scope.
func (r *Repository) GetEventTx(ctx context.Context, tx *sql.Tx, eventID string) (Event, error) {
	var (
		ev      Event
		created time.Time
	)

	err := tx.QueryRowContext(ctx, getEventSQL, eventID).
		Scan(&ev.ID, &ev.OrgID, &ev.SeasonID, &ev.VenueID, &ev.Name, &ev.Status, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, ErrEventNotFound
	}

	if err != nil {
		return Event{}, fmt.Errorf("catalog: get event: %w", err)
	}

	ev.CreatedAt = created

	return ev, nil
}

const listEventsSQL = `
SELECT id, org_id, season_id, COALESCE(venue_id::text, ''), name, status, created_at
FROM events ORDER BY created_at DESC`

// ListEventsTx returns all events in scope, newest first.
func (r *Repository) ListEventsTx(ctx context.Context, tx *sql.Tx) ([]Event, error) {
	rows, err := tx.QueryContext(ctx, listEventsSQL)
	if err != nil {
		return nil, fmt.Errorf("catalog: list events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []Event

	for rows.Next() {
		var (
			ev      Event
			created time.Time
		)

		if err := rows.Scan(&ev.ID, &ev.OrgID, &ev.SeasonID, &ev.VenueID, &ev.Name, &ev.Status, &created); err != nil {
			return nil, fmt.Errorf("catalog: scan event: %w", err)
		}

		ev.CreatedAt = created
		events = append(events, ev)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: list events rows: %w", err)
	}

	return events, nil
}

const listSessionsSQL = `
SELECT id, org_id, event_id, type, name, ordinal
FROM sessions WHERE event_id = $1 ORDER BY ordinal`

// ListSessionsTx returns an event's sessions in order.
func (r *Repository) ListSessionsTx(ctx context.Context, tx *sql.Tx, eventID string) ([]Session, error) {
	rows, err := tx.QueryContext(ctx, listSessionsSQL, eventID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sessions []Session

	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.OrgID, &s.EventID, &s.Type, &s.Name, &s.Ordinal); err != nil {
			return nil, fmt.Errorf("catalog: scan session: %w", err)
		}

		sessions = append(sessions, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: list sessions rows: %w", err)
	}

	return sessions, nil
}

const insertEntryListSQL = `
INSERT INTO entry_lists (org_id, event_id, source_filename) VALUES ($1, $2, $3) RETURNING id`

// InsertEntryListTx inserts an entry-list header and returns its id.
func (r *Repository) InsertEntryListTx(ctx context.Context, tx *sql.Tx, orgID, eventID, filename string) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, insertEntryListSQL, orgID, eventID, filename).Scan(&id); err != nil {
		return "", fmt.Errorf("catalog: insert entry list: %w", err)
	}

	return id, nil
}

const insertEntrySQL = `
INSERT INTO entries (org_id, entry_list_id, car_no, team, class, drivers, livery_refs)
VALUES ($1, $2, $3, $4, $5, $6, $7)`

// InsertEntriesTx inserts each entry under an entry list.
func (r *Repository) InsertEntriesTx(ctx context.Context, tx *sql.Tx, orgID, entryListID string, entries []Entry) error {
	for i, e := range entries {
		_, err := tx.ExecContext(ctx, insertEntrySQL,
			orgID, entryListID, e.CarNo, e.Team, e.Class, textArray(e.Drivers), textArray(e.LiveryRefs))
		if err != nil {
			return fmt.Errorf("catalog: insert entry %d: %w", i, err)
		}
	}

	return nil
}

const insertAccreditationSQL = `
INSERT INTO accreditations (org_id, event_id, user_id, person_name, email, tier, valid_from, valid_to, credential_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (event_id, email) DO NOTHING
RETURNING id`

// InsertAccreditationTx inserts an accreditation, deduping on (event_id, email).
// The returned bool is false when the row already existed (no insert, so no
// invite should be enqueued for it).
func (r *Repository) InsertAccreditationTx(ctx context.Context, tx *sql.Tx, a Accreditation) (string, bool, error) {
	var id string

	err := tx.QueryRowContext(ctx, insertAccreditationSQL,
		a.OrgID, a.EventID, nullString(a.UserID), a.PersonName, a.Email, string(a.Tier),
		nullDate(a.ValidFrom), nullDate(a.ValidTo), a.CredentialRef).
		Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}

	if err != nil {
		return "", false, fmt.Errorf("catalog: insert accreditation: %w", err)
	}

	return id, true, nil
}

const countEntriesSQL = `SELECT count(*) FROM entries e
JOIN entry_lists l ON l.id = e.entry_list_id WHERE l.event_id = $1`

// CountEntriesTx counts entries for an event (across its entry lists).
func (r *Repository) CountEntriesTx(ctx context.Context, tx *sql.Tx, eventID string) (int, error) {
	var n int
	if err := tx.QueryRowContext(ctx, countEntriesSQL, eventID).Scan(&n); err != nil {
		return 0, fmt.Errorf("catalog: count entries: %w", err)
	}

	return n, nil
}

const countAccreditationsByTierSQL = `
SELECT tier, count(*) FROM accreditations WHERE event_id = $1 GROUP BY tier`

// CountAccreditationsByTierTx returns per-tier accreditation counts for an event.
func (r *Repository) CountAccreditationsByTierTx(ctx context.Context, tx *sql.Tx, eventID string) (map[Tier]int, error) {
	rows, err := tx.QueryContext(ctx, countAccreditationsByTierSQL, eventID)
	if err != nil {
		return nil, fmt.Errorf("catalog: count accreditations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[Tier]int)

	for rows.Next() {
		var (
			tier string
			n    int
		)

		if err := rows.Scan(&tier, &n); err != nil {
			return nil, fmt.Errorf("catalog: scan accreditation count: %w", err)
		}

		counts[Tier(tier)] = n
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: count accreditations rows: %w", err)
	}

	return counts, nil
}

// nullString maps "" to a SQL NULL so optional FKs/text store NULL not empty.
func nullString(s string) any {
	if s == "" {
		return nil
	}

	return s
}

// nullDate maps the zero time to SQL NULL.
func nullDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}

	return t
}

// textArray encodes a string slice as a Postgres array literal so it is a valid
// driver.Value (a string) usable both under the pgx driver and sqlmock. database/
// sql rejects a bare []string; lib/pq-style literal building avoids that without
// a dependency.
type textArray []string

func (a textArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}

	var b strings.Builder

	b.WriteByte('{')

	escaper := strings.NewReplacer(`\`, `\\`, `"`, `\"`)

	for i, s := range a {
		if i > 0 {
			b.WriteByte(',')
		}

		b.WriteByte('"')
		b.WriteString(escaper.Replace(s))
		b.WriteByte('"')
	}

	b.WriteByte('}')

	return b.String(), nil
}
