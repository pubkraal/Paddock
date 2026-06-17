package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"time"
)

// tokenBytes is the entropy of an opaque token / session id (256 bits).
const tokenBytes = 32

// generateToken reads tokenBytes of randomness and returns a URL-safe opaque
// string. The reader is injected so the (practically unreachable) failure path
// is testable.
func generateToken(r io.Reader) (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken returns the hex SHA-256 of a raw token. Only the hash is stored, so
// a leaked store never yields a usable link.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))

	return hex.EncodeToString(sum[:])
}

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
