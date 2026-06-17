package identity

import (
	"io"
	"time"

	"github.com/pubkraal/paddock/internal/platform/redis"
)

// NewTokenStoreWithSeams injects the random source and marshaller so the
// infra-failure branches of Issue are covered without a real failure.
func NewTokenStoreWithSeams(
	client *redis.Client, ttl time.Duration, r io.Reader, marshal func(any) ([]byte, error),
) *TokenStore {
	s := NewTokenStore(client, ttl)
	s.rand = r
	s.marshal = marshal

	return s
}

// NewSessionStoreWithSeams is the SessionStore equivalent of the above.
func NewSessionStoreWithSeams(
	client *redis.Client, ttl time.Duration, r io.Reader, marshal func(any) ([]byte, error),
) *SessionStore {
	s := NewSessionStore(client, ttl)
	s.rand = r
	s.marshal = marshal

	return s
}
