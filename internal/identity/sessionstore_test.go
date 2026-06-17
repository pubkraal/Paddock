package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/identity"
)

func testSession() identity.Session {
	return identity.Session{
		UserID: "user-1",
		OrgID:  "org-1",
		Role:   identity.RoleSeasonAdmin,
		Kind:   identity.KindAdmin,
		Scope:  "",
	}
}

func TestSessionStore_CreateThenGetRoundTrips(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)
	ctx := context.Background()

	id, err := store.Create(ctx, testSession())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if id == "" {
		t.Fatal("Create returned an empty id")
	}

	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// The CSRF token is generated at create time, so compare the rest exactly and
	// assert the token round-trips as a non-empty value.
	if got.CSRFToken == "" {
		t.Error("session round-trip lost the CSRF token")
	}

	want := testSession()
	want.ID = id
	want.CSRFToken = got.CSRFToken

	if got != want {
		t.Errorf("session = %+v, want %+v", got, want)
	}
}

func TestSessionStore_GetUnknown(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)

	_, err := store.Get(context.Background(), "nope")
	if !errors.Is(err, identity.ErrSessionNotFound) {
		t.Errorf("Get error = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStore_GetAfterExpiry(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)
	ctx := context.Background()

	id, err := store.Create(ctx, testSession())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mr.FastForward(2 * time.Hour)

	if _, err := store.Get(ctx, id); !errors.Is(err, identity.ErrSessionNotFound) {
		t.Errorf("Get after expiry = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStore_CreateSetsTTL(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)

	if _, err := store.Create(context.Background(), testSession()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	keys := mr.Keys()
	if len(keys) != 1 {
		t.Fatalf("expected one key, got %v", keys)
	}

	if ttl := mr.TTL(keys[0]); ttl != time.Hour {
		t.Errorf("session TTL = %v, want 1h", ttl)
	}
}

func TestSessionStore_DeleteRemovesSession(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)
	ctx := context.Background()

	id, err := store.Create(ctx, testSession())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := store.Get(ctx, id); !errors.Is(err, identity.ErrSessionNotFound) {
		t.Errorf("Get after delete = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStore_DeleteAbsentIsNoError(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)

	if err := store.Delete(context.Background(), "absent"); err != nil {
		t.Errorf("Delete of absent session = %v, want nil", err)
	}
}

func TestSessionStore_GetCorruptPayload(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)
	ctx := context.Background()

	id, err := store.Create(ctx, testSession())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, k := range mr.Keys() {
		if err := mr.Set(k, "not-json"); err != nil {
			t.Fatalf("poison: %v", err)
		}
	}

	if _, err := store.Get(ctx, id); err == nil {
		t.Fatal("expected unmarshal error for a corrupt payload, got nil")
	}
}

func TestSessionStore_CreateGenerateError(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewSessionStoreWithSeams(client, time.Hour, failingReader{}, json.Marshal)

	if _, err := store.Create(context.Background(), testSession()); err == nil {
		t.Fatal("expected error when the entropy source fails, got nil")
	}
}

// secondReadFails fills the buffer on the first Read (the session id) then fails
// on the second (the CSRF token), so the csrf-generation error path is covered.
type secondReadFails struct{ calls int }

func (s *secondReadFails) Read(p []byte) (int, error) {
	s.calls++
	if s.calls > 1 {
		return 0, errors.New("no entropy")
	}

	return len(p), nil
}

func TestSessionStore_CreateCSRFGenerateError(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewSessionStoreWithSeams(client, time.Hour, &secondReadFails{}, json.Marshal)

	if _, err := store.Create(context.Background(), testSession()); err == nil {
		t.Fatal("expected error when CSRF token generation fails, got nil")
	}
}

func TestSessionStore_CreateMarshalError(t *testing.T) {
	t.Parallel()

	_, client := newRedis(t)
	store := identity.NewSessionStoreWithSeams(client, time.Hour, zeroReader{}, failingMarshal)

	if _, err := store.Create(context.Background(), testSession()); err == nil {
		t.Fatal("expected error when marshalling fails, got nil")
	}
}

func TestSessionStore_CreateStoreError(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)
	mr.Close()

	if _, err := store.Create(context.Background(), testSession()); err == nil {
		t.Fatal("expected store error against a closed server, got nil")
	}
}

func TestSessionStore_GetStoreError(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)
	mr.Close()

	if _, err := store.Get(context.Background(), "anything"); err == nil {
		t.Fatal("expected load error against a closed server, got nil")
	}
}

func TestSessionStore_DeleteStoreError(t *testing.T) {
	t.Parallel()

	mr, client := newRedis(t)
	store := identity.NewSessionStore(client, time.Hour)
	mr.Close()

	if err := store.Delete(context.Background(), "anything"); err == nil {
		t.Fatal("expected delete error against a closed server, got nil")
	}
}
