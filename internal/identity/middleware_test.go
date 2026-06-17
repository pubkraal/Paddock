package identity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// csrfChain wraps RequireCSRF behind Authenticate carrying a session whose CSRF
// token is "tok-123".
func csrfChain(next http.Handler) http.Handler {
	getter := mockSessionGetter{sess: identity.Session{
		UserID: "u1", OrgID: "o1", Role: identity.RolePressOfficer, CSRFToken: "tok-123",
	}}

	return identity.Authenticate(getter)(identity.RequireCSRF(next))
}

func csrfRequest(method, ct, body string) *http.Request {
	req := httptest.NewRequest(method, "/admin/events", nil)
	if body != "" {
		req = httptest.NewRequest(method, "/admin/events", strings.NewReader(body))
	}

	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})

	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}

	return req
}

func TestRequireCSRF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		setup    func(*http.Request)
		wantRan  bool
		wantCode int
	}{
		{
			name:    "safe method passes without token",
			method:  http.MethodGet,
			setup:   func(*http.Request) {},
			wantRan: true,
		},
		{
			name:    "valid header token passes",
			method:  http.MethodPost,
			setup:   func(r *http.Request) { r.Header.Set(identity.CSRFHeader, "tok-123") },
			wantRan: true,
		},
		{
			name:    "valid form token passes (urlencoded)",
			method:  http.MethodPost,
			setup:   func(*http.Request) {},
			wantRan: true,
		},
		{
			name:     "missing token rejected",
			method:   http.MethodPost,
			setup:    func(*http.Request) {},
			wantRan:  false,
			wantCode: http.StatusForbidden,
		},
		{
			name:     "wrong header token rejected",
			method:   http.MethodPost,
			setup:    func(r *http.Request) { r.Header.Set(identity.CSRFHeader, "nope") },
			wantRan:  false,
			wantCode: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				ran  bool
				seen identity.Identity
			)

			var req *http.Request

			switch tt.name {
			case "valid form token passes (urlencoded)":
				req = csrfRequest(tt.method, "application/x-www-form-urlencoded", "_csrf=tok-123")
			case "missing token rejected":
				req = csrfRequest(tt.method, "application/x-www-form-urlencoded", "x=1")
			default:
				req = csrfRequest(tt.method, "", "")
			}

			tt.setup(req)

			rr := httptest.NewRecorder()
			csrfChain(recordNext(&ran, &seen)).ServeHTTP(rr, req)

			if ran != tt.wantRan {
				t.Errorf("next ran = %v, want %v", ran, tt.wantRan)
			}

			if tt.wantCode != 0 && rr.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantCode)
			}
		})
	}
}

func TestRequireCSRF_MultipartIgnoresBodyToken(t *testing.T) {
	t.Parallel()

	// A multipart body carrying _csrf is NOT read (the size-capped handler owns
	// the body); without a header token the request is rejected.
	body := "--b\r\nContent-Disposition: form-data; name=\"_csrf\"\r\n\r\ntok-123\r\n--b--\r\n"
	req := csrfRequest(http.MethodPost, "multipart/form-data; boundary=b", body)

	var (
		ran  bool
		seen identity.Identity
	)

	rr := httptest.NewRecorder()
	csrfChain(recordNext(&ran, &seen)).ServeHTTP(rr, req)

	if ran {
		t.Error("next ran; multipart body token must not be accepted")
	}

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestRequireCSRF_EmptySessionTokenRejected(t *testing.T) {
	t.Parallel()

	getter := mockSessionGetter{sess: identity.Session{UserID: "u1", OrgID: "o1", Role: identity.RolePressOfficer}}
	chain := identity.Authenticate(getter)(identity.RequireCSRF(recordNext(new(bool), new(identity.Identity))))

	req := httptest.NewRequest(http.MethodPost, "/admin/events", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})
	req.Header.Set(identity.CSRFHeader, "anything")

	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 when the session has no CSRF token", rr.Code)
	}
}

func TestRequireCSRF_NoIdentity(t *testing.T) {
	t.Parallel()

	var (
		ran  bool
		seen identity.Identity
	)

	// RequireCSRF without Authenticate in front → no identity → 403 on unsafe.
	req := httptest.NewRequest(http.MethodPost, "/admin/events", nil)
	rr := httptest.NewRecorder()
	identity.RequireCSRF(recordNext(&ran, &seen)).ServeHTTP(rr, req)

	if ran || rr.Code != http.StatusForbidden {
		t.Errorf("ran=%v code=%d, want false/403", ran, rr.Code)
	}
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
