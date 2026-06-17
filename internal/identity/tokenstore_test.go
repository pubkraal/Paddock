package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/platform/config"
	"github.com/pubkraal/paddock/internal/platform/redis"
)

func newRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.Open(config.Redis{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return mr, client
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { return len(p), nil }

func failingMarshal(any) ([]byte, error) { return nil, errors.New("cannot marshal") }

func testToken() identity.MagicLinkToken {
	return identity.MagicLinkToken{
		UserID:   "user-1",
		OrgID:    "org-1",
		Role:     identity.RolePressOfficer,
		Purpose:  identity.PurposeAdminLogin,
		IssuedAt: time.Unix(1700000000, 0).UTC(),
	}
}

func TestTokenStore_IssueThenRedeemRoundTrips(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewTokenStore(client, time.Minute)
	ctx := context.Background()

	raw, err := store.Issue(ctx, testToken())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if raw == "" {
		t.Fatal("Issue returned an empty token")
	}

	// The raw token must not appear as a key — only its hash is stored.
	for _, k := range mr.Keys() {
		if strings.Contains(k, raw) {
			t.Errorf("raw token leaked into key %q", k)
		}
	}

	got, err := store.Redeem(ctx, raw)
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}

	if got != testToken() {
		t.Errorf("redeemed token = %+v, want %+v", got, testToken())
	}
}

func TestTokenStore_RedeemIsSingleUse(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewTokenStore(client, time.Minute)
	ctx := context.Background()

	raw, err := store.Issue(ctx, testToken())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if _, err := store.Redeem(ctx, raw); err != nil {
		t.Fatalf("first Redeem: %v", err)
	}

	_, err = store.Redeem(ctx, raw)
	if !errors.Is(err, identity.ErrTokenInvalidOrUsed) {
		t.Errorf("second Redeem error = %v, want ErrTokenInvalidOrUsed", err)
	}
}

func TestTokenStore_RedeemUnknownToken(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewTokenStore(client, time.Minute)

	_, err := store.Redeem(context.Background(), "never-issued")
	if !errors.Is(err, identity.ErrTokenInvalidOrUsed) {
		t.Errorf("Redeem error = %v, want ErrTokenInvalidOrUsed", err)
	}
}

func TestTokenStore_RedeemAfterExpiry(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewTokenStore(client, time.Minute)
	ctx := context.Background()

	raw, err := store.Issue(ctx, testToken())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	mr.FastForward(2 * time.Minute)

	_, err = store.Redeem(ctx, raw)
	if !errors.Is(err, identity.ErrTokenInvalidOrUsed) {
		t.Errorf("Redeem after expiry = %v, want ErrTokenInvalidOrUsed", err)
	}
}

func TestTokenStore_IssueSetsTTL(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewTokenStore(client, time.Minute)

	if _, err := store.Issue(context.Background(), testToken()); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	keys := mr.Keys()
	if len(keys) != 1 {
		t.Fatalf("expected exactly one key, got %v", keys)
	}

	if ttl := mr.TTL(keys[0]); ttl != time.Minute {
		t.Errorf("key TTL = %v, want 1m", ttl)
	}
}

func TestTokenStore_RedeemCorruptPayload(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewTokenStore(client, time.Minute)
	ctx := context.Background()

	raw, err := store.Issue(ctx, testToken())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Poison the stored payload so unmarshal fails on redeem.
	for _, k := range mr.Keys() {
		if err := mr.Set(k, "not-json"); err != nil {
			t.Fatalf("poison key: %v", err)
		}
	}

	if _, err := store.Redeem(ctx, raw); err == nil {
		t.Fatal("expected unmarshal error redeeming a corrupt payload, got nil")
	}
}

func TestTokenStore_IssueGenerateError(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewTokenStoreWithSeams(client, time.Minute, failingReader{}, json.Marshal)

	if _, err := store.Issue(context.Background(), testToken()); err == nil {
		t.Fatal("expected error when the entropy source fails, got nil")
	}
}

func TestTokenStore_IssueMarshalError(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewTokenStoreWithSeams(client, time.Minute, zeroReader{}, failingMarshal)

	if _, err := store.Issue(context.Background(), testToken()); err == nil {
		t.Fatal("expected error when marshalling fails, got nil")
	}

	if len(mr.Keys()) != 0 {
		t.Error("nothing should be stored when marshalling fails")
	}
}

func TestTokenStore_IssueStoreError(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewTokenStore(client, time.Minute)
	mr.Close() // server gone — the SET must fail

	if _, err := store.Issue(context.Background(), testToken()); err == nil {
		t.Fatal("expected store error against a closed server, got nil")
	}
}

func TestTokenStore_RedeemStoreError(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewTokenStore(client, time.Minute)
	mr.Close()

	if _, err := store.Redeem(context.Background(), "anything"); err == nil {
		t.Fatal("expected redeem error against a closed server, got nil")
	}
}
