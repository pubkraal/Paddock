// Command seed populates a dev database with two organizations and a
// season-admin for each, so a developer can sign in via Mailpit (ADR-0013).
// It connects as the migration role (DATABASE_URL) and is idempotent — there is
// no self-signup UI in Phase 1, so seeding is a dev tool, not a migration.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/pubkraal/paddock/internal/platform/postgres"
)

var errMissingDatabaseURL = errors.New("seed: DATABASE_URL is required (use the migration role)")

type orgSeed struct {
	ID         string
	Name       string
	Type       string
	Region     string
	AdminEmail string
}

var seeds = []orgSeed{
	{
		ID:         "0a000000-0000-4000-8000-000000000001",
		Name:       "Northern Lights Series",
		Type:       "series",
		Region:     "eu-central-1",
		AdminEmail: "press@nls-media.test",
	},
	{
		ID:         "0b000000-0000-4000-8000-000000000002",
		Name:       "Veldhoven Racing",
		Type:       "team",
		Region:     "eu-west-1",
		AdminEmail: "comms@veldhoven.test",
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

		fmt.Printf("seeded org %q (%s) — admin sign-in: %s\n", s.Name, s.Type, s.AdminEmail)
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
