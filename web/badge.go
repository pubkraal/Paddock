package web

// Status is a design-system status token. In Phase 5 the typed Asset state
// machine becomes the single source of truth and is projected onto these
// tokens; the badge is always the *visual projection* of that enum, never a
// second model of state (PLAN §4).
type Status string

// The asset-state tokens of the racing-flag system (PLAN §4).
const (
	StatusRaw       Status = "raw"
	StatusSelect    Status = "select"
	StatusPublished Status = "published"
	StatusEmbargoed Status = "embargoed"
	StatusKilled    Status = "killed"
	StatusArchived  Status = "archived"
)

// Badge is the rendered form of a status: a flag colour class and an uppercase
// label, shown as a pill (dot + label) on tiles, tables, and inspectors.
type Badge struct {
	Status Status
	Label  string
	Flag   string
}

// flags is the racing-flag table from PLAN §4. The flag name is the CSS modifier
// (badge--<flag>) whose colour is defined in the token layer.
var flags = map[Status]Badge{
	StatusRaw:       {Status: StatusRaw, Label: "RAW", Flag: "neutral"},
	StatusSelect:    {Status: StatusSelect, Label: "SELECT", Flag: "yellow"},
	StatusPublished: {Status: StatusPublished, Label: "PUBLISHED", Flag: "green"},
	StatusEmbargoed: {Status: StatusEmbargoed, Label: "EMBARGO", Flag: "red"},
	StatusKilled:    {Status: StatusKilled, Label: "KILLED", Flag: "red"},
	StatusArchived:  {Status: StatusArchived, Label: "CLOSED", Flag: "chequered"},
}

// BadgeFor returns the badge for a status token, or ok=false if the token is not
// part of the racing-flag system.
func BadgeFor(s Status) (Badge, bool) {
	b, ok := flags[s]

	return b, ok
}

// Statuses returns the asset-state tokens in display order, for the styleguide.
func Statuses() []Status {
	return []Status{
		StatusRaw,
		StatusSelect,
		StatusPublished,
		StatusEmbargoed,
		StatusKilled,
		StatusArchived,
	}
}
