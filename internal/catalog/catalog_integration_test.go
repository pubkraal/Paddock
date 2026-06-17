//go:build integration

package catalog_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/catalog"
)

// createEvent scaffolds a full championship→season→venue→event with the sprint
// template's sessions under one org, returning the event id.
func createEvent(t *testing.T, repo *catalog.Repository, orgID, name string) string {
	t.Helper()

	var eventID string

	err := repo.WithOrg(context.Background(), orgID, func(ctx context.Context, tx *sql.Tx) error {
		champID, err := repo.InsertChampionshipTx(ctx, tx, orgID, "NLS")
		if err != nil {
			return err
		}

		seasonID, err := repo.InsertSeasonTx(ctx, tx, orgID, champID, 2027, "2027")
		if err != nil {
			return err
		}

		venueID, err := repo.InsertVenueTx(ctx, tx, orgID, "Nürburgring", "nordschleife")
		if err != nil {
			return err
		}

		tmpl, err := catalog.TemplateByKey("sprint")
		if err != nil {
			return err
		}

		ev, err := repo.InsertEventTx(ctx, tx, orgID, seasonID, venueID, name, time.Time{}, time.Time{})
		if err != nil {
			return err
		}

		eventID = ev.ID

		return repo.InsertSessionsTx(ctx, tx, orgID, eventID, tmpl.Sessions)
	})
	if err != nil {
		t.Fatalf("createEvent(%s): %v", orgID, err)
	}

	return eventID
}

func TestIntegration_EventsAreOrgScoped(t *testing.T) {
	t.Parallel()

	appPool, migrateDB := startCatalogDB(t)
	seedOrg(t, migrateDB, orgA, "Northern Lights", "series", "eu-central-1")
	seedOrg(t, migrateDB, orgB, "Veldhoven Racing", "team", "eu-west-1")

	repo := catalog.NewRepository(appPool)

	eventA := createEvent(t, repo, orgA, "24H Nürburgring")
	_ = createEvent(t, repo, orgB, "Veldhoven Sprint")

	// Org A sees only its own event; org B's is invisible at the engine level.
	var aEvents []catalog.Event

	if err := repo.WithOrg(context.Background(), orgA, func(ctx context.Context, tx *sql.Tx) error {
		var e error
		aEvents, e = repo.ListEventsTx(ctx, tx)

		return e
	}); err != nil {
		t.Fatalf("list A: %v", err)
	}

	if len(aEvents) != 1 || aEvents[0].ID != eventA {
		t.Fatalf("org A sees %d events, want exactly its own", len(aEvents))
	}

	// Org A's sessions are present (template scaffolded them); org B cannot read
	// org A's event at all.
	if err := repo.WithOrg(context.Background(), orgA, func(ctx context.Context, tx *sql.Tx) error {
		sessions, e := repo.ListSessionsTx(ctx, tx, eventA)
		if e != nil {
			return e
		}

		if len(sessions) != 5 {
			t.Errorf("org A event has %d sessions, want 5 (sprint)", len(sessions))
		}

		return nil
	}); err != nil {
		t.Fatalf("list A sessions: %v", err)
	}

	if err := repo.WithOrg(context.Background(), orgB, func(ctx context.Context, tx *sql.Tx) error {
		if _, e := repo.GetEventTx(ctx, tx, eventA); !errors.Is(e, catalog.ErrEventNotFound) {
			t.Errorf("org B reading org A event = %v, want ErrEventNotFound", e)
		}

		return nil
	}); err != nil {
		t.Fatalf("org B get A: %v", err)
	}
}

func TestIntegration_CrossTenantInsertRejected(t *testing.T) {
	t.Parallel()

	appPool, migrateDB := startCatalogDB(t)
	seedOrg(t, migrateDB, orgA, "Northern Lights", "series", "eu-central-1")
	seedOrg(t, migrateDB, orgB, "Veldhoven Racing", "team", "eu-west-1")

	repo := catalog.NewRepository(appPool)

	// Under scope A, inserting a championship whose org_id is B violates the
	// WITH CHECK policy and is rejected.
	err := repo.WithOrg(context.Background(), orgA, func(ctx context.Context, tx *sql.Tx) error {
		_, e := repo.InsertChampionshipTx(ctx, tx, orgB, "Sneaky")

		return e
	})
	if err == nil {
		t.Fatal("cross-tenant championship insert succeeded, want RLS rejection")
	}
}

func TestIntegration_NoScopeFailsClosed(t *testing.T) {
	t.Parallel()

	appPool, migrateDB := startCatalogDB(t)
	seedOrg(t, migrateDB, orgA, "Northern Lights", "series", "eu-central-1")

	repo := catalog.NewRepository(appPool)
	_ = createEvent(t, repo, orgA, "24H Nürburgring")

	// A bare query with no app.current_org GUC set returns zero rows, not an
	// error — RLS fails closed (current_setting(..., true) → NULL).
	var n int
	if err := appPool.SQL().QueryRowContext(context.Background(), "SELECT count(*) FROM events").Scan(&n); err != nil {
		t.Fatalf("unscoped count: %v", err)
	}

	if n != 0 {
		t.Errorf("unscoped event count = %d, want 0 (fail closed)", n)
	}
}
