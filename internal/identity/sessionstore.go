package identity

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/pubkraal/paddock/internal/platform/redis"
	goredis "github.com/redis/go-redis/v9"
)

// ErrSessionNotFound is returned when a session id resolves to no live session
// (it never existed, expired, or was deleted by logout).
var ErrSessionNotFound = errors.New("identity: session not found")

const sessionKeyPrefix = "session:"

// sessionValue is the JSON payload stored in Redis. The id is the key, and
// expiry is the Redis TTL, so neither is duplicated in the value.
type sessionValue struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Role   Role   `json:"role"`
	Kind   Kind   `json:"kind"`
	Scope  string `json:"scope"`
}

// SessionStore creates, loads, and deletes Redis-backed sessions (ADR-0013).
type SessionStore struct {
	rdb     *goredis.Client
	ttl     time.Duration
	rand    io.Reader
	marshal func(any) ([]byte, error)
}

// NewSessionStore builds a SessionStore over the Redis client with the given
// session TTL.
func NewSessionStore(client *redis.Client, ttl time.Duration) *SessionStore {
	return &SessionStore{rdb: client.Redis(), ttl: ttl, rand: rand.Reader, marshal: json.Marshal}
}

// Create stores a new session under a fresh opaque id and returns the id (the
// cookie value).
func (s *SessionStore) Create(ctx context.Context, sess Session) (string, error) {
	id, err := generateToken(s.rand)
	if err != nil {
		return "", fmt.Errorf("identity: generate session id: %w", err)
	}

	payload, err := s.marshal(sessionValue{
		UserID: sess.UserID,
		OrgID:  sess.OrgID,
		Role:   sess.Role,
		Kind:   sess.Kind,
		Scope:  sess.Scope,
	})
	if err != nil {
		return "", fmt.Errorf("identity: marshal session: %w", err)
	}

	if err := s.rdb.Set(ctx, sessionKeyPrefix+id, payload, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("identity: store session: %w", err)
	}

	return id, nil
}

// Get loads the session for an id, returning ErrSessionNotFound when none is
// live.
func (s *SessionStore) Get(ctx context.Context, id string) (Session, error) {
	payload, err := s.rdb.Get(ctx, sessionKeyPrefix+id).Bytes()
	if errors.Is(err, goredis.Nil) {
		return Session{}, ErrSessionNotFound
	}

	if err != nil {
		return Session{}, fmt.Errorf("identity: load session: %w", err)
	}

	var v sessionValue
	if err := json.Unmarshal(payload, &v); err != nil {
		return Session{}, fmt.Errorf("identity: unmarshal session: %w", err)
	}

	return Session{
		ID:     id,
		UserID: v.UserID,
		OrgID:  v.OrgID,
		Role:   v.Role,
		Kind:   v.Kind,
		Scope:  v.Scope,
	}, nil
}

// Delete removes a session (logout). Deleting an absent session is not an error.
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	if err := s.rdb.Del(ctx, sessionKeyPrefix+id).Err(); err != nil {
		return fmt.Errorf("identity: delete session: %w", err)
	}

	return nil
}
