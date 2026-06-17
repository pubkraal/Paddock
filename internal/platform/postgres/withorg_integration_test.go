//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/platform/postgres"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// rolesInitScript is the canonical two-role bootstrap (ADR-0008/0009) shared
// with the docker-compose dev stack, so integration tests exercise the exact
// role setup production uses.
func rolesInitScript(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}

	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "deploy", "postgres", "init", "00-roles.sql")
}

// startPostgres brings up a real Postgres with the two-role bootstrap applied
// and returns a pool connected as the NON-superuser application role
// (paddock_app), so RLS policies actually apply to it. Superuser connections
// bypass RLS (ADR-0008); tenant-isolation proofs must never use them.
func startPostgres(t *testing.T) *postgres.Pool {
	t.Helper()

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17",
		tcpostgres.WithDatabase("paddock"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.WithInitScripts(rolesInitScript(t)),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://paddock_app:paddock_app@%s:%s/paddock?sslmode=disable", host, port.Port())

	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool as paddock_app: %v", err)
	}

	t.Cleanup(func() { _ = pool.Close() })

	return pool
}

// currentOrg reads the tenancy GUC inside the supplied transaction. With
// missing_ok=true it returns "" when the GUC is unset rather than erroring.
func currentOrg(t *testing.T, ctx context.Context, tx *sql.Tx) string {
	t.Helper()

	var got sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT current_setting('app.current_org', true)`).Scan(&got); err != nil {
		t.Fatalf("read app.current_org: %v", err)
	}

	return got.String
}

func TestPool_ConnectsAsNonSuperuserAppRole(t *testing.T) {
	t.Parallel()

	pool := startPostgres(t)
	ctx := context.Background()

	err := pool.WithOrg(ctx, "11111111-1111-1111-1111-111111111111", func(ctx context.Context, tx *sql.Tx) error {
		var (
			role  string
			super bool
		)

		const q = `SELECT current_user, rolsuper FROM pg_roles WHERE rolname = current_user`
		if err := tx.QueryRowContext(ctx, q).Scan(&role, &super); err != nil {
			return err
		}

		if role != "paddock_app" {
			t.Errorf("current_user = %q, want paddock_app (RLS-bound app role)", role)
		}

		if super {
			t.Error("application role is a superuser — RLS would be bypassed (ADR-0008)")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg: %v", err)
	}
}

func TestWithOrg_SetsGUCInsideTransaction(t *testing.T) {
	t.Parallel()

	pool := startPostgres(t)
	ctx := context.Background()

	const orgA = "11111111-1111-1111-1111-111111111111"

	err := pool.WithOrg(ctx, orgA, func(ctx context.Context, tx *sql.Tx) error {
		if got := currentOrg(t, ctx, tx); got != orgA {
			t.Errorf("app.current_org = %q, want %q", got, orgA)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg: %v", err)
	}
}

func TestWithOrg_GUCDoesNotLeakAcrossTransactions(t *testing.T) {
	t.Parallel()

	pool := startPostgres(t)
	ctx := context.Background()

	const (
		orgA = "11111111-1111-1111-1111-111111111111"
		orgB = "22222222-2222-2222-2222-222222222222"
	)

	if err := pool.WithOrg(ctx, orgA, func(context.Context, *sql.Tx) error { return nil }); err != nil {
		t.Fatalf("WithOrg orgA: %v", err)
	}

	// A second transaction for a different org must see only its own value,
	// proving SET LOCAL is transaction-scoped and never leaks.
	err := pool.WithOrg(ctx, orgB, func(ctx context.Context, tx *sql.Tx) error {
		if got := currentOrg(t, ctx, tx); got != orgB {
			t.Errorf("app.current_org = %q, want %q (no leak from orgA)", got, orgB)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg orgB: %v", err)
	}
}

func TestWithOrg_GUCUnsetOutsideHelper(t *testing.T) {
	t.Parallel()

	pool := startPostgres(t)
	ctx := context.Background()

	// A transaction opened directly (bypassing WithOrg) has no tenancy GUC set:
	// the value is empty, which is what makes RLS fail closed.
	tx, err := pool.SQL().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if got := currentOrg(t, ctx, tx); got != "" {
		t.Errorf("app.current_org = %q, want empty outside WithOrg", got)
	}
}
