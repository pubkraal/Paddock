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

// provision runs ProvisionConsumerTx inside a WithOrg transaction against the
// mock, returning the result. The caller registers the per-call query
// expectations before invoking; matching is unordered so they need not precede
// the begin/commit envelope set here.
func provision(
	t *testing.T, pool *postgres.Pool, mock sqlmock.Sqlmock, commit bool, email string,
) (identity.User, bool, error) {
	t.Helper()

	repo := identity.NewRepository(pool)

	mock.MatchExpectationsInOrder(false)
	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))

	if commit {
		mock.ExpectCommit()
	} else {
		mock.ExpectRollback()
	}

	var (
		u       identity.User
		created bool
	)

	err := pool.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		u, created, e = repo.ProvisionConsumerTx(ctx, tx, "org-1", email)

		return e
	})

	return u, created, err
}

func TestProvisionConsumer_Inserts(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := identity.NewRepository(pool)
	createdAt := time.Unix(1700000000, 0).UTC()

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO users").
		WithArgs("org-1", "new@nls.test").
		WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "email", "role", "status", "created_at"}).
			AddRow("user-1", "org-1", "new@nls.test", "consumer", "active", createdAt))
	mock.ExpectCommit()

	var (
		u       identity.User
		created bool
	)

	err := pool.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		u, created, e = repo.ProvisionConsumerTx(ctx, tx, "org-1", "new@nls.test")

		return e
	})
	if err != nil {
		t.Fatalf("ProvisionConsumerTx: %v", err)
	}

	if !created || u.ID != "user-1" || u.Role != identity.RoleConsumer {
		t.Errorf("u=%+v created=%v, want new consumer user-1", u, created)
	}
}

func TestProvisionConsumer_ExistingInOrg(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	createdAt := time.Unix(1700000000, 0).UTC()

	mock.ExpectQuery("INSERT INTO users").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("FROM users WHERE email").
		WithArgs("dup@nls.test").
		WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "email", "role", "status", "created_at"}).
			AddRow("user-9", "org-1", "dup@nls.test", "consumer", "active", createdAt))

	u, created, err := provision(t, pool, mock, true, "dup@nls.test")
	if err != nil {
		t.Fatalf("ProvisionConsumerTx: %v", err)
	}

	if created || u.ID != "user-9" {
		t.Errorf("u=%+v created=%v, want existing user-9 created=false", u, created)
	}
}

func TestProvisionConsumer_EmailTakenByAnotherOrg(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)

	mock.ExpectQuery("INSERT INTO users").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("FROM users WHERE email").WithArgs("elsewhere@x.test").WillReturnError(sql.ErrNoRows)

	_, _, err := provision(t, pool, mock, false, "elsewhere@x.test")
	if !errors.Is(err, identity.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

func TestProvisionConsumer_InsertError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)

	mock.ExpectQuery("INSERT INTO users").WillReturnError(errors.New("connection reset"))

	_, _, err := provision(t, pool, mock, false, "x@nls.test")
	if err == nil || errors.Is(err, identity.ErrEmailTaken) {
		t.Fatalf("err = %v, want a wrapped insert error", err)
	}
}

func TestProvisionConsumer_ReselectError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)

	mock.ExpectQuery("INSERT INTO users").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("FROM users WHERE email").WithArgs("x@nls.test").WillReturnError(errors.New("boom"))

	_, _, err := provision(t, pool, mock, false, "x@nls.test")
	if err == nil || errors.Is(err, identity.ErrEmailTaken) {
		t.Fatalf("err = %v, want a wrapped reselect error", err)
	}
}
