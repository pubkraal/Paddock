//go:build integration

package catalog_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/platform/postgres"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// repoRoot resolves the repository root relative to this test file so the shared
// roles bootstrap and the migrations apply exactly as production does.
func repoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}

	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// startCatalogDB brings up a real Postgres with the two-role bootstrap, applies
// the app migrations as the BYPASSRLS migrate role, and returns a pool connected
// as the non-superuser paddock_app (so RLS applies) plus the migrate *sql.DB for
// cross-tenant seeding. Mirrors the Phase 1 identity harness.
func startCatalogDB(t *testing.T) (*postgres.Pool, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	root := repoRoot(t)
	rolesScript := filepath.Join(root, "deploy", "postgres", "init", "00-roles.sql")

	container, err := tcpostgres.Run(ctx, "postgres:17",
		tcpostgres.WithDatabase("paddock"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.WithInitScripts(rolesScript),
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

	dsn := func(role string) string {
		return fmt.Sprintf("postgres://%s:%s@%s:%s/paddock?sslmode=disable", role, role, host, port.Port())
	}

	migratePool, err := postgres.Open(ctx, dsn("paddock_migrate"))
	if err != nil {
		t.Fatalf("open migrate pool: %v", err)
	}

	t.Cleanup(func() { _ = migratePool.Close() })

	applyMigrations(t, migratePool.SQL(), filepath.Join(root, "migrations"))

	appPool, err := postgres.Open(ctx, dsn("paddock_app"))
	if err != nil {
		t.Fatalf("open app pool: %v", err)
	}

	t.Cleanup(func() { _ = appPool.Close() })

	return appPool, migratePool.SQL()
}

// applyMigrations runs every *.up.sql in order as the migrate role.
func applyMigrations(t *testing.T, db *sql.DB, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	var ups []string

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}

	sort.Strings(ups)

	for _, name := range ups {
		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}

		if _, err := db.ExecContext(context.Background(), string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

// seedOrg inserts an organization as the migrate role (cross-tenant fixtures
// cannot go through the RLS-bound app role).
func seedOrg(t *testing.T, db *sql.DB, id, name, orgType, region string) {
	t.Helper()

	const q = `INSERT INTO organizations (id, name, type, region) VALUES ($1, $2, $3, $4)`
	if _, err := db.ExecContext(context.Background(), q, id, name, orgType, region); err != nil {
		t.Fatalf("seed org %s: %v", id, err)
	}
}

const (
	orgA = "11111111-1111-1111-1111-111111111111"
	orgB = "22222222-2222-2222-2222-222222222222"
)
