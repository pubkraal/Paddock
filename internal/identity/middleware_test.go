package identity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pubkraal/paddock/internal/identity"
)

type mockSessionGetter struct {
	sess identity.Session
	err  error
}

func (m mockSessionGetter) Get(_ context.Context, _ string) (identity.Session, error) {
	return m.sess, m.err
}

// recordNext is a downstream handler that notes it ran and captures the
// identity it saw.
func recordNext(ran *bool, seen *identity.Identity) http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		*ran = true

		if id, ok := identity.IdentityFromContext(r.Context()); ok {
			*seen = id
		}
	})
}

func TestAuthenticate_ValidSession(t *testing.T) {
	t.Parallel()

	store := mockSessionGetter{sess: identity.Session{
		UserID: "user-1", OrgID: "org-1", Role: identity.RolePressOfficer, Kind: identity.KindAdmin,
	}}

	var (
		ran  bool
		seen identity.Identity
	)

	h := identity.Authenticate(store)(recordNext(&ran, &seen))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if !ran {
		t.Fatal("next handler was not called for a valid session")
	}

	if seen.OrgID != "org-1" || seen.UserID != "user-1" {
		t.Errorf("identity in context = %+v, want org-1/user-1", seen)
	}
}

func TestAuthenticate_NoCookieRedirects(t *testing.T) {
	t.Parallel()

	ran := false
	h := identity.Authenticate(mockSessionGetter{})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		ran = true
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if ran {
		t.Error("next handler must not run without a session")
	}

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect", rr.Code)
	}

	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestAuthenticate_UnknownSessionRedirects(t *testing.T) {
	t.Parallel()

	store := mockSessionGetter{err: identity.ErrSessionNotFound}

	ran := false
	h := identity.Authenticate(store)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { ran = true }))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "stale"})
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if ran {
		t.Error("next handler must not run for an unresolved session")
	}

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect", rr.Code)
	}
}

// adminChain composes RequireAdmin behind Authenticate exactly as it is used,
// with a session carrying the given role so the identity reaches RequireAdmin
// through the real path.
func adminChain(role identity.Role, next http.Handler) http.Handler {
	store := mockSessionGetter{sess: identity.Session{
		UserID: "user-1", OrgID: "org-1", Role: role, Kind: identity.KindAdmin,
	}}

	return identity.Authenticate(store)(identity.RequireAdmin(next))
}

func adminRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})

	return req
}

func TestRequireAdmin_AdminPasses(t *testing.T) {
	t.Parallel()

	var (
		ran  bool
		seen identity.Identity
	)

	rr := httptest.NewRecorder()
	adminChain(identity.RoleSeasonAdmin, recordNext(&ran, &seen)).ServeHTTP(rr, adminRequest())

	if !ran {
		t.Error("admin should pass RequireAdmin")
	}
}

func TestRequireAdmin_ConsumerForbidden(t *testing.T) {
	t.Parallel()

	ran := false
	h := adminChain(identity.RoleConsumer, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { ran = true }))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, adminRequest())

	if ran {
		t.Error("consumer must not pass RequireAdmin")
	}

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestRequireAdmin_NoIdentityForbidden(t *testing.T) {
	t.Parallel()

	ran := false
	h := identity.RequireAdmin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { ran = true }))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if ran {
		t.Error("a request with no identity must not pass RequireAdmin")
	}

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}
