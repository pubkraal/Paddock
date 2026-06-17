//go:build integration

package catalog_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pubkraal/paddock/internal/catalog"
)

func TestIntegration_EntriesAndAccreditationsOrgScoped(t *testing.T) {
	t.Parallel()

	appPool, migrateDB := startCatalogDB(t)
	seedOrg(t, migrateDB, orgA, "Northern Lights", "series", "eu-central-1")
	seedOrg(t, migrateDB, orgB, "Veldhoven Racing", "team", "eu-west-1")

	repo := catalog.NewRepository(appPool)
	eventA := createEvent(t, repo, orgA, "24H Nürburgring")

	// Import an entry list and two accreditations under org A.
	if err := repo.WithOrg(context.Background(), orgA, func(ctx context.Context, tx *sql.Tx) error {
		listID, err := repo.InsertEntryListTx(ctx, tx, orgA, eventA, "entrylist.csv")
		if err != nil {
			return err
		}

		entries := []catalog.Entry{
			{CarNo: "72", Team: "AMG Landgraf", Class: "SP9", Drivers: []string{"Stefan Mücke", `Max "Mad" Müller`}},
			{CarNo: "27", Team: "Lionspeed", Class: "GT3", LiveryRefs: []string{"livery-27"}},
		}
		if err := repo.InsertEntriesTx(ctx, tx, orgA, listID, entries); err != nil {
			return err
		}

		for _, a := range []catalog.Accreditation{
			{OrgID: orgA, EventID: eventA, PersonName: "S. Bauer", Email: "s.bauer@nls.test", Tier: catalog.TierMedia},
			{OrgID: orgA, EventID: eventA, PersonName: "P. Iredi", Email: "p.iredi@pirelli.test", Tier: catalog.TierSponsor},
		} {
			if _, _, err := repo.InsertAccreditationTx(ctx, tx, a); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		t.Fatalf("import under A: %v", err)
	}

	// Org A counts what it imported; the text[] driver round-trips UTF-8/escapes.
	if err := repo.WithOrg(context.Background(), orgA, func(ctx context.Context, tx *sql.Tx) error {
		n, err := repo.CountEntriesTx(ctx, tx, eventA)
		if err != nil {
			return err
		}

		if n != 2 {
			t.Errorf("org A entries = %d, want 2", n)
		}

		counts, err := repo.CountAccreditationsByTierTx(ctx, tx, eventA)
		if err != nil {
			return err
		}

		if counts[catalog.TierMedia] != 1 || counts[catalog.TierSponsor] != 1 {
			t.Errorf("org A tier counts = %+v, want 1 media + 1 sponsor", counts)
		}

		return nil
	}); err != nil {
		t.Fatalf("count under A: %v", err)
	}

	// Org B sees none of org A's rows (event id is A's, but RLS scopes by org).
	if err := repo.WithOrg(context.Background(), orgB, func(ctx context.Context, tx *sql.Tx) error {
		n, err := repo.CountEntriesTx(ctx, tx, eventA)
		if err != nil {
			return err
		}

		if n != 0 {
			t.Errorf("org B sees %d of org A's entries, want 0", n)
		}

		counts, err := repo.CountAccreditationsByTierTx(ctx, tx, eventA)
		if err != nil {
			return err
		}

		if len(counts) != 0 {
			t.Errorf("org B sees org A accreditations %+v, want none", counts)
		}

		return nil
	}); err != nil {
		t.Fatalf("count under B: %v", err)
	}

	// A cross-tenant entry-list insert (B's org_id under scope A) is rejected.
	err := repo.WithOrg(context.Background(), orgA, func(ctx context.Context, tx *sql.Tx) error {
		_, e := repo.InsertEntryListTx(ctx, tx, orgB, eventA, "sneaky.csv")

		return e
	})
	if err == nil {
		t.Fatal("cross-tenant entry-list insert succeeded, want RLS rejection")
	}
}

func TestIntegration_AccreditationDedupe(t *testing.T) {
	t.Parallel()

	appPool, migrateDB := startCatalogDB(t)
	seedOrg(t, migrateDB, orgA, "Northern Lights", "series", "eu-central-1")

	repo := catalog.NewRepository(appPool)
	eventA := createEvent(t, repo, orgA, "24H Nürburgring")

	acc := catalog.Accreditation{
		OrgID: orgA, EventID: eventA, PersonName: "S. Bauer",
		Email: "s.bauer@nls.test", Tier: catalog.TierMedia,
	}

	if err := repo.WithOrg(context.Background(), orgA, func(ctx context.Context, tx *sql.Tx) error {
		id, created, err := repo.InsertAccreditationTx(ctx, tx, acc)
		if err != nil {
			return err
		}

		if id == "" || !created {
			t.Errorf("first insert id=%q created=%v, want a new row", id, created)
		}

		// Re-import the same (event, email): deduped, no second row, no invite.
		id2, created2, err := repo.InsertAccreditationTx(ctx, tx, acc)
		if err != nil {
			return err
		}

		if id2 != "" || created2 {
			t.Errorf("duplicate insert id=%q created=%v, want empty/false", id2, created2)
		}

		return nil
	}); err != nil {
		t.Fatalf("dedupe under A: %v", err)
	}
}
