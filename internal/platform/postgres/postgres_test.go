package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pubkraal/paddock/internal/platform/postgres"
)

func newMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}

		_ = db.Close()
	})

	return db, mock
}

func TestWithOrg_CommitsOnSuccess(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-A").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	var ran bool
	err := postgres.New(db).WithOrg(context.Background(), "org-A", func(_ context.Context, tx *sql.Tx) error {
		ran = true

		if tx == nil {
			t.Error("fn received nil tx")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithOrg: %v", err)
	}

	if !ran {
		t.Error("fn was not run")
	}
}

func TestWithOrg_RollsBackOnFnError(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-B").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	wantErr := errors.New("work failed")
	err := postgres.New(db).WithOrg(context.Background(), "org-B", func(context.Context, *sql.Tx) error {
		return wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestWithOrg_EmptyOrgRejected(t *testing.T) {
	t.Parallel()

	db, _ := newMock(t)

	err := postgres.New(db).WithOrg(context.Background(), "", func(context.Context, *sql.Tx) error {
		t.Error("fn must not run with an empty org")

		return nil
	})

	if !errors.Is(err, postgres.ErrEmptyOrg) {
		t.Errorf("err = %v, want ErrEmptyOrg", err)
	}
}

func TestWithOrg_BeginError(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectBegin().WillReturnError(errors.New("no connection"))

	err := postgres.New(db).WithOrg(context.Background(), "org-A", func(context.Context, *sql.Tx) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected begin error, got nil")
	}
}

func TestWithOrg_SetConfigErrorRollsBack(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-A").WillReturnError(errors.New("guc failed"))
	mock.ExpectRollback()

	err := postgres.New(db).WithOrg(context.Background(), "org-A", func(context.Context, *sql.Tx) error {
		t.Error("fn must not run when the GUC could not be set")

		return nil
	})
	if err == nil {
		t.Fatal("expected set_config error, got nil")
	}
}

func TestWithOrg_CommitError(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-A").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	err := postgres.New(db).WithOrg(context.Background(), "org-A", func(context.Context, *sql.Tx) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected commit error, got nil")
	}
}

func TestPing(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectPing()

	if err := postgres.New(db).Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectClose()

	if err := postgres.New(db).Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSQLAccessor(t *testing.T) {
	t.Parallel()

	db, _ := newMock(t)

	if postgres.New(db).SQL() != db {
		t.Error("SQL() did not return the wrapped *sql.DB")
	}
}

func TestOpenWith_PingSuccess(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectPing()

	pool, err := postgres.OpenWith(context.Background(), "dsn", func(string) (*sql.DB, error) {
		return db, nil
	})
	if err != nil {
		t.Fatalf("OpenWith: %v", err)
	}

	if pool.SQL() != db {
		t.Error("OpenWith did not wrap the opened *sql.DB")
	}
}

func TestOpenWith_OpenerError(t *testing.T) {
	t.Parallel()

	_, err := postgres.OpenWith(context.Background(), "dsn", func(string) (*sql.DB, error) {
		return nil, errors.New("bad dsn")
	})
	if err == nil {
		t.Fatal("expected opener error, got nil")
	}
}

func TestOpenWith_PingFailureClosesAndErrors(t *testing.T) {
	t.Parallel()

	db, mock := newMock(t)
	mock.ExpectPing().WillReturnError(errors.New("unreachable"))
	mock.ExpectClose()

	_, err := postgres.OpenWith(context.Background(), "dsn", func(string) (*sql.DB, error) {
		return db, nil
	})
	if err == nil {
		t.Fatal("expected ping error, got nil")
	}
}

func TestOpen_RealDriverUnreachable(t *testing.T) {
	t.Parallel()

	_, err := postgres.Open(
		context.Background(),
		"postgres://u:p@127.0.0.1:1/x?sslmode=disable&connect_timeout=1",
	)
	if err == nil {
		t.Fatal("expected connection error against an unreachable port, got nil")
	}
}
