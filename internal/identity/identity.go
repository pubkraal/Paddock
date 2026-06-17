// Package identity owns organizations, users, and passwordless magic-link
// access (ADR-0013). It is the first tenant-scoped domain: every user belongs to
// an organization, isolation is enforced by Postgres RLS (ADR-0008), and the one
// pre-scope lookup at login goes through a SECURITY DEFINER function (ADR-0012).
package identity

import (
	"errors"
	"time"
)

var (
	// ErrUserNotFound means no user matched the given identifier.
	ErrUserNotFound = errors.New("identity: user not found")
	// ErrUserDisabled means the user exists but is not permitted to sign in.
	ErrUserDisabled = errors.New("identity: user is disabled")
)

// OrgType is the kind of organization (PLAN §5). It is a tenant, whatever its
// kind; the type drives templates and defaults in later phases.
type OrgType string

// The organization kinds Paddock models.
const (
	OrgSeries   OrgType = "series"
	OrgPromoter OrgType = "promoter"
	OrgCircuit  OrgType = "circuit"
	OrgTeam     OrgType = "team"
	OrgASN      OrgType = "asn"
)

// Role is a user's role within their organization. The three admin roles carry
// org session access; consumer is accredited media with scoped magic-link
// access.
type Role string

// The user roles Paddock models.
const (
	RolePressOfficer Role = "press_officer"
	RoleSeasonAdmin  Role = "season_admin"
	RoleFinance      Role = "finance"
	RoleConsumer     Role = "consumer"
)

// IsAdmin reports whether the role is one of the org-staff admin roles (i.e.
// not a consumer). Admin-only routes gate on this.
func (r Role) IsAdmin() bool {
	switch r {
	case RolePressOfficer, RoleSeasonAdmin, RoleFinance:
		return true
	case RoleConsumer:
		return false
	default:
		return false
	}
}

// Status is whether a user may sign in.
type Status string

// The account statuses Paddock models.
const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// Organization is a tenant.
type Organization struct {
	ID        string
	Name      string
	Type      OrgType
	Region    string
	CreatedAt time.Time
}

// User is an admin or consumer account belonging to exactly one organization.
type User struct {
	ID        string
	OrgID     string
	Email     string
	Role      Role
	Status    Status
	CreatedAt time.Time
}

// IsActive reports whether the user is permitted to sign in.
func (u User) IsActive() bool {
	return u.Status == StatusActive
}
