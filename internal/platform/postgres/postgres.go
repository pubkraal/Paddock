// Package postgres provides the application's Postgres pool and the WithOrg
// transaction helper — the ONLY sanctioned way to touch tenant-owned data
// (ADR-0008). WithOrg opens a transaction, sets the per-transaction
// `app.current_org` GUC, and runs the caller's work inside it; RLS policies do
// the enforcing. A missing org fails closed: the helper refuses to run.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // register the "pgx" database/sql driver
)

// ErrEmptyOrg is returned by WithOrg when called without an org id. Tenant work
// must always be scoped; an empty org would defeat RLS, so it is refused.
var ErrEmptyOrg = errors.New("postgres: empty org id")

// setOrgSQL sets the tenancy GUC for the lifetime of the current transaction.
// set_config(..., is_local=true) is the parameterized equivalent of
// `SET LOCAL app.current_org = $1`, which Postgres will not accept with a bind
// parameter.
const setOrgSQL = `SELECT set_config('app.current_org', $1, true)`

const (
	maxOpenConns    = 20
	maxIdleConns    = 10
	connMaxLifetime = time.Hour
)

// Pool wraps the application's *sql.DB. It is the non-superuser, RLS-bound
// connection; migrations use a separate BYPASSRLS role and never this pool.
type Pool struct {
	db *sql.DB
}

// New wraps an existing *sql.DB. Used in tests (sqlmock) and by OpenWith.
func New(db *sql.DB) *Pool {
	return &Pool{db: db}
}

// Open connects to Postgres via the pgx database/sql driver and verifies the
// connection with a ping.
func Open(ctx context.Context, url string) (*Pool, error) {
	return OpenWith(ctx, url, func(u string) (*sql.DB, error) {
		return sql.Open("pgx", u)
	})
}

// OpenWith is Open with an injectable opener, so connection setup is testable
// without a live database.
func OpenWith(ctx context.Context, url string, opener func(string) (*sql.DB, error)) (*Pool, error) {
	db, err := opener(url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Pool{db: db}, nil
}

// SQL exposes the underlying *sql.DB for components that need it directly
// (e.g. River's driver and golang-migrate in integration harnesses).
func (p *Pool) SQL() *sql.DB {
	return p.db
}

// Ping verifies the pool can reach the database.
func (p *Pool) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// Close releases the pool.
func (p *Pool) Close() error {
	return p.db.Close()
}

// WithOrg runs fn inside a transaction whose `app.current_org` GUC is set to
// orgID. On any error from setting the GUC or from fn, the transaction is rolled
// back; otherwise it is committed. This is the only sanctioned entry point for
// tenant-scoped queries.
func (p *Pool) WithOrg(ctx context.Context, orgID string, fn func(context.Context, *sql.Tx) error) error {
	if orgID == "" {
		return ErrEmptyOrg
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if _, err := tx.ExecContext(ctx, setOrgSQL, orgID); err != nil {
		_ = tx.Rollback()

		return fmt.Errorf("set org guc: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		_ = tx.Rollback()

		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
