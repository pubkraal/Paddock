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
