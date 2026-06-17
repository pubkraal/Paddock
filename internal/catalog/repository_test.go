package catalog_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pubkraal/paddock/internal/catalog"
	"github.com/pubkraal/paddock/internal/platform/postgres"
)

var errBoom = errors.New("boom")

func newMock(t *testing.T) (*postgres.Pool, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}

	// Several error-path tests register their query expectation lazily inside the
	// WithOrg callback (after the begin/commit envelope), so match unordered. The
	// set_config arg still proves every operation is org-scoped.
	mock.MatchExpectationsInOrder(false)

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}

		_ = db.Close()
	})

	return postgres.New(db), mock
}

// inOrg runs fn inside a WithOrg transaction against the mock, asserting the
// begin/set_config/commit envelope that proves the work is org-scoped.
func inOrg(t *testing.T, repo *catalog.Repository, mock sqlmock.Sqlmock, commit bool, fn func(*sql.Tx) error) error {
	t.Helper()

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))

	if commit {
		mock.ExpectCommit()
	} else {
		mock.ExpectRollback()
	}

	return repo.WithOrg(context.Background(), "org-1", func(_ context.Context, tx *sql.Tx) error {
		return fn(tx)
	})
}

func TestRepository_InsertChampionship(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO championships").
		WithArgs("org-1", "NLS").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("champ-1"))
	mock.ExpectCommit()

	var id string

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		id, e = repo.InsertChampionshipTx(ctx, tx, "org-1", "NLS")

		return e
	})
	if err != nil {
		t.Fatalf("WithOrg: %v", err)
	}

	if id != "champ-1" {
		t.Errorf("id = %q, want champ-1", id)
	}
}

func TestRepository_InsertChampionshipError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("INSERT INTO championships").WillReturnError(errBoom)
		_, e := repo.InsertChampionshipTx(context.Background(), tx, "org-1", "NLS")

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_InsertSeason(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO seasons").
		WithArgs("org-1", "champ-1", 2027, "2027").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("season-1"))
	mock.ExpectCommit()

	var id string

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		id, e = repo.InsertSeasonTx(ctx, tx, "org-1", "champ-1", 2027, "2027")

		return e
	})
	if err != nil || id != "season-1" {
		t.Fatalf("InsertSeasonTx id=%q err=%v", id, err)
	}
}

func TestRepository_InsertSeasonError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("INSERT INTO seasons").WillReturnError(errBoom)
		_, e := repo.InsertSeasonTx(context.Background(), tx, "org-1", "champ-1", 2027, "2027")

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_InsertVenue(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO venues").
		WithArgs("org-1", "Nürburgring", "nordschleife").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("venue-1"))
	mock.ExpectCommit()

	var id string

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		id, e = repo.InsertVenueTx(ctx, tx, "org-1", "Nürburgring", "nordschleife")

		return e
	})
	if err != nil || id != "venue-1" {
		t.Fatalf("InsertVenueTx id=%q err=%v", id, err)
	}
}

func TestRepository_InsertVenueError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("INSERT INTO venues").WillReturnError(errBoom)
		_, e := repo.InsertVenueTx(context.Background(), tx, "org-1", "v", "m")

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_InsertEvent(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)
	created := time.Unix(1700000000, 0).UTC()
	starts := time.Date(2027, 5, 22, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO events").
		WithArgs("org-1", "season-1", "venue-1", "24H", starts, sqlmock.AnyArg(), "draft").
		WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "season_id", "venue_id", "name", "status", "created_at"}).
			AddRow("event-1", "org-1", "season-1", "venue-1", "24H", "draft", created))
	mock.ExpectCommit()

	var ev catalog.Event

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		ev, e = repo.InsertEventTx(ctx, tx, "org-1", "season-1", "venue-1", "24H", starts, time.Time{})

		return e
	})
	if err != nil {
		t.Fatalf("InsertEventTx: %v", err)
	}

	if ev.ID != "event-1" || ev.Status != catalog.EventDraft || !ev.StartsOn.Equal(starts) {
		t.Errorf("event = %+v", ev)
	}
}

func TestRepository_InsertEventError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("INSERT INTO events").WillReturnError(errBoom)
		_, e := repo.InsertEventTx(context.Background(), tx, "org-1", "s", "", "n", time.Time{}, time.Time{})

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_InsertSessions(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	specs := []catalog.SessionSpec{
		{Type: catalog.SessionPractice, Name: "FP"},
		{Type: catalog.SessionRace, Name: "Race"},
	}

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO sessions").
		WithArgs("org-1", "event-1", "practice", "FP", 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO sessions").
		WithArgs("org-1", "event-1", "race", "Race", 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		return repo.InsertSessionsTx(ctx, tx, "org-1", "event-1", specs)
	})
	if err != nil {
		t.Fatalf("InsertSessionsTx: %v", err)
	}
}

func TestRepository_InsertSessionsError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectExec("INSERT INTO sessions").WillReturnError(errBoom)

		return repo.InsertSessionsTx(context.Background(), tx, "org-1", "event-1",
			[]catalog.SessionSpec{{Type: catalog.SessionRace, Name: "Race"}})
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_SetEventStatus(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE events SET status").
		WithArgs("event-1", "live").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		return repo.SetEventStatusTx(ctx, tx, "event-1", catalog.EventLive)
	})
	if err != nil {
		t.Fatalf("SetEventStatusTx: %v", err)
	}
}

func TestRepository_SetEventStatusNotFound(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectExec("UPDATE events SET status").
			WithArgs("ghost", "live").
			WillReturnResult(sqlmock.NewResult(0, 0))

		return repo.SetEventStatusTx(context.Background(), tx, "ghost", catalog.EventLive)
	})
	if !errors.Is(err, catalog.ErrEventNotFound) {
		t.Fatalf("err = %v, want ErrEventNotFound", err)
	}
}

func TestRepository_SetEventStatusExecError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectExec("UPDATE events SET status").WillReturnError(errBoom)

		return repo.SetEventStatusTx(context.Background(), tx, "event-1", catalog.EventLive)
	})
	if err == nil || errors.Is(err, catalog.ErrEventNotFound) {
		t.Fatalf("err = %v, want a wrapped exec error", err)
	}
}

func TestRepository_SetEventStatusRowsError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectExec("UPDATE events SET status").
			WithArgs("event-1", "live").
			WillReturnResult(sqlmock.NewErrorResult(errBoom))

		return repo.SetEventStatusTx(context.Background(), tx, "event-1", catalog.EventLive)
	})
	if err == nil {
		t.Fatal("expected error from RowsAffected")
	}
}

func TestRepository_GetEvent(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)
	created := time.Unix(1700000000, 0).UTC()

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM events WHERE id").
		WithArgs("event-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "season_id", "venue_id", "name", "status", "created_at"}).
			AddRow("event-1", "org-1", "season-1", "", "24H", "live", created))
	mock.ExpectCommit()

	var ev catalog.Event

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		ev, e = repo.GetEventTx(ctx, tx, "event-1")

		return e
	})
	if err != nil {
		t.Fatalf("GetEventTx: %v", err)
	}

	if !ev.IsLive() || ev.Name != "24H" {
		t.Errorf("event = %+v", ev)
	}
}

func TestRepository_GetEventNotFound(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("FROM events WHERE id").WithArgs("ghost").WillReturnError(sql.ErrNoRows)
		_, e := repo.GetEventTx(context.Background(), tx, "ghost")

		return e
	})
	if !errors.Is(err, catalog.ErrEventNotFound) {
		t.Fatalf("err = %v, want ErrEventNotFound", err)
	}
}

func TestRepository_GetEventQueryError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("FROM events WHERE id").WithArgs("event-1").WillReturnError(errBoom)
		_, e := repo.GetEventTx(context.Background(), tx, "event-1")

		return e
	})
	if err == nil || errors.Is(err, catalog.ErrEventNotFound) {
		t.Fatalf("err = %v, want wrapped query error", err)
	}
}

func TestRepository_ListEvents(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)
	created := time.Unix(1700000000, 0).UTC()

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM events ORDER BY created_at").
		WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "season_id", "venue_id", "name", "status", "created_at"}).
			AddRow("event-2", "org-1", "season-1", "venue-1", "Sprint", "draft", created).
			AddRow("event-1", "org-1", "season-1", "", "24H", "live", created))
	mock.ExpectCommit()

	var events []catalog.Event

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		events, e = repo.ListEventsTx(ctx, tx)

		return e
	})
	if err != nil {
		t.Fatalf("ListEventsTx: %v", err)
	}

	if len(events) != 2 || events[0].ID != "event-2" {
		t.Errorf("events = %+v", events)
	}
}

func TestRepository_ListEventsQueryError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("FROM events ORDER BY created_at").WillReturnError(errBoom)
		_, e := repo.ListEventsTx(context.Background(), tx)

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_ListEventsScanError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("FROM events ORDER BY created_at").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only-one-column"))
		_, e := repo.ListEventsTx(context.Background(), tx)

		return e
	})
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestRepository_ListEventsRowsErr(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		rows := sqlmock.NewRows([]string{"id", "org_id", "season_id", "venue_id", "name", "status", "created_at"}).
			AddRow("event-1", "org-1", "season-1", "", "24H", "live", time.Now()).
			RowError(0, errBoom)
		mock.ExpectQuery("FROM events ORDER BY created_at").WillReturnRows(rows)
		_, e := repo.ListEventsTx(context.Background(), tx)

		return e
	})
	if err == nil {
		t.Fatal("expected rows error")
	}
}

func TestRepository_ListSessions(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM sessions WHERE event_id").
		WithArgs("event-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "event_id", "type", "name", "ordinal"}).
			AddRow("s1", "org-1", "event-1", "practice", "FP", 1).
			AddRow("s2", "org-1", "event-1", "race", "Race", 2))
	mock.ExpectCommit()

	var sessions []catalog.Session

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		sessions, e = repo.ListSessionsTx(ctx, tx, "event-1")

		return e
	})
	if err != nil {
		t.Fatalf("ListSessionsTx: %v", err)
	}

	if len(sessions) != 2 || sessions[1].Type != catalog.SessionRace {
		t.Errorf("sessions = %+v", sessions)
	}
}

func TestRepository_ListSessionsErrors(t *testing.T) {
	t.Parallel()

	t.Run("query", func(t *testing.T) {
		t.Parallel()

		pool, mock := newMock(t)
		repo := catalog.NewRepository(pool)

		err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
			mock.ExpectQuery("FROM sessions WHERE event_id").WithArgs("event-1").WillReturnError(errBoom)
			_, e := repo.ListSessionsTx(context.Background(), tx, "event-1")

			return e
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("scan", func(t *testing.T) {
		t.Parallel()

		pool, mock := newMock(t)
		repo := catalog.NewRepository(pool)

		err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
			mock.ExpectQuery("FROM sessions WHERE event_id").WithArgs("event-1").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("bad"))
			_, e := repo.ListSessionsTx(context.Background(), tx, "event-1")

			return e
		})
		if err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("rows", func(t *testing.T) {
		t.Parallel()

		pool, mock := newMock(t)
		repo := catalog.NewRepository(pool)

		err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
			rows := sqlmock.NewRows([]string{"id", "org_id", "event_id", "type", "name", "ordinal"}).
				AddRow("s1", "org-1", "event-1", "race", "Race", 1).RowError(0, errBoom)
			mock.ExpectQuery("FROM sessions WHERE event_id").WithArgs("event-1").WillReturnRows(rows)
			_, e := repo.ListSessionsTx(context.Background(), tx, "event-1")

			return e
		})
		if err == nil {
			t.Fatal("expected rows error")
		}
	})
}

func TestRepository_InsertEntryList(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO entry_lists").
		WithArgs("org-1", "event-1", "entrylist.csv").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("list-1"))
	mock.ExpectCommit()

	var id string

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		id, e = repo.InsertEntryListTx(ctx, tx, "org-1", "event-1", "entrylist.csv")

		return e
	})
	if err != nil || id != "list-1" {
		t.Fatalf("InsertEntryListTx id=%q err=%v", id, err)
	}
}

func TestRepository_InsertEntryListError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("INSERT INTO entry_lists").WillReturnError(errBoom)
		_, e := repo.InsertEntryListTx(context.Background(), tx, "org-1", "event-1", "f.csv")

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_InsertEntries(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	entries := []catalog.Entry{
		{CarNo: "72", Team: "AMG", Class: "SP9", Drivers: []string{"A", "B"}, LiveryRefs: []string{"l1"}},
		{CarNo: "27", Team: "Lionspeed", Class: "GT3"},
	}

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO entries").
		WithArgs("org-1", "list-1", "72", "AMG", "SP9", `{"A","B"}`, `{"l1"}`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO entries").
		WithArgs("org-1", "list-1", "27", "Lionspeed", "GT3", "{}", "{}").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		return repo.InsertEntriesTx(ctx, tx, "org-1", "list-1", entries)
	})
	if err != nil {
		t.Fatalf("InsertEntriesTx: %v", err)
	}
}

func TestRepository_InsertEntriesError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectExec("INSERT INTO entries").WillReturnError(errBoom)

		return repo.InsertEntriesTx(context.Background(), tx, "org-1", "list-1",
			[]catalog.Entry{{CarNo: "72"}})
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_InsertAccreditationInserted(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	acc := catalog.Accreditation{
		OrgID: "org-1", EventID: "event-1", UserID: "user-1",
		PersonName: "S. Bauer", Email: "s.bauer@nls.test", Tier: catalog.TierMedia,
	}

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO accreditations").
		WithArgs("org-1", "event-1", "user-1", "S. Bauer", "s.bauer@nls.test", "media",
			sqlmock.AnyArg(), sqlmock.AnyArg(), "").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("acc-1"))
	mock.ExpectCommit()

	var (
		id      string
		created bool
	)

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		id, created, e = repo.InsertAccreditationTx(ctx, tx, acc)

		return e
	})
	if err != nil {
		t.Fatalf("InsertAccreditationTx: %v", err)
	}

	if id != "acc-1" || !created {
		t.Errorf("id=%q created=%v, want acc-1 true", id, created)
	}
}

func TestRepository_InsertAccreditationConflict(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	var created bool

	err := inOrg(t, repo, mock, true, func(tx *sql.Tx) error {
		mock.ExpectQuery("INSERT INTO accreditations").WillReturnError(sql.ErrNoRows)

		var e error
		_, created, e = repo.InsertAccreditationTx(context.Background(), tx,
			catalog.Accreditation{OrgID: "org-1", EventID: "event-1", Email: "dup@nls.test", Tier: catalog.TierMedia})

		return e
	})
	if err != nil {
		t.Fatalf("conflict should not error: %v", err)
	}

	if created {
		t.Error("created = true on conflict, want false")
	}
}

func TestRepository_InsertAccreditationError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("INSERT INTO accreditations").WillReturnError(errBoom)
		_, _, e := repo.InsertAccreditationTx(context.Background(), tx,
			catalog.Accreditation{OrgID: "org-1", EventID: "event-1", Email: "x@nls.test", Tier: catalog.TierMedia})

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_ConsumerTier(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM accreditations WHERE user_id").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"tier"}).AddRow("team"))
	mock.ExpectCommit()

	var (
		tier  catalog.Tier
		found bool
	)

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		tier, found, e = repo.ConsumerTierTx(ctx, tx, "user-1")

		return e
	})
	if err != nil || !found || tier != catalog.TierTeam {
		t.Fatalf("ConsumerTierTx tier=%q found=%v err=%v", tier, found, err)
	}
}

func TestRepository_ConsumerTierNotFound(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	var found bool

	err := inOrg(t, repo, mock, true, func(tx *sql.Tx) error {
		mock.ExpectQuery("FROM accreditations WHERE user_id").WithArgs("ghost").WillReturnError(sql.ErrNoRows)

		var e error
		_, found, e = repo.ConsumerTierTx(context.Background(), tx, "ghost")

		return e
	})
	if err != nil || found {
		t.Fatalf("found=%v err=%v, want not found no error", found, err)
	}
}

func TestRepository_ConsumerTierError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("FROM accreditations WHERE user_id").WithArgs("user-1").WillReturnError(errBoom)
		_, _, e := repo.ConsumerTierTx(context.Background(), tx, "user-1")

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_CountEntries(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("count.*FROM entries").
		WithArgs("event-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(40))
	mock.ExpectCommit()

	var n int

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		n, e = repo.CountEntriesTx(ctx, tx, "event-1")

		return e
	})
	if err != nil || n != 40 {
		t.Fatalf("CountEntriesTx n=%d err=%v", n, err)
	}
}

func TestRepository_CountEntriesError(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
		mock.ExpectQuery("count.*FROM entries").WillReturnError(errBoom)
		_, e := repo.CountEntriesTx(context.Background(), tx, "event-1")

		return e
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepository_CountAccreditationsByTier(t *testing.T) {
	t.Parallel()

	pool, mock := newMock(t)
	repo := catalog.NewRepository(pool)

	mock.ExpectBegin()
	mock.ExpectExec("set_config").WithArgs("org-1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM accreditations WHERE event_id").
		WithArgs("event-1").
		WillReturnRows(sqlmock.NewRows([]string{"tier", "count"}).
			AddRow("media", 62).
			AddRow("sponsor", 28))
	mock.ExpectCommit()

	var counts map[catalog.Tier]int

	err := repo.WithOrg(context.Background(), "org-1", func(ctx context.Context, tx *sql.Tx) error {
		var e error
		counts, e = repo.CountAccreditationsByTierTx(ctx, tx, "event-1")

		return e
	})
	if err != nil {
		t.Fatalf("CountAccreditationsByTierTx: %v", err)
	}

	if counts[catalog.TierMedia] != 62 || counts[catalog.TierSponsor] != 28 {
		t.Errorf("counts = %+v", counts)
	}
}

func TestRepository_CountAccreditationsByTierErrors(t *testing.T) {
	t.Parallel()

	t.Run("query", func(t *testing.T) {
		t.Parallel()

		pool, mock := newMock(t)
		repo := catalog.NewRepository(pool)

		err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
			mock.ExpectQuery("FROM accreditations WHERE event_id").WithArgs("event-1").WillReturnError(errBoom)
			_, e := repo.CountAccreditationsByTierTx(context.Background(), tx, "event-1")

			return e
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("scan", func(t *testing.T) {
		t.Parallel()

		pool, mock := newMock(t)
		repo := catalog.NewRepository(pool)

		err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
			mock.ExpectQuery("FROM accreditations WHERE event_id").WithArgs("event-1").
				WillReturnRows(sqlmock.NewRows([]string{"tier"}).AddRow("media"))
			_, e := repo.CountAccreditationsByTierTx(context.Background(), tx, "event-1")

			return e
		})
		if err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("rows", func(t *testing.T) {
		t.Parallel()

		pool, mock := newMock(t)
		repo := catalog.NewRepository(pool)

		err := inOrg(t, repo, mock, false, func(tx *sql.Tx) error {
			rows := sqlmock.NewRows([]string{"tier", "count"}).AddRow("media", 1).RowError(0, errBoom)
			mock.ExpectQuery("FROM accreditations WHERE event_id").WithArgs("event-1").WillReturnRows(rows)
			_, e := repo.CountAccreditationsByTierTx(context.Background(), tx, "event-1")

			return e
		})
		if err == nil {
			t.Fatal("expected rows error")
		}
	})
}
