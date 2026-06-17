//go:build integration

package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pubkraal/paddock/internal/identity"
)

func TestIntegration_LookupResolvesAcrossTenantsWithoutScope(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)

	repo := identity.NewRepository(app)
	ctx := context.Background()

	// The SECURITY DEFINER lookup resolves a user in EITHER tenant with no
	// app.current_org set — exactly where a plain RLS SELECT returns zero rows.
	a, err := repo.Lookup(ctx, "a@series-a.test")
	if err != nil {
		t.Fatalf("Lookup A: %v", err)
	}

	if a.OrgID != orgA || a.Role != identity.RolePressOfficer {
		t.Errorf("lookup A = %+v, want org A press_officer", a)
	}

	b, err := repo.Lookup(ctx, "b@team-b.test")
	if err != nil {
		t.Fatalf("Lookup B: %v", err)
	}

	if b.OrgID != orgB || b.Role != identity.RoleSeasonAdmin {
		t.Errorf("lookup B = %+v, want org B season_admin", b)
	}

	// Case-insensitive email (citext) still resolves.
	if _, err := repo.Lookup(ctx, "A@SERIES-A.TEST"); err != nil {
		t.Errorf("case-insensitive Lookup: %v", err)
	}
}

func TestIntegration_LookupUnknownEmail(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)

	_, err := identity.NewRepository(app).Lookup(context.Background(), "nobody@nowhere.test")
	if !errors.Is(err, identity.ErrUserNotFound) {
		t.Errorf("Lookup unknown = %v, want ErrUserNotFound", err)
	}
}

func TestIntegration_OnlyTheFunctionBypassesRLS(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)
	ctx := context.Background()

	// In a single unscoped connection: the function resolves the user, but a
	// direct SELECT on users returns zero rows. Only the function bypasses.
	repo := identity.NewRepository(app)
	if _, err := repo.Lookup(ctx, "a@series-a.test"); err != nil {
		t.Fatalf("function lookup should succeed unscoped: %v", err)
	}

	var n int
	if err := app.SQL().QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("direct count: %v", err)
	}

	if n != 0 {
		t.Errorf("direct unscoped SELECT saw %d users, want 0 — only the function may bypass", n)
	}
}

func TestIntegration_GetUserWithinScope(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)
	ctx := context.Background()

	repo := identity.NewRepository(app)

	got, err := repo.GetUser(ctx, orgA, "a1111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	if got.Email != "a@series-a.test" || got.OrgID != orgA {
		t.Errorf("GetUser = %+v, want a@series-a.test in org A", got)
	}

	// B's user is invisible under A's scope — GetUser returns not-found.
	_, err = repo.GetUser(ctx, orgA, "b2222222-2222-2222-2222-222222222222")
	if !errors.Is(err, identity.ErrUserNotFound) {
		t.Errorf("cross-tenant GetUser = %v, want ErrUserNotFound", err)
	}
}
