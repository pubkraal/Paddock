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

// ErrTokenInvalidOrUsed is returned when a presented magic-link token does not
// resolve — it never existed, expired, or was already redeemed. The cases are
// deliberately indistinguishable to the caller.
var ErrTokenInvalidOrUsed = errors.New("identity: magic-link token invalid, expired, or already used")

const magicLinkKeyPrefix = "magiclink:"

// TokenStore issues and redeems single-use magic-link tokens in Redis. The raw
// token is returned once at issue and never stored; the store holds only its
// hash with a TTL (ADR-0013).
type TokenStore struct {
	rdb     *goredis.Client
	ttl     time.Duration
	rand    io.Reader
	marshal func(any) ([]byte, error)
}

// NewTokenStore builds a TokenStore over the Redis client with the given token
// TTL.
func NewTokenStore(client *redis.Client, ttl time.Duration) *TokenStore {
	return &TokenStore{rdb: client.Redis(), ttl: ttl, rand: rand.Reader, marshal: json.Marshal}
}

// Issue generates an opaque token, stores its payload under the token's hash for
// the configured TTL, and returns the raw token — the only moment it exists.
func (s *TokenStore) Issue(ctx context.Context, tok MagicLinkToken) (string, error) {
	raw, err := generateToken(s.rand)
	if err != nil {
		return "", fmt.Errorf("identity: generate token: %w", err)
	}

	payload, err := s.marshal(tok)
	if err != nil {
		return "", fmt.Errorf("identity: marshal token: %w", err)
	}

	if err := s.rdb.Set(ctx, magicLinkKeyPrefix+hashToken(raw), payload, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("identity: store token: %w", err)
	}

	return raw, nil
}

// Redeem atomically consumes the token for the given raw value: the payload is
// read and deleted in one GETDEL, so a replayed link finds nothing and fails
// with ErrTokenInvalidOrUsed.
func (s *TokenStore) Redeem(ctx context.Context, raw string) (MagicLinkToken, error) {
	payload, err := s.rdb.GetDel(ctx, magicLinkKeyPrefix+hashToken(raw)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return MagicLinkToken{}, ErrTokenInvalidOrUsed
	}

	if err != nil {
		return MagicLinkToken{}, fmt.Errorf("identity: redeem token: %w", err)
	}

	var tok MagicLinkToken
	if err := json.Unmarshal(payload, &tok); err != nil {
		return MagicLinkToken{}, fmt.Errorf("identity: unmarshal token: %w", err)
	}

	return tok, nil
}
