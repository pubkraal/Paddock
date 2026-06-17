// Package catalog owns the event-setup domain (PLAN §6 Phase 2): the
// Championship → Season → Event → Session hierarchy and Venue, onboarding
// templates that scaffold an event's sessions (ADR-0014), and the entry-list /
// accreditation imports that populate it. Every type is tenant-scoped; writes
// go through postgres.WithOrg so RLS applies (ADR-0008).
package catalog

import (
	"errors"
	"strings"
	"time"
)

var (
	// ErrInvalidSessionType means a string did not name a known session kind.
	ErrInvalidSessionType = errors.New("catalog: invalid session type")
	// ErrInvalidTier means a string did not name a known accreditation tier.
	ErrInvalidTier = errors.New("catalog: invalid accreditation tier")
	// ErrEventNotFound means no event matched within the org scope.
	ErrEventNotFound = errors.New("catalog: event not found")
	// ErrUnknownTemplate means no onboarding template has the given key.
	ErrUnknownTemplate = errors.New("catalog: unknown template")
)

// SessionType is the typed kind of a session within an event (PLAN §5).
type SessionType string

// The session kinds Paddock models.
const (
	SessionPractice   SessionType = "practice"
	SessionQualifying SessionType = "qualifying"
	SessionRace       SessionType = "race"
	SessionWarmup     SessionType = "warmup"
	SessionPodium     SessionType = "podium"
	SessionPaddock    SessionType = "paddock"
)

// Valid reports whether the value is a known session kind.
func (s SessionType) Valid() bool {
	switch s {
	case SessionPractice, SessionQualifying, SessionRace, SessionWarmup, SessionPodium, SessionPaddock:
		return true
	default:
		return false
	}
}

// ParseSessionType resolves an exact session-kind string, rejecting anything
// else with ErrInvalidSessionType.
func ParseSessionType(s string) (SessionType, error) {
	st := SessionType(s)
	if !st.Valid() {
		return "", ErrInvalidSessionType
	}

	return st, nil
}

// Tier is an accreditation band (the design's four tiers), which selects a
// consumer's portal dashboard.
type Tier string

// The accreditation tiers Paddock models.
const (
	TierMedia    Tier = "media"
	TierSponsor  Tier = "sponsor"
	TierTeam     Tier = "team"
	TierInternal Tier = "internal"
)

// Valid reports whether the value is a known tier.
func (t Tier) Valid() bool {
	switch t {
	case TierMedia, TierSponsor, TierTeam, TierInternal:
		return true
	default:
		return false
	}
}

// ParseTier resolves a tier string case-insensitively (rosters are
// human-entered), rejecting anything else with ErrInvalidTier.
func ParseTier(s string) (Tier, error) {
	t := Tier(strings.ToLower(strings.TrimSpace(s)))
	if !t.Valid() {
		return "", ErrInvalidTier
	}

	return t, nil
}

// EventStatus is an event's lifecycle state: draft during setup, live once the
// operator clicks "Go live".
type EventStatus string

// The event statuses Paddock models.
const (
	EventDraft EventStatus = "draft"
	EventLive  EventStatus = "live"
)

// Venue is a circuit. Zones (corner/marshal-post) are data-only this phase.
type Venue struct {
	ID            string
	OrgID         string
	Name          string
	CircuitMapRef string
	CreatedAt     time.Time
}

// Championship is the top of the catalog hierarchy.
type Championship struct {
	ID        string
	OrgID     string
	Name      string
	CreatedAt time.Time
}

// Season is one running of a championship.
type Season struct {
	ID             string
	OrgID          string
	ChampionshipID string
	Year           int
	Name           string
	CreatedAt      time.Time
}

// Event is a race weekend scaffolded from a template.
type Event struct {
	ID        string
	OrgID     string
	SeasonID  string
	VenueID   string
	Name      string
	StartsOn  time.Time
	EndsOn    time.Time
	Status    EventStatus
	CreatedAt time.Time
}

// IsLive reports whether the event has gone live.
func (e Event) IsLive() bool {
	return e.Status == EventLive
}

// Session is one on-track or paddock session of an event.
type Session struct {
	ID        string
	OrgID     string
	EventID   string
	Type      SessionType
	Name      string
	Ordinal   int
	CreatedAt time.Time
}

// EntryList is one imported entry list for an event, carrying the source
// filename for the audit trail.
type EntryList struct {
	ID             string
	OrgID          string
	EventID        string
	SourceFilename string
	CreatedAt      time.Time
}

// Entry is one car in an entry list (PLAN §5: Car № → Team → Driver lineup →
// Class → livery refs). Drivers and livery refs are multi-valued.
type Entry struct {
	ID          string
	OrgID       string
	EntryListID string
	CarNo       string
	Team        string
	Class       string
	Drivers     []string
	LiveryRefs  []string
	CreatedAt   time.Time
}

// Accreditation is one accredited person for an event (PLAN §5: person → tier →
// validity window → credential ref). UserID links to the provisioned consumer
// account once invited (ADR-0016).
type Accreditation struct {
	ID            string
	OrgID         string
	EventID       string
	UserID        string
	PersonName    string
	Email         string
	Tier          Tier
	ValidFrom     time.Time
	ValidTo       time.Time
	CredentialRef string
	CreatedAt     time.Time
}
