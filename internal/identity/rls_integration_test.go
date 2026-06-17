//go:build integration

package identity_test

import (
	"context"
	"database/sql"
	"testing"
)

// seedTwoOrgs sets up org A and org B, each with one user, as the migrate role.
func seedTwoOrgs(t *testing.T, migrate *sql.DB) {
	t.Helper()

	seedOrg(t, migrate, orgA, "Series A", "series", "eu-central-1")
	seedOrg(t, migrate, orgB, "Team B", "team", "eu-west-1")
	seedUser(t, migrate, "a1111111-1111-1111-1111-111111111111", orgA, "a@series-a.test", "press_officer", "active")
	seedUser(t, migrate, "b2222222-2222-2222-2222-222222222222", orgB, "b@team-b.test", "season_admin", "active")
}

func countUsers(t *testing.T, ctx context.Context, tx *sql.Tx) int {
	t.Helper()

	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}

	return n
}

func TestRLS_SelectIsolation(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)
	ctx := context.Background()

	// Scoped to A, the app role sees only A's single user — never B's.
	err := app.WithOrg(ctx, orgA, func(ctx context.Context, tx *sql.Tx) error {
		if n := countUsers(t, ctx, tx); n != 1 {
			t.Errorf("org A sees %d users, want 1 (zero cross-tenant rows)", n)
		}

		var email string
		if err := tx.QueryRowContext(ctx, `SELECT email FROM users`).Scan(&email); err != nil {
			return err
		}

		if email != "a@series-a.test" {
			t.Errorf("org A sees email %q, want a@series-a.test", email)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg A: %v", err)
	}

	// Symmetric: scoped to B, only B's user is visible.
	err = app.WithOrg(ctx, orgB, func(ctx context.Context, tx *sql.Tx) error {
		if n := countUsers(t, ctx, tx); n != 1 {
			t.Errorf("org B sees %d users, want 1", n)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg B: %v", err)
	}
}

func TestRLS_OrganizationSelfScope(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)
	ctx := context.Background()

	err := app.WithOrg(ctx, orgA, func(ctx context.Context, tx *sql.Tx) error {
		var (
			n    int
			name string
		)

		if err := tx.QueryRowContext(ctx, `SELECT count(*), max(name) FROM organizations`).Scan(&n, &name); err != nil {
			return err
		}

		if n != 1 || name != "Series A" {
			t.Errorf("org A sees %d orgs (%q), want exactly its own (Series A)", n, name)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg A: %v", err)
	}
}

func TestRLS_InsertIntoAnotherTenantRejected(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)
	ctx := context.Background()

	// Scoped to A, inserting a row carrying B's org_id must fail the WITH CHECK.
	err := app.WithOrg(ctx, orgA, func(ctx context.Context, tx *sql.Tx) error {
		const q = `INSERT INTO users (org_id, email, role, status) VALUES ($1, $2, 'consumer', 'active')`
		_, insErr := tx.ExecContext(ctx, q, orgB, "smuggled@series-a.test")

		return insErr
	})
	if err == nil {
		t.Fatal("expected RLS WITH CHECK to reject an insert into another tenant, got nil")
	}
}

func TestRLS_UpdateAcrossTenantAffectsZeroRows(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)
	ctx := context.Background()

	// Scoped to A, an UPDATE targeting B's user matches no visible row.
	err := app.WithOrg(ctx, orgA, func(ctx context.Context, tx *sql.Tx) error {
		res, execErr := tx.ExecContext(ctx,
			`UPDATE users SET status = 'disabled' WHERE id = $1`,
			"b2222222-2222-2222-2222-222222222222",
		)
		if execErr != nil {
			return execErr
		}

		n, _ := res.RowsAffected()
		if n != 0 {
			t.Errorf("update across tenant affected %d rows, want 0", n)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg A: %v", err)
	}

	// Confirm B's user is untouched, read under B's own scope.
	err = app.WithOrg(ctx, orgB, func(ctx context.Context, tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM users`).Scan(&status); err != nil {
			return err
		}

		if status != "active" {
			t.Errorf("B's user status = %q, want untouched 'active'", status)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg B: %v", err)
	}
}

func TestRLS_DeleteDeniedForAppRole(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)
	ctx := context.Background()

	// There is no FOR DELETE policy, so even within its own scope the app role
	// cannot delete its org or users — both DELETEs affect zero rows.
	err := app.WithOrg(ctx, orgA, func(ctx context.Context, tx *sql.Tx) error {
		users, execErr := tx.ExecContext(ctx, `DELETE FROM users`)
		if execErr != nil {
			return execErr
		}

		if n, _ := users.RowsAffected(); n != 0 {
			t.Errorf("app role deleted %d users, want 0 (no FOR DELETE policy)", n)
		}

		orgs, execErr := tx.ExecContext(ctx, `DELETE FROM organizations`)
		if execErr != nil {
			return execErr
		}

		if n, _ := orgs.RowsAffected(); n != 0 {
			t.Errorf("app role deleted %d orgs, want 0", n)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg A: %v", err)
	}

	// Confirm the rows still exist, read back under scope.
	err = app.WithOrg(ctx, orgA, func(ctx context.Context, tx *sql.Tx) error {
		if n := countUsers(t, ctx, tx); n != 1 {
			t.Errorf("after attempted delete, org A sees %d users, want 1", n)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg A recheck: %v", err)
	}
}

func TestRLS_NoScopeFailsClosed(t *testing.T) {
	t.Parallel()

	app, migrate := startIdentityDB(t)
	seedTwoOrgs(t, migrate)
	ctx := context.Background()

	// A plain transaction with no app.current_org set must see zero rows in
	// every tenant table — a missing scope fails closed, never leaks.
	tx, err := app.SQL().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if n := countUsers(t, ctx, tx); n != 0 {
		t.Errorf("unscoped read saw %d users, want 0 (fails closed)", n)
	}

	var orgs int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM organizations`).Scan(&orgs); err != nil {
		t.Fatalf("count orgs: %v", err)
	}

	if orgs != 0 {
		t.Errorf("unscoped read saw %d orgs, want 0", orgs)
	}
}

func TestRLS_ConnectsAsNonSuperuserAppRole(t *testing.T) {
	t.Parallel()

	app, _ := startIdentityDB(t)

	err := app.WithOrg(context.Background(), orgA, func(ctx context.Context, tx *sql.Tx) error {
		var (
			role  string
			super bool
		)

		const q = `SELECT current_user, rolsuper FROM pg_roles WHERE rolname = current_user`
		if err := tx.QueryRowContext(ctx, q).Scan(&role, &super); err != nil {
			return err
		}

		if role != "paddock_app" || super {
			t.Errorf("connected as %q (super=%v), want non-superuser paddock_app", role, super)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg: %v", err)
	}
}
