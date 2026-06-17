//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/platform/postgres"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func startPostgres(t *testing.T) *postgres.Pool {
	t.Helper()

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17",
		tcpostgres.WithDatabase("paddock"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
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

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
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
