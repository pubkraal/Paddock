package identity_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/platform/postgres"
)

func newMock(t *testing.T) (*postgres.Pool, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}

		_ = db.Close()
	})

	return postgres.New(db), mock
}

func TestRepository_LookupFound(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)

	// No Begin/Commit is expected: Lookup must NOT open a WithOrg transaction.
	mock.ExpectQuery("identity_lookup").
		WithArgs("press@example.test").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "org_id", "role", "status"}).
			AddRow("user-1", "org-1", "press_officer", "active"))

	repo := identity.NewRepository(pool)

	got, err := repo.Lookup(context.Background(), "press@example.test")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	want := identity.User{
		ID:     "user-1",
		OrgID:  "org-1",
		Email:  "press@example.test",
		Role:   identity.RolePressOfficer,
		Status: identity.StatusActive,
	}

	if got != want {
		t.Errorf("user = %+v, want %+v", got, want)
	}
}

func TestRepository_LookupNotFound(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	mock.ExpectQuery("identity_lookup").
		WithArgs("ghost@example.test").
		WillReturnError(sql.ErrNoRows)

	_, err := identity.NewRepository(pool).Lookup(context.Background(), "ghost@example.test")
	if !errors.Is(err, identity.ErrUserNotFound) {
		t.Errorf("Lookup error = %v, want ErrUserNotFound", err)
	}
}

func TestRepository_LookupQueryError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	mock.ExpectQuery("identity_lookup").
		WithArgs("press@example.test").
		WillReturnError(errors.New("connection reset"))

	_, err := identity.NewRepository(pool).Lookup(context.Background(), "press@example.test")
	if err == nil || errors.Is(err, identity.ErrUserNotFound) {
		t.Errorf("Lookup error = %v, want a wrapped driver error", err)
	}
}

func TestRepository_GetUserFound(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	created := time.Unix(1700000000, 0).UTC()

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM users WHERE id").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "email", "role", "status", "created_at"}).
			AddRow("user-1", "org-1", "press@example.test", "season_admin", "active", created))
	mock.ExpectCommit()

	got, err := identity.NewRepository(pool).GetUser(context.Background(), "org-1", "user-1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	if got.Role != identity.RoleSeasonAdmin || got.Email != "press@example.test" {
		t.Errorf("user = %+v, want season_admin press@example.test", got)
	}
}

func TestRepository_GetUserNotFound(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM users WHERE id").WithArgs("ghost").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err := identity.NewRepository(pool).GetUser(context.Background(), "org-1", "ghost")
	if !errors.Is(err, identity.ErrUserNotFound) {
		t.Errorf("GetUser error = %v, want ErrUserNotFound", err)
	}
}

func TestRepository_GetUserScopeError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnError(errors.New("guc failed"))
	mock.ExpectRollback()

	_, err := identity.NewRepository(pool).GetUser(context.Background(), "org-1", "user-1")
	if err == nil || errors.Is(err, identity.ErrUserNotFound) {
		t.Errorf("GetUser error = %v, want a wrapped scope error", err)
	}
}

func TestRepository_GetUserEmptyOrg(t *testing.T) {
	t.Parallel()

	pool, _ := newMock(t)

	// WithOrg rejects an empty org before any SQL, so no expectations are set.
	_, err := identity.NewRepository(pool).GetUser(context.Background(), "", "user-1")
	if err == nil {
		t.Fatal("expected error for empty org, got nil")
	}
}
