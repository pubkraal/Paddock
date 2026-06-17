package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pubkraal/paddock/internal/platform/postgres"
)

// Repository is the identity data-access layer over Postgres. Reads of tenant
// data go through pool.WithOrg so RLS applies; the one exception is Lookup, the
// pre-scope login bootstrap (ADR-0012).
type Repository struct {
	pool *postgres.Pool
}

// NewRepository builds a Repository over the given pool.
func NewRepository(pool *postgres.Pool) *Repository {
	return &Repository{pool: pool}
}

const lookupSQL = `SELECT user_id, org_id, role, status FROM identity_lookup($1)`

// Lookup resolves an email to its user WITHOUT a tenant scope — the single
// sanctioned pre-scope read (ADR-0012), served by the SECURITY DEFINER function.
// It is deliberately not wrapped in WithOrg: at login no org is known yet. An
// unknown email returns ErrUserNotFound; the caller must not reveal that.
func (r *Repository) Lookup(ctx context.Context, email string) (User, error) {
	u := User{Email: email}

	err := r.pool.SQL().QueryRowContext(ctx, lookupSQL, email).
		Scan(&u.ID, &u.OrgID, &u.Role, &u.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		return User{}, fmt.Errorf("identity: lookup: %w", err)
	}

	return u, nil
}

const provisionConsumerSQL = `
INSERT INTO users (org_id, email, role, status)
VALUES ($1, $2, 'consumer', 'active')
ON CONFLICT (email) DO NOTHING
RETURNING id, org_id, email, role, status, created_at`

const selectUserByEmailSQL = `
SELECT id, org_id, email, role, status, created_at FROM users WHERE email = $1`

// ProvisionConsumerTx ensures a consumer account exists for email within the
// transaction's org scope, returning the user and whether it was newly created.
// It runs inside a caller-supplied tx (the accreditation-import transaction, so
// the user, its accreditation, and its invite job commit atomically — ADR-0016).
//
// Provisioning is idempotent on the globally-unique email (ADR-0012): a fresh
// email inserts (created=true); an email already in this org re-selects it
// (created=false), so re-import does not double-invite. An email registered to a
// DIFFERENT org conflicts on insert and is invisible under this scope, yielding
// ErrEmailTaken — the importer reports it as a row error rather than silently
// rebinding the person.
func (r *Repository) ProvisionConsumerTx(ctx context.Context, tx *sql.Tx, orgID, email string) (User, bool, error) {
	var u User

	err := tx.QueryRowContext(ctx, provisionConsumerSQL, orgID, email).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Role, &u.Status, &u.CreatedAt)
	if err == nil {
		return u, true, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return User{}, false, fmt.Errorf("identity: provision consumer: %w", err)
	}

	err = tx.QueryRowContext(ctx, selectUserByEmailSQL, email).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Role, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, ErrEmailTaken
	}

	if err != nil {
		return User{}, false, fmt.Errorf("identity: reselect consumer: %w", err)
	}

	return u, false, nil
}

const getUserSQL = `SELECT id, org_id, email, role, status, created_at FROM users WHERE id = $1`

// GetUser loads a user by id within the org's scope. The org scope is set via
// WithOrg; RLS guarantees the row, if any, belongs to that org, so the query
// filters on id alone.
func (r *Repository) GetUser(ctx context.Context, orgID, userID string) (User, error) {
	var u User

	err := r.pool.WithOrg(ctx, orgID, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, getUserSQL, userID).
			Scan(&u.ID, &u.OrgID, &u.Email, &u.Role, &u.Status, &u.CreatedAt)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		return User{}, fmt.Errorf("identity: get user: %w", err)
	}

	return u, nil
}
