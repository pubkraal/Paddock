package catalog_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/catalog"
	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/platform/tabular"
)

// mockStore records calls and returns scripted results. WithOrg invokes fn with
// a nil *sql.Tx — the tx-level methods ignore it.
type mockStore struct {
	withOrgErr error

	champID, seasonID, venueID, listID string
	event                              catalog.Event
	events                             []catalog.Event
	sessions                           []catalog.Session
	entryCount                         int
	tierCounts                         map[catalog.Tier]int

	champErr, seasonErr, venueErr, eventErr, sessionsErr error
	statusErr, getEventErr, listErr, listSessErr         error
	entryListErr, entriesErr, accErr                     error
	countEntriesErr, countTierErr                        error

	accCreated bool

	consumerTier      catalog.Tier
	consumerTierFound bool
	consumerTierErr   error

	insertedSessions []catalog.SessionSpec
	insertedEntries  []catalog.Entry
	insertedAccs     []catalog.Accreditation
}

func (m *mockStore) WithOrg(ctx context.Context, _ string, fn func(context.Context, *sql.Tx) error) error {
	if m.withOrgErr != nil {
		return m.withOrgErr
	}

	return fn(ctx, nil)
}

func (m *mockStore) InsertChampionshipTx(context.Context, *sql.Tx, string, string) (string, error) {
	return m.champID, m.champErr
}

func (m *mockStore) InsertSeasonTx(context.Context, *sql.Tx, string, string, int, string) (string, error) {
	return m.seasonID, m.seasonErr
}

func (m *mockStore) InsertVenueTx(context.Context, *sql.Tx, string, string, string) (string, error) {
	return m.venueID, m.venueErr
}

func (m *mockStore) InsertEventTx(
	_ context.Context, _ *sql.Tx, _, _, _, _ string, _, _ time.Time,
) (catalog.Event, error) {
	return m.event, m.eventErr
}

func (m *mockStore) InsertSessionsTx(_ context.Context, _ *sql.Tx, _, _ string, specs []catalog.SessionSpec) error {
	m.insertedSessions = specs

	return m.sessionsErr
}

func (m *mockStore) SetEventStatusTx(context.Context, *sql.Tx, string, catalog.EventStatus) error {
	return m.statusErr
}

func (m *mockStore) GetEventTx(context.Context, *sql.Tx, string) (catalog.Event, error) {
	return m.event, m.getEventErr
}

func (m *mockStore) ListEventsTx(context.Context, *sql.Tx) ([]catalog.Event, error) {
	return m.events, m.listErr
}

func (m *mockStore) ListSessionsTx(context.Context, *sql.Tx, string) ([]catalog.Session, error) {
	return m.sessions, m.listSessErr
}

func (m *mockStore) InsertEntryListTx(context.Context, *sql.Tx, string, string, string) (string, error) {
	return m.listID, m.entryListErr
}

func (m *mockStore) InsertEntriesTx(_ context.Context, _ *sql.Tx, _, _ string, entries []catalog.Entry) error {
	m.insertedEntries = entries

	return m.entriesErr
}

func (m *mockStore) InsertAccreditationTx(_ context.Context, _ *sql.Tx, a catalog.Accreditation) (string, bool, error) {
	m.insertedAccs = append(m.insertedAccs, a)

	return "acc-id", m.accCreated, m.accErr
}

func (m *mockStore) CountEntriesTx(context.Context, *sql.Tx, string) (int, error) {
	return m.entryCount, m.countEntriesErr
}

func (m *mockStore) CountAccreditationsByTierTx(context.Context, *sql.Tx, string) (map[catalog.Tier]int, error) {
	return m.tierCounts, m.countTierErr
}

func (m *mockStore) ConsumerTierTx(context.Context, *sql.Tx, string) (catalog.Tier, bool, error) {
	return m.consumerTier, m.consumerTierFound, m.consumerTierErr
}

type mockProvisioner struct {
	user      identity.User
	created   bool
	err       error
	gotEmails []string
}

func (m *mockProvisioner) ProvisionConsumerTx(
	_ context.Context, _ *sql.Tx, _, email string,
) (identity.User, bool, error) {
	m.gotEmails = append(m.gotEmails, email)

	return m.user, m.created, m.err
}

type mockEnqueuer struct {
	count int
	err   error
}

func (m *mockEnqueuer) EnqueueInviteTx(context.Context, *sql.Tx, string, string, string) error {
	if m.err != nil {
		return m.err
	}

	m.count++

	return nil
}

var errBoomSvc = errors.New("boom")

func TestService_CreateEventFromTemplate(t *testing.T) {
	t.Parallel()

	store := &mockStore{
		champID:  "champ-1",
		seasonID: "season-1",
		venueID:  "venue-1",
		event:    catalog.Event{ID: "event-1", Status: catalog.EventDraft},
	}
	svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

	ev, err := svc.CreateEventFromTemplate(context.Background(), "org-1", catalog.CreateEventInput{
		TemplateKey:      "endurance",
		ChampionshipName: "NLS",
		SeasonName:       "2027",
		SeasonYear:       2027,
		EventName:        "24H Nürburgring",
		VenueName:        "Nürburgring",
	})
	if err != nil {
		t.Fatalf("CreateEventFromTemplate: %v", err)
	}

	if ev.ID != "event-1" {
		t.Errorf("event id = %q, want event-1", ev.ID)
	}

	if len(store.insertedSessions) != 6 {
		t.Errorf("scaffolded %d sessions, want 6 (endurance)", len(store.insertedSessions))
	}
}

func TestService_CreateEvent_NoVenueSkipsVenueInsert(t *testing.T) {
	t.Parallel()

	store := &mockStore{event: catalog.Event{ID: "event-1"}, venueErr: errBoomSvc}
	svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

	// VenueName blank → InsertVenueTx not called, so its scripted error never fires.
	if _, err := svc.CreateEventFromTemplate(context.Background(), "org-1", catalog.CreateEventInput{
		TemplateKey: "sprint",
		EventName:   "Sprint",
	}); err != nil {
		t.Fatalf("CreateEventFromTemplate: %v", err)
	}
}

func TestService_CreateEvent_Errors(t *testing.T) {
	t.Parallel()

	base := func() catalog.CreateEventInput {
		return catalog.CreateEventInput{TemplateKey: "sprint", EventName: "E", VenueName: "V"}
	}

	tests := []struct {
		name  string
		in    catalog.CreateEventInput
		store *mockStore
		want  error
	}{
		{"unknown template", catalog.CreateEventInput{TemplateKey: "nope"}, &mockStore{}, catalog.ErrUnknownTemplate},
		{"blank name", catalog.CreateEventInput{TemplateKey: "sprint"}, &mockStore{}, catalog.ErrEventNameRequired},
		{"championship", base(), &mockStore{champErr: errBoomSvc}, errBoomSvc},
		{"season", base(), &mockStore{seasonErr: errBoomSvc}, errBoomSvc},
		{"venue", base(), &mockStore{venueErr: errBoomSvc}, errBoomSvc},
		{"event", base(), &mockStore{eventErr: errBoomSvc}, errBoomSvc},
		{"sessions", base(), &mockStore{sessionsErr: errBoomSvc}, errBoomSvc},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := catalog.NewService(tt.store, &mockProvisioner{}, &mockEnqueuer{})

			_, err := svc.CreateEventFromTemplate(context.Background(), "org-1", tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestService_GoLive(t *testing.T) {
	t.Parallel()

	store := &mockStore{}
	svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

	if err := svc.GoLive(context.Background(), "org-1", "event-1"); err != nil {
		t.Fatalf("GoLive: %v", err)
	}

	store2 := &mockStore{statusErr: errBoomSvc}
	svc2 := catalog.NewService(store2, &mockProvisioner{}, &mockEnqueuer{})

	if err := svc2.GoLive(context.Background(), "org-1", "event-1"); !errors.Is(err, errBoomSvc) {
		t.Fatalf("GoLive err = %v, want errBoomSvc", err)
	}
}

func TestService_ListEvents(t *testing.T) {
	t.Parallel()

	store := &mockStore{events: []catalog.Event{{ID: "e1"}, {ID: "e2"}}}
	svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

	events, err := svc.ListEvents(context.Background(), "org-1")
	if err != nil || len(events) != 2 {
		t.Fatalf("ListEvents = %v, err %v", events, err)
	}

	store2 := &mockStore{listErr: errBoomSvc}
	svc2 := catalog.NewService(store2, &mockProvisioner{}, &mockEnqueuer{})

	if _, err := svc2.ListEvents(context.Background(), "org-1"); !errors.Is(err, errBoomSvc) {
		t.Fatalf("ListEvents err = %v, want errBoomSvc", err)
	}
}

func TestService_EventDetail(t *testing.T) {
	t.Parallel()

	store := &mockStore{
		event:      catalog.Event{ID: "event-1", Status: catalog.EventLive},
		sessions:   []catalog.Session{{ID: "s1"}},
		entryCount: 40,
		tierCounts: map[catalog.Tier]int{catalog.TierMedia: 62},
	}
	svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

	detail, err := svc.EventDetail(context.Background(), "org-1", "event-1")
	if err != nil {
		t.Fatalf("EventDetail: %v", err)
	}

	if detail.EntryCount != 40 || detail.TierCounts[catalog.TierMedia] != 62 || len(detail.Sessions) != 1 {
		t.Errorf("detail = %+v", detail)
	}
}

func TestService_EventDetail_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		store *mockStore
	}{
		{"get", &mockStore{getEventErr: errBoomSvc}},
		{"sessions", &mockStore{listSessErr: errBoomSvc}},
		{"entries", &mockStore{countEntriesErr: errBoomSvc}},
		{"tiers", &mockStore{countTierErr: errBoomSvc}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := catalog.NewService(tt.store, &mockProvisioner{}, &mockEnqueuer{})
			if _, err := svc.EventDetail(context.Background(), "org-1", "event-1"); !errors.Is(err, errBoomSvc) {
				t.Fatalf("err = %v, want errBoomSvc", err)
			}
		})
	}
}

func TestService_ConsumerTier(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		store := &mockStore{consumerTier: catalog.TierSponsor, consumerTierFound: true}
		svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

		tier, err := svc.ConsumerTier(context.Background(), "org-1", "user-1")
		if err != nil || tier != catalog.TierSponsor {
			t.Fatalf("tier = %q err = %v, want sponsor", tier, err)
		}
	})

	t.Run("default media when none", func(t *testing.T) {
		t.Parallel()

		store := &mockStore{consumerTierFound: false}
		svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

		tier, err := svc.ConsumerTier(context.Background(), "org-1", "user-1")
		if err != nil || tier != catalog.TierMedia {
			t.Fatalf("tier = %q err = %v, want media default", tier, err)
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		store := &mockStore{consumerTierErr: errBoomSvc}
		svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

		if _, err := svc.ConsumerTier(context.Background(), "org-1", "user-1"); !errors.Is(err, errBoomSvc) {
			t.Fatalf("err = %v, want errBoomSvc", err)
		}
	})
}

func entrySheet() tabular.Sheet {
	return tabular.Sheet{
		Header: []string{"Car", "Team", "Class", "Drivers"},
		Rows: [][]string{
			{"72", "AMG Landgraf", "SP9", "Mücke; Müller"},
			{"", "Ghost", "GT3", ""},       // missing car number → row error
			{"27", "Lionspeed", "GT3", ""}, // ok
			{"27", "Dup", "GT3", ""},       // duplicate car number → row error
		},
	}
}

func TestService_PreviewEntryList(t *testing.T) {
	t.Parallel()

	svc := catalog.NewService(&mockStore{}, &mockProvisioner{}, &mockEnqueuer{})

	preview, err := svc.PreviewEntryList(entrySheet())
	if err != nil {
		t.Fatalf("PreviewEntryList: %v", err)
	}

	if len(preview.Entries) != 2 {
		t.Errorf("valid entries = %d, want 2", len(preview.Entries))
	}

	if len(preview.Errors) != 2 {
		t.Errorf("row errors = %d, want 2 (missing + duplicate)", len(preview.Errors))
	}

	if len(preview.Entries[0].Drivers) != 2 {
		t.Errorf("drivers = %v, want 2 split", preview.Entries[0].Drivers)
	}
}

func TestService_PreviewEntryList_MissingColumns(t *testing.T) {
	t.Parallel()

	svc := catalog.NewService(&mockStore{}, &mockProvisioner{}, &mockEnqueuer{})

	if _, err := svc.PreviewEntryList(tabular.Sheet{Header: []string{"Foo", "Bar"}}); err == nil {
		t.Fatal("expected missing-columns error")
	}
}

func TestService_ImportEntryList(t *testing.T) {
	t.Parallel()

	store := &mockStore{listID: "list-1"}
	svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

	preview, err := svc.ImportEntryList(context.Background(), "org-1", "event-1", "entrylist.csv", entrySheet())
	if err != nil {
		t.Fatalf("ImportEntryList: %v", err)
	}

	if len(store.insertedEntries) != 2 {
		t.Errorf("persisted %d entries, want 2", len(store.insertedEntries))
	}

	if len(preview.Errors) != 2 {
		t.Errorf("preview errors = %d, want 2", len(preview.Errors))
	}
}

func TestService_ImportEntryList_ForeignEventAborts(t *testing.T) {
	t.Parallel()

	// GetEventTx returns ErrEventNotFound (the event is not in the caller's org):
	// the import must abort before inserting anything.
	store := &mockStore{getEventErr: catalog.ErrEventNotFound, listID: "should-not-be-used"}
	svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

	_, err := svc.ImportEntryList(context.Background(), "org-1", "foreign-event", "f.csv", entrySheet())
	if !errors.Is(err, catalog.ErrEventNotFound) {
		t.Fatalf("err = %v, want ErrEventNotFound", err)
	}

	if len(store.insertedEntries) != 0 {
		t.Errorf("inserted %d entries for a foreign event, want 0", len(store.insertedEntries))
	}
}

func TestService_ImportAccreditation_ForeignEventAborts(t *testing.T) {
	t.Parallel()

	store := &mockStore{getEventErr: catalog.ErrEventNotFound}
	prov := &mockProvisioner{user: identity.User{ID: "u1"}, created: true}
	enq := &mockEnqueuer{}
	svc := catalog.NewService(store, prov, enq)

	_, err := svc.ImportAccreditation(context.Background(), "org-1", "foreign-event", accSheet())
	if !errors.Is(err, catalog.ErrEventNotFound) {
		t.Fatalf("err = %v, want ErrEventNotFound", err)
	}

	if len(store.insertedAccs) != 0 || len(prov.gotEmails) != 0 || enq.count != 0 {
		t.Errorf("foreign-event accreditation import wrote/provisioned/invited; want none")
	}
}

func TestService_ImportEntryList_NoValidRowsSkipsWrite(t *testing.T) {
	t.Parallel()

	store := &mockStore{withOrgErr: errBoomSvc} // would fail if WithOrg were called
	svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})

	sheet := tabular.Sheet{Header: []string{"Car"}, Rows: [][]string{{""}}}

	preview, err := svc.ImportEntryList(context.Background(), "org-1", "event-1", "f.csv", sheet)
	if err != nil {
		t.Fatalf("ImportEntryList: %v", err)
	}

	if len(preview.Entries) != 0 || len(preview.Errors) != 1 {
		t.Errorf("preview = %+v, want 0 entries 1 error", preview)
	}
}

func TestService_ImportEntryList_Errors(t *testing.T) {
	t.Parallel()

	t.Run("mapping", func(t *testing.T) {
		t.Parallel()

		svc := catalog.NewService(&mockStore{}, &mockProvisioner{}, &mockEnqueuer{})
		if _, err := svc.ImportEntryList(context.Background(), "o", "e", "f.csv",
			tabular.Sheet{Header: []string{"X"}}); err == nil {
			t.Fatal("expected mapping error")
		}
	})

	t.Run("entry-list insert", func(t *testing.T) {
		t.Parallel()

		store := &mockStore{entryListErr: errBoomSvc}
		svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})
		if _, err := svc.ImportEntryList(context.Background(), "o", "e", "f.csv", entrySheet()); !errors.Is(err, errBoomSvc) {
			t.Fatalf("err = %v, want errBoomSvc", err)
		}
	})

	t.Run("entries insert", func(t *testing.T) {
		t.Parallel()

		store := &mockStore{listID: "l1", entriesErr: errBoomSvc}
		svc := catalog.NewService(store, &mockProvisioner{}, &mockEnqueuer{})
		if _, err := svc.ImportEntryList(context.Background(), "o", "e", "f.csv", entrySheet()); !errors.Is(err, errBoomSvc) {
			t.Fatalf("err = %v, want errBoomSvc", err)
		}
	})
}

func accSheet() tabular.Sheet {
	return tabular.Sheet{
		Header: []string{"Name", "Email", "Tier"},
		Rows: [][]string{
			{"S. Bauer", "s.bauer@nls.test", "media"},
			{"P. Iredi", "p.iredi@pirelli.test", "sponsor"},
			{"No Email", "", "media"},            // missing email
			{"Bad Mail", "not-an-email", "team"}, // invalid email
			{"Bad Tier", "x@nls.test", "vip"},    // invalid tier
		},
	}
}

func TestService_PreviewAccreditation(t *testing.T) {
	t.Parallel()

	svc := catalog.NewService(&mockStore{}, &mockProvisioner{}, &mockEnqueuer{})

	preview, err := svc.PreviewAccreditation(accSheet())
	if err != nil {
		t.Fatalf("PreviewAccreditation: %v", err)
	}

	if len(preview.Rows) != 2 {
		t.Errorf("valid rows = %d, want 2", len(preview.Rows))
	}

	if len(preview.Errors) != 3 {
		t.Errorf("errors = %d, want 3", len(preview.Errors))
	}

	if preview.TierCounts[catalog.TierMedia] != 1 || preview.TierCounts[catalog.TierSponsor] != 1 {
		t.Errorf("tier counts = %+v", preview.TierCounts)
	}
}

func TestService_PreviewAccreditation_MissingColumns(t *testing.T) {
	t.Parallel()

	svc := catalog.NewService(&mockStore{}, &mockProvisioner{}, &mockEnqueuer{})

	if _, err := svc.PreviewAccreditation(tabular.Sheet{Header: []string{"Name"}}); err == nil {
		t.Fatal("expected missing-columns error")
	}
}

func TestService_ImportAccreditation_ProvisionsAndInvites(t *testing.T) {
	t.Parallel()

	store := &mockStore{accCreated: true}
	prov := &mockProvisioner{user: identity.User{ID: "user-1"}, created: true}
	enq := &mockEnqueuer{}
	svc := catalog.NewService(store, prov, enq)

	result, err := svc.ImportAccreditation(context.Background(), "org-1", "event-1", accSheet())
	if err != nil {
		t.Fatalf("ImportAccreditation: %v", err)
	}

	if result.Invited != 2 {
		t.Errorf("invited = %d, want 2", result.Invited)
	}

	if enq.count != 2 {
		t.Errorf("enqueued = %d, want 2", enq.count)
	}

	if len(store.insertedAccs) != 2 {
		t.Errorf("inserted accreditations = %d, want 2", len(store.insertedAccs))
	}

	if len(result.Errors) != 3 {
		t.Errorf("errors = %d, want 3 (parse rejects)", len(result.Errors))
	}
}

func TestService_ImportAccreditation_DedupeNoInvite(t *testing.T) {
	t.Parallel()

	store := &mockStore{accCreated: false} // already accredited → no invite
	prov := &mockProvisioner{user: identity.User{ID: "user-1"}, created: false}
	enq := &mockEnqueuer{}
	svc := catalog.NewService(store, prov, enq)

	result, err := svc.ImportAccreditation(context.Background(), "org-1", "event-1", accSheet())
	if err != nil {
		t.Fatalf("ImportAccreditation: %v", err)
	}

	if result.Invited != 0 || enq.count != 0 {
		t.Errorf("invited = %d enqueued = %d, want 0/0 on dedupe", result.Invited, enq.count)
	}
}

func TestService_ImportAccreditation_EmailTakenBecomesRowError(t *testing.T) {
	t.Parallel()

	store := &mockStore{accCreated: true}
	prov := &mockProvisioner{err: identity.ErrEmailTaken}
	enq := &mockEnqueuer{}
	svc := catalog.NewService(store, prov, enq)

	result, err := svc.ImportAccreditation(context.Background(), "org-1", "event-1", accSheet())
	if err != nil {
		t.Fatalf("ImportAccreditation: %v", err)
	}

	// 3 parse errors + 2 email-taken rows = 5; nothing provisioned/invited.
	if len(result.Errors) != 5 || result.Invited != 0 {
		t.Errorf("errors = %d invited = %d, want 5/0", len(result.Errors), result.Invited)
	}
}

func TestService_ImportAccreditation_Errors(t *testing.T) {
	t.Parallel()

	t.Run("mapping", func(t *testing.T) {
		t.Parallel()

		svc := catalog.NewService(&mockStore{}, &mockProvisioner{}, &mockEnqueuer{})
		if _, err := svc.ImportAccreditation(context.Background(), "o", "e",
			tabular.Sheet{Header: []string{"Name"}}); err == nil {
			t.Fatal("expected mapping error")
		}
	})

	t.Run("provision", func(t *testing.T) {
		t.Parallel()

		svc := catalog.NewService(&mockStore{}, &mockProvisioner{err: errBoomSvc}, &mockEnqueuer{})
		if _, err := svc.ImportAccreditation(context.Background(), "o", "e", accSheet()); !errors.Is(err, errBoomSvc) {
			t.Fatalf("err = %v, want errBoomSvc", err)
		}
	})

	t.Run("accreditation insert", func(t *testing.T) {
		t.Parallel()

		store := &mockStore{accErr: errBoomSvc}
		prov := &mockProvisioner{user: identity.User{ID: "u1"}, created: true}
		svc := catalog.NewService(store, prov, &mockEnqueuer{})
		if _, err := svc.ImportAccreditation(context.Background(), "o", "e", accSheet()); !errors.Is(err, errBoomSvc) {
			t.Fatalf("err = %v, want errBoomSvc", err)
		}
	})

	t.Run("enqueue", func(t *testing.T) {
		t.Parallel()

		store := &mockStore{accCreated: true}
		prov := &mockProvisioner{user: identity.User{ID: "u1"}, created: true}
		svc := catalog.NewService(store, prov, &mockEnqueuer{err: errBoomSvc})
		if _, err := svc.ImportAccreditation(context.Background(), "o", "e", accSheet()); !errors.Is(err, errBoomSvc) {
			t.Fatalf("err = %v, want errBoomSvc", err)
		}
	})
}
