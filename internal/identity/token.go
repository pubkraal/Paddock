package identity

import "time"

// Purpose records why a magic-link token was issued, so redemption builds the
// right kind of session.
type Purpose string

// The magic-link purposes.
const (
	PurposeAdminLogin    Purpose = "admin_login"
	PurposeConsumerGrant Purpose = "consumer_grant"
)

// MagicLinkToken is the payload stored in Redis under the hash of an opaque
// token. The raw token is never stored; redemption hashes the presented token
// and looks it up (ADR-0013).
type MagicLinkToken struct {
	UserID   string
	OrgID    string
	Role     Role
	Purpose  Purpose
	Scope    string
	IssuedAt time.Time
}
