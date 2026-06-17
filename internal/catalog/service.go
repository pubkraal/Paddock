package catalog

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pubkraal/paddock/internal/identity"
)

// The Service depends on these narrow interfaces (defined at the consumer) so
// the concrete repository, the identity provisioner, and the River enqueuer are
// injected, and tests use mocks.
type (
	store interface {
		WithOrg(ctx context.Context, orgID string, fn func(context.Context, *sql.Tx) error) error
		InsertChampionshipTx(ctx context.Context, tx *sql.Tx, orgID, name string) (string, error)
		InsertSeasonTx(ctx context.Context, tx *sql.Tx, orgID, championshipID string, year int, name string) (string, error)
		InsertVenueTx(ctx context.Context, tx *sql.Tx, orgID, name, circuitMapRef string) (string, error)
		InsertEventTx(
			ctx context.Context, tx *sql.Tx, orgID, seasonID, venueID, name string, startsOn, endsOn time.Time,
		) (Event, error)
		InsertSessionsTx(ctx context.Context, tx *sql.Tx, orgID, eventID string, specs []SessionSpec) error
		SetEventStatusTx(ctx context.Context, tx *sql.Tx, eventID string, status EventStatus) error
		GetEventTx(ctx context.Context, tx *sql.Tx, eventID string) (Event, error)
		ListEventsTx(ctx context.Context, tx *sql.Tx) ([]Event, error)
		ListSessionsTx(ctx context.Context, tx *sql.Tx, eventID string) ([]Session, error)
		InsertEntryListTx(ctx context.Context, tx *sql.Tx, orgID, eventID, filename string) (string, error)
		InsertEntriesTx(ctx context.Context, tx *sql.Tx, orgID, entryListID string, entries []Entry) error
		InsertAccreditationTx(ctx context.Context, tx *sql.Tx, a Accreditation) (string, bool, error)
		CountEntriesTx(ctx context.Context, tx *sql.Tx, eventID string) (int, error)
		CountAccreditationsByTierTx(ctx context.Context, tx *sql.Tx, eventID string) (map[Tier]int, error)
	}

	consumerProvisioner interface {
		ProvisionConsumerTx(ctx context.Context, tx *sql.Tx, orgID, email string) (identity.User, bool, error)
	}

	inviteEnqueuer interface {
		EnqueueInviteTx(ctx context.Context, tx *sql.Tx, userID, orgID, email string) error
	}
)

// Service is the catalog application layer: stand up an event from a template,
// import entry lists and accreditation rosters, and read back the setup state.
type Service struct {
	store       store
	provisioner consumerProvisioner
	enqueuer    inviteEnqueuer
}

// NewService wires the Service.
func NewService(s store, p consumerProvisioner, e inviteEnqueuer) *Service {
	return &Service{store: s, provisioner: p, enqueuer: e}
}

// defaulted returns v trimmed, or fallback when v is blank — so a wizard that
// omits an optional name still produces a sensibly-labelled row.
func defaulted(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}

	return v
}

// CreateEventInput is the wizard's event-setup form (PLAN §6).
type CreateEventInput struct {
	TemplateKey      string
	ChampionshipName string
	SeasonYear       int
	SeasonName       string
	VenueName        string
	VenueMapRef      string
	EventName        string
	StartsOn         time.Time
	EndsOn           time.Time
}

// CreateEventFromTemplate scaffolds a championship → season → venue → event and
// the template's sessions in one transaction (ADR-0014), returning the event.
func (s *Service) CreateEventFromTemplate(ctx context.Context, orgID string, in CreateEventInput) (Event, error) {
	tmpl, err := TemplateByKey(in.TemplateKey)
	if err != nil {
		return Event{}, err
	}

	if strings.TrimSpace(in.EventName) == "" {
		return Event{}, ErrEventNameRequired
	}

	var event Event

	err = s.store.WithOrg(ctx, orgID, func(ctx context.Context, tx *sql.Tx) error {
		champID, err := s.store.InsertChampionshipTx(ctx, tx, orgID, defaulted(in.ChampionshipName, "Championship"))
		if err != nil {
			return err
		}

		seasonID, err := s.store.InsertSeasonTx(ctx, tx, orgID, champID, in.SeasonYear, defaulted(in.SeasonName, "Season"))
		if err != nil {
			return err
		}

		var venueID string
		if strings.TrimSpace(in.VenueName) != "" {
			venueID, err = s.store.InsertVenueTx(ctx, tx, orgID, in.VenueName, in.VenueMapRef)
			if err != nil {
				return err
			}
		}

		event, err = s.store.InsertEventTx(ctx, tx, orgID, seasonID, venueID, in.EventName, in.StartsOn, in.EndsOn)
		if err != nil {
			return err
		}

		return s.store.InsertSessionsTx(ctx, tx, orgID, event.ID, tmpl.Sessions)
	})
	if err != nil {
		return Event{}, err
	}

	return event, nil
}

// GoLive moves an event from draft to live.
func (s *Service) GoLive(ctx context.Context, orgID, eventID string) error {
	return s.store.WithOrg(ctx, orgID, func(ctx context.Context, tx *sql.Tx) error {
		return s.store.SetEventStatusTx(ctx, tx, eventID, EventLive)
	})
}

// ListEvents returns the org's events, newest first (the dashboard list).
func (s *Service) ListEvents(ctx context.Context, orgID string) ([]Event, error) {
	var events []Event

	err := s.store.WithOrg(ctx, orgID, func(ctx context.Context, tx *sql.Tx) error {
		var e error
		events, e = s.store.ListEventsTx(ctx, tx)

		return e
	})
	if err != nil {
		return nil, err
	}

	return events, nil
}

// EventDetail is an event with its scaffolded sessions and import tallies, for
// the wizard's later steps and the dashboards.
type EventDetail struct {
	Event      Event
	Sessions   []Session
	EntryCount int
	TierCounts map[Tier]int
}

// EventDetail loads an event, its sessions, and its import counts in one
// transaction.
func (s *Service) EventDetail(ctx context.Context, orgID, eventID string) (EventDetail, error) {
	var detail EventDetail

	err := s.store.WithOrg(ctx, orgID, func(ctx context.Context, tx *sql.Tx) error {
		ev, err := s.store.GetEventTx(ctx, tx, eventID)
		if err != nil {
			return err
		}

		sessions, err := s.store.ListSessionsTx(ctx, tx, eventID)
		if err != nil {
			return err
		}

		entries, err := s.store.CountEntriesTx(ctx, tx, eventID)
		if err != nil {
			return err
		}

		tiers, err := s.store.CountAccreditationsByTierTx(ctx, tx, eventID)
		if err != nil {
			return err
		}

		detail = EventDetail{Event: ev, Sessions: sessions, EntryCount: entries, TierCounts: tiers}

		return nil
	})
	if err != nil {
		return EventDetail{}, err
	}

	return detail, nil
}
