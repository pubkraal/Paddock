//go:build integration

package catalog_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pubkraal/paddock/internal/catalog"
	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/platform/tabular"
)

// countingEnqueuer satisfies the service's invite-enqueuer dependency without a
// real River client; it just tallies enqueues (the transactional-enqueue path
// itself is unit-covered in internal/invite).
type countingEnqueuer struct{ n int }

func (c *countingEnqueuer) EnqueueInviteTx(context.Context, *sql.Tx, string, string, string) error {
	c.n++

	return nil
}

func TestIntegration_ImportAccreditationProvisionsScopedConsumers(t *testing.T) {
	t.Parallel()

	appPool, migrateDB := startCatalogDB(t)
	seedOrg(t, migrateDB, orgA, "Northern Lights", "series", "eu-central-1")
	seedOrg(t, migrateDB, orgB, "Veldhoven Racing", "team", "eu-west-1")

	repo := catalog.NewRepository(appPool)
	idRepo := identity.NewRepository(appPool)
	enq := &countingEnqueuer{}
	svc := catalog.NewService(repo, idRepo, enq)

	eventA := createEvent(t, repo, orgA, "24H Nürburgring")

	sheet := tabular.Sheet{
		Header: []string{"Name", "Email", "Tier"},
		Rows: [][]string{
			{"S. Bauer", "s.bauer@nls.test", "media"},
			{"P. Iredi", "p.iredi@pirelli.test", "sponsor"},
			{"Bad", "not-an-email", "media"},
		},
	}

	result, err := svc.ImportAccreditation(context.Background(), orgA, eventA, sheet)
	if err != nil {
		t.Fatalf("ImportAccreditation: %v", err)
	}

	if result.Invited != 2 || enq.n != 2 {
		t.Errorf("invited = %d enqueued = %d, want 2/2", result.Invited, enq.n)
	}

	if len(result.Errors) != 1 {
		t.Errorf("row errors = %d, want 1 (bad email)", len(result.Errors))
	}

	// Re-import is idempotent: same roster provisions nothing new, invites none.
	again, err := svc.ImportAccreditation(context.Background(), orgA, eventA, sheet)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}

	if again.Invited != 0 || enq.n != 2 {
		t.Errorf("re-import invited = %d enqueued total = %d, want 0/2", again.Invited, enq.n)
	}

	// The provisioned consumers are org A users and resolve via the login lookup.
	user, err := idRepo.Lookup(context.Background(), "s.bauer@nls.test")
	if err != nil {
		t.Fatalf("lookup provisioned consumer: %v", err)
	}

	if user.OrgID != orgA || user.Role != identity.RoleConsumer {
		t.Errorf("provisioned user = %+v, want org A consumer", user)
	}

	// Org B sees none of org A's accreditations.
	if err := repo.WithOrg(context.Background(), orgB, func(ctx context.Context, tx *sql.Tx) error {
		counts, e := repo.CountAccreditationsByTierTx(ctx, tx, eventA)
		if e != nil {
			return e
		}

		if len(counts) != 0 {
			t.Errorf("org B sees org A accreditations %+v, want none", counts)
		}

		return nil
	}); err != nil {
		t.Fatalf("org B count: %v", err)
	}
}
