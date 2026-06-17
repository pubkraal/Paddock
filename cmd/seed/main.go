// Command seed populates a dev database with two organizations, a season-admin
// for each, and a templated event so the dashboard is populated on first run
// (ADR-0013, ADR-0014). It connects as the migration role (DATABASE_URL) and is
// idempotent — there is no self-signup UI yet, so seeding is a dev tool, not a
// migration.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/pubkraal/paddock/internal/catalog"
	"github.com/pubkraal/paddock/internal/platform/postgres"
)

var errMissingDatabaseURL = errors.New("seed: DATABASE_URL is required (use the migration role)")

type orgSeed struct {
	ID         string
	Name       string
	Type       string
	Region     string
	AdminEmail string

	ChampionshipID   string
	SeasonID         string
	VenueID          string
	EventID          string
	ChampionshipName string
	VenueName        string
	EventName        string
	TemplateKey      string
}

var seeds = []orgSeed{
	{
		ID:               "0a000000-0000-4000-8000-000000000001",
		Name:             "Northern Lights Series",
		Type:             "series",
		Region:           "eu-central-1",
		AdminEmail:       "press@nls-media.test",
		ChampionshipID:   "0a000000-0000-4000-8000-0000000000c1",
		SeasonID:         "0a000000-0000-4000-8000-0000000000e1",
		VenueID:          "0a000000-0000-4000-8000-0000000000d1",
		EventID:          "0a000000-0000-4000-8000-0000000000f1",
		ChampionshipName: "Nürburgring Langstrecken-Serie",
		VenueName:        "Nürburgring Nordschleife",
		EventName:        "24H Nürburgring 2027",
		TemplateKey:      "endurance",
	},
	{
		ID:               "0b000000-0000-4000-8000-000000000002",
		Name:             "Veldhoven Racing",
		Type:             "team",
		Region:           "eu-west-1",
		AdminEmail:       "comms@veldhoven.test",
		ChampionshipID:   "0b000000-0000-4000-8000-0000000000c2",
		SeasonID:         "0b000000-0000-4000-8000-0000000000e2",
		VenueID:          "0b000000-0000-4000-8000-0000000000d2",
		EventID:          "0b000000-0000-4000-8000-0000000000f2",
		ChampionshipName: "GT Sprint Cup",
		VenueName:        "Circuit Zandvoort",
		EventName:        "Zandvoort Sprint 2027",
		TemplateKey:      "sprint",
	},
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := run(context.Background()); err != nil {
		logger.Error("seed failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errMissingDatabaseURL
	}

	pool, err := postgres.Open(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = pool.Close() }()

	db := pool.SQL()

	for _, s := range seeds {
		if err := seedOrg(ctx, db, s); err != nil {
			return fmt.Errorf("seed %s: %w", s.Name, err)
		}

		if err := seedEvent(ctx, db, s); err != nil {
			return fmt.Errorf("seed event %s: %w", s.EventName, err)
		}

		fmt.Printf("seeded org %q (%s) — event %q — admin sign-in: %s\n",
			s.Name, s.Type, s.EventName, s.AdminEmail)
	}

	return nil
}

func seedOrg(ctx context.Context, db *sql.DB, s orgSeed) error {
	const orgQ = `INSERT INTO organizations (id, name, type, region)
		VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`
	if _, err := db.ExecContext(ctx, orgQ, s.ID, s.Name, s.Type, s.Region); err != nil {
		return err
	}

	const userQ = `INSERT INTO users (org_id, email, role, status)
		VALUES ($1, $2, 'season_admin', 'active') ON CONFLICT (email) DO NOTHING`
	if _, err := db.ExecContext(ctx, userQ, s.ID, s.AdminEmail); err != nil {
		return err
	}

	return nil
}

// seedEvent scaffolds a templated event idempotently: the championship, season,
// venue and event use fixed ids (ON CONFLICT DO NOTHING); the template's sessions
// are inserted only when the event row is newly created, so re-running adds no
// duplicates.
func seedEvent(ctx context.Context, db *sql.DB, s orgSeed) error {
	tmpl, err := catalog.TemplateByKey(s.TemplateKey)
	if err != nil {
		return err
	}

	const champQ = `INSERT INTO championships (id, org_id, name)
		VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`
	if _, err := db.ExecContext(ctx, champQ, s.ChampionshipID, s.ID, s.ChampionshipName); err != nil {
		return err
	}

	const seasonQ = `INSERT INTO seasons (id, org_id, championship_id, year, name)
		VALUES ($1, $2, $3, 2027, '2027') ON CONFLICT (id) DO NOTHING`
	if _, err := db.ExecContext(ctx, seasonQ, s.SeasonID, s.ID, s.ChampionshipID); err != nil {
		return err
	}

	const venueQ = `INSERT INTO venues (id, org_id, name)
		VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`
	if _, err := db.ExecContext(ctx, venueQ, s.VenueID, s.ID, s.VenueName); err != nil {
		return err
	}

	const eventQ = `INSERT INTO events (id, org_id, season_id, venue_id, name, status)
		VALUES ($1, $2, $3, $4, $5, 'live') ON CONFLICT (id) DO NOTHING RETURNING id`

	var newID string

	err = db.QueryRowContext(ctx, eventQ, s.EventID, s.ID, s.SeasonID, s.VenueID, s.EventName).Scan(&newID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // event already seeded; leave its sessions as-is
	}

	if err != nil {
		return err
	}

	const sessionQ = `INSERT INTO sessions (org_id, event_id, type, name, ordinal)
		VALUES ($1, $2, $3, $4, $5)`
	for i, spec := range tmpl.Sessions {
		if _, err := db.ExecContext(ctx, sessionQ, s.ID, s.EventID, string(spec.Type), spec.Name, i+1); err != nil {
			return err
		}
	}

	return nil
}
