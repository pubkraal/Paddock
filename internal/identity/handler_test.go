package identity_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/web"
)

type mockAuthService struct {
	requestErr error
	gotEmail   string
	calls      chan string
	redeemSess identity.Session
	redeemErr  error
	loggedOut  string
	logoutErr  error
}

func (m *mockAuthService) RequestMagicLink(_ context.Context, email string) error {
	m.gotEmail = email

	if m.calls != nil {
		m.calls <- email
	}

	return m.requestErr
}

func (m *mockAuthService) Redeem(_ context.Context, _ string) (identity.Session, error) {
	return m.redeemSess, m.redeemErr
}

func (m *mockAuthService) Logout(_ context.Context, sessionID string) error {
	m.loggedOut = sessionID

	return m.logoutErr
}

type mockUserReader struct {
	user identity.User
	err  error
}

func (m mockUserReader) GetUser(_ context.Context, _, _ string) (identity.User, error) {
	return m.user, m.err
}

type fakeRenderer struct {
	err error
}

func (f fakeRenderer) Render(_ io.Writer, _ string, _ any) error { return f.err }

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newHandler(t *testing.T, svc *mockAuthService, users mockUserReader) *identity.Handler {
	t.Helper()

	r, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	h := identity.NewHandler(identity.HandlerConfig{
		Service:      svc,
		Users:        users,
		Renderer:     r,
		Logger:       quietLogger(),
		CookieSecure: true,
		SessionTTL:   time.Hour,
	})
	identity.RunDispatchSynchronously(h)

	return h
}

func TestHandler_LoginPage(t *testing.T) {
	t.Parallel()

	h := newHandler(t, &mockAuthService{}, mockUserReader{})

	rr := httptest.NewRecorder()
	h.LoginPage().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Sign in to your organization") {
		t.Error("login page missing its heading")
	}

	if strings.Contains(body, `type="password"`) {
		t.Error("login page must not contain a password field (passwordless, ADR-0013)")
	}

	if strings.Contains(body, "SSO") || strings.Contains(body, "Okta") {
		t.Error("login page must not contain SSO controls (out of MVP)")
	}
}

func TestHandler_LoginPageRenderError(t *testing.T) {
	t.Parallel()

	h := identity.NewHandler(identity.HandlerConfig{
		Service:  &mockAuthService{},
		Users:    mockUserReader{},
		Renderer: fakeRenderer{err: errors.New("template boom")},
		Logger:   quietLogger(),
	})

	rr := httptest.NewRecorder()
	h.LoginPage().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 on render failure", rr.Code)
	}
}

const testEmail = "press@example.test"

func requestLink(t *testing.T, h *identity.Handler, htmx bool) *httptest.ResponseRecorder {
	t.Helper()

	body := strings.NewReader("email=" + testEmail)
	req := httptest.NewRequest(http.MethodPost, "/auth/magic", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if htmx {
		req.Header.Set("HX-Request", "true")
	}

	rr := httptest.NewRecorder()
	h.RequestLink().ServeHTTP(rr, req)

	return rr
}

func TestHandler_RequestLinkAntiEnumeration(t *testing.T) {
	t.Parallel()

	// An existing-but-infra-failing send and a clean send must produce the
	// identical response for the same email: existence/outcome must not leak.
	clean := requestLink(t, newHandler(t, &mockAuthService{}, mockUserReader{}), false)
	failing := requestLink(t,
		newHandler(t, &mockAuthService{requestErr: errors.New("redis down")}, mockUserReader{}),
		false)

	if clean.Code != http.StatusOK || failing.Code != http.StatusOK {
		t.Fatalf("statuses = %d / %d, want both 200", clean.Code, failing.Code)
	}

	if clean.Body.String() != failing.Body.String() {
		t.Error("response differs between a clean send and an infra failure — leaks information")
	}

	if !strings.Contains(clean.Body.String(), "Check your inbox") {
		t.Error("confirmation missing")
	}
}

func TestHandler_RequestLinkDispatchesOffResponsePath(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{}
	h := newHandler(t, svc, mockUserReader{})

	rr := requestLink(t, h, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	// The dispatch (run synchronously in tests) still resolves the submitted
	// email — it just happens after the response is rendered, so the response
	// timing is independent of the email.
	if svc.gotEmail != testEmail {
		t.Errorf("dispatched email = %q, want %q", svc.gotEmail, testEmail)
	}
}

func TestHandler_RequestLinkDispatchRunsInGoroutine(t *testing.T) {
	t.Parallel()

	// Use the real (goroutine) async runner — no RunDispatchSynchronously — and
	// confirm the dispatch still reaches the service.
	r, err := web.NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	calls := make(chan string, 1)
	svc := &mockAuthService{calls: calls}
	h := identity.NewHandler(identity.HandlerConfig{
		Service: svc, Users: mockUserReader{}, Renderer: r,
		Logger: quietLogger(), CookieSecure: true, SessionTTL: time.Hour,
	})

	requestLink(t, h, false)

	select {
	case got := <-calls:
		if got != testEmail {
			t.Errorf("dispatched email = %q, want %q", got, testEmail)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch goroutine never called RequestMagicLink")
	}
}

func TestHandler_RequestLinkHTMXSwapsFragment(t *testing.T) {
	t.Parallel()

	h := newHandler(t, &mockAuthService{}, mockUserReader{})

	frag := requestLink(t, h, true).Body.String()
	if strings.Contains(frag, "<!doctype html>") {
		t.Error("HTMX response should be a bare fragment, not a full page")
	}

	full := requestLink(t, h, false).Body.String()
	if !strings.Contains(full, "<!doctype html>") {
		t.Error("non-HTMX response should be a full page")
	}
}

func TestHandler_RedeemAdminSetsCookieAndRedirects(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{redeemSess: identity.Session{ID: "sess-1", Kind: identity.KindAdmin}}
	h := newHandler(t, svc, mockUserReader{})

	rr := httptest.NewRecorder()
	h.Redeem().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/redeem?token=abc", nil))

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}

	if loc := rr.Header().Get("Location"); loc != "/admin" {
		t.Errorf("Location = %q, want /admin", loc)
	}

	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}

	c := cookies[0]
	if c.Name != identity.SessionCookie || c.Value != "sess-1" {
		t.Errorf("cookie = %s=%s, want %s=sess-1", c.Name, c.Value, identity.SessionCookie)
	}

	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie attrs HttpOnly=%v Secure=%v SameSite=%v, want true/true/Lax", c.HttpOnly, c.Secure, c.SameSite)
	}

	if c.MaxAge != int((time.Hour).Seconds()) {
		t.Errorf("cookie MaxAge = %d, want %d", c.MaxAge, int((time.Hour).Seconds()))
	}
}

func TestHandler_RedeemConsumerRedirectsToPortal(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{redeemSess: identity.Session{ID: "sess-2", Kind: identity.KindConsumer}}
	h := newHandler(t, svc, mockUserReader{})

	rr := httptest.NewRecorder()
	h.Redeem().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/redeem?token=abc", nil))

	if loc := rr.Header().Get("Location"); loc != "/portal" {
		t.Errorf("Location = %q, want /portal", loc)
	}
}

func TestHandler_RedeemInvalidTokenRendersRecovery(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{redeemErr: identity.ErrTokenInvalidOrUsed}
	h := newHandler(t, svc, mockUserReader{})

	rr := httptest.NewRecorder()
	h.Redeem().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/redeem?token=used", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "no longer valid") {
		t.Error("recovery page missing")
	}

	if len(rr.Result().Cookies()) != 0 {
		t.Error("no session cookie should be set for an invalid token")
	}
}

func TestHandler_RedeemInfraErrorIs500(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{redeemErr: errors.New("redis down")}
	h := newHandler(t, svc, mockUserReader{})

	rr := httptest.NewRecorder()
	h.Redeem().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/redeem?token=x", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestHandler_LogoutClearsCookieAndDeletesSession(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{}
	h := newHandler(t, svc, mockUserReader{})

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})
	rr := httptest.NewRecorder()

	h.Logout().ServeHTTP(rr, req)

	if svc.loggedOut != "sess-1" {
		t.Errorf("deleted session = %q, want sess-1", svc.loggedOut)
	}

	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
		t.Errorf("logout should redirect to /login (got %d %q)", rr.Code, rr.Header().Get("Location"))
	}

	c := rr.Result().Cookies()[0]
	if c.MaxAge >= 0 {
		t.Errorf("logout cookie MaxAge = %d, want negative (cleared)", c.MaxAge)
	}
}

func TestHandler_LogoutWithoutCookie(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{}
	h := newHandler(t, svc, mockUserReader{})

	rr := httptest.NewRecorder()
	h.Logout().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/logout", nil))

	if svc.loggedOut != "" {
		t.Error("no session delete should happen without a cookie")
	}

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}
}

func TestHandler_LogoutDeleteErrorStillRedirects(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{logoutErr: errors.New("redis down")}
	h := newHandler(t, svc, mockUserReader{})

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})
	rr := httptest.NewRecorder()

	h.Logout().ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 even when delete fails", rr.Code)
	}
}

// authed wraps a gated handler with Authenticate carrying the given session.
func authed(handler http.Handler, sess identity.Session) http.Handler {
	return identity.Authenticate(mockSessionGetter{sess: sess})(handler)
}

func TestHandler_AdminHomeScopedRead(t *testing.T) {
	t.Parallel()

	svc := &mockAuthService{}
	users := mockUserReader{user: identity.User{Email: "a@series-a.test", Role: identity.RoleSeasonAdmin}}
	h := newHandler(t, svc, users)

	sess := identity.Session{UserID: "user-1", OrgID: "org-1", Role: identity.RoleSeasonAdmin, Kind: identity.KindAdmin}

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})
	rr := httptest.NewRecorder()

	authed(h.AdminHome(), sess).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "a@series-a.test") {
		t.Error("admin home should show the scoped-read user's email")
	}
}

func TestHandler_PortalHomeScopedRead(t *testing.T) {
	t.Parallel()

	users := mockUserReader{user: identity.User{Email: "j@autosport.test", Role: identity.RoleConsumer}}
	h := newHandler(t, &mockAuthService{}, users)

	sess := identity.Session{UserID: "user-2", OrgID: "org-2", Role: identity.RoleConsumer, Kind: identity.KindConsumer}

	req := httptest.NewRequest(http.MethodGet, "/portal", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-2"})
	rr := httptest.NewRecorder()

	authed(h.PortalHome(), sess).ServeHTTP(rr, req)

	if !strings.Contains(rr.Body.String(), "granted access") {
		t.Error("portal home should render the granted landing")
	}
}

func TestHandler_HomeWithoutIdentityRedirects(t *testing.T) {
	t.Parallel()

	h := newHandler(t, &mockAuthService{}, mockUserReader{})

	rr := httptest.NewRecorder()
	h.AdminHome().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect when unauthenticated", rr.Code)
	}
}

func TestHandler_HomeGetUserErrorIs500(t *testing.T) {
	t.Parallel()

	users := mockUserReader{err: errors.New("db down")}
	h := newHandler(t, &mockAuthService{}, users)

	sess := identity.Session{UserID: "user-1", OrgID: "org-1", Role: identity.RoleSeasonAdmin, Kind: identity.KindAdmin}

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: identity.SessionCookie, Value: "sess-1"})
	rr := httptest.NewRecorder()

	authed(h.AdminHome(), sess).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the scoped read fails", rr.Code)
	}
}
