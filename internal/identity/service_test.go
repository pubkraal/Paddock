package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/identity"
)

type mockRepo struct {
	user     identity.User
	err      error
	gotEmail string
}

func (m *mockRepo) Lookup(_ context.Context, email string) (identity.User, error) {
	m.gotEmail = email

	return m.user, m.err
}

type mockTokens struct {
	raw         string
	issueErr    error
	issued      identity.MagicLinkToken
	issueCalled bool
	redeemTok   identity.MagicLinkToken
	redeemErr   error
}

func (m *mockTokens) Issue(_ context.Context, tok identity.MagicLinkToken) (string, error) {
	m.issueCalled = true
	m.issued = tok

	return m.raw, m.issueErr
}

func (m *mockTokens) Redeem(_ context.Context, _ string) (identity.MagicLinkToken, error) {
	return m.redeemTok, m.redeemErr
}

type mockSessions struct {
	id           string
	createErr    error
	created      identity.Session
	createCalled bool
	deleted      string
	deleteErr    error
}

func (m *mockSessions) Create(_ context.Context, s identity.Session) (string, error) {
	m.createCalled = true
	m.created = s

	return m.id, m.createErr
}

func (m *mockSessions) Delete(_ context.Context, id string) error {
	m.deleted = id

	return m.deleteErr
}

type mockLinks struct {
	to     string
	link   string
	called bool
	err    error
}

func (m *mockLinks) SendMagicLink(_ context.Context, to, link string) error {
	m.called = true
	m.to = to
	m.link = link

	return m.err
}

func fixedClock() func() time.Time {
	return func() time.Time { return time.Unix(1700000000, 0).UTC() }
}

func newService(repo *mockRepo, tokens *mockTokens, sessions *mockSessions, links *mockLinks) *identity.Service {
	return identity.NewService(repo, tokens, sessions, links, "https://paddock.test", fixedClock())
}

func activeUser(role identity.Role) identity.User {
	return identity.User{ID: "user-1", OrgID: "org-1", Email: "press@example.test", Role: role, Status: identity.StatusActive}
}

func TestService_RequestMagicLink_ActiveAdmin(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{user: activeUser(identity.RolePressOfficer)}
	tokens := &mockTokens{raw: "RAWTOKEN"}
	links := &mockLinks{}

	if err := newService(repo, tokens, &mockSessions{}, links).
		RequestMagicLink(context.Background(), "press@example.test"); err != nil {
		t.Fatalf("RequestMagicLink: %v", err)
	}

	if !tokens.issueCalled {
		t.Fatal("token was not issued for an active user")
	}

	if tokens.issued.Purpose != identity.PurposeAdminLogin {
		t.Errorf("purpose = %q, want admin_login", tokens.issued.Purpose)
	}

	if tokens.issued.IssuedAt != time.Unix(1700000000, 0).UTC() {
		t.Errorf("IssuedAt = %v, want the injected clock", tokens.issued.IssuedAt)
	}

	if !links.called || links.to != "press@example.test" {
		t.Errorf("link not sent to the user (called=%v to=%q)", links.called, links.to)
	}

	if !strings.Contains(links.link, "https://paddock.test/auth/redeem?token=RAWTOKEN") {
		t.Errorf("link = %q, want the redeem URL with the raw token", links.link)
	}
}

func TestService_RequestMagicLink_ActiveConsumerUsesGrantPurpose(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{user: activeUser(identity.RoleConsumer)}
	tokens := &mockTokens{raw: "RAWTOKEN"}

	if err := newService(repo, tokens, &mockSessions{}, &mockLinks{}).
		RequestMagicLink(context.Background(), "press@example.test"); err != nil {
		t.Fatalf("RequestMagicLink: %v", err)
	}

	if tokens.issued.Purpose != identity.PurposeConsumerGrant {
		t.Errorf("purpose = %q, want consumer_grant", tokens.issued.Purpose)
	}
}

func TestService_RequestMagicLink_UnknownEmailIsSilent(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{err: identity.ErrUserNotFound}
	tokens := &mockTokens{}
	links := &mockLinks{}

	if err := newService(repo, tokens, &mockSessions{}, links).
		RequestMagicLink(context.Background(), "ghost@example.test"); err != nil {
		t.Fatalf("RequestMagicLink should be silent for unknown email, got: %v", err)
	}

	if tokens.issueCalled || links.called {
		t.Error("no token or email should be produced for an unknown email")
	}
}

func TestService_RequestMagicLink_DisabledUserIsSilent(t *testing.T) {
	t.Parallel()

	user := activeUser(identity.RolePressOfficer)
	user.Status = identity.StatusDisabled

	repo := &mockRepo{user: user}
	tokens := &mockTokens{}
	links := &mockLinks{}

	if err := newService(repo, tokens, &mockSessions{}, links).
		RequestMagicLink(context.Background(), "press@example.test"); err != nil {
		t.Fatalf("RequestMagicLink should be silent for disabled user, got: %v", err)
	}

	if tokens.issueCalled || links.called {
		t.Error("no token or email should be produced for a disabled user")
	}
}

func TestService_RequestMagicLink_LookupError(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{err: errors.New("db down")}

	err := newService(repo, &mockTokens{}, &mockSessions{}, &mockLinks{}).
		RequestMagicLink(context.Background(), "press@example.test")
	if err == nil {
		t.Fatal("expected lookup error to surface, got nil")
	}
}

func TestService_RequestMagicLink_IssueError(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{user: activeUser(identity.RolePressOfficer)}
	tokens := &mockTokens{issueErr: errors.New("redis down")}
	links := &mockLinks{}

	err := newService(repo, tokens, &mockSessions{}, links).
		RequestMagicLink(context.Background(), "press@example.test")
	if err == nil {
		t.Fatal("expected issue error to surface, got nil")
	}

	if links.called {
		t.Error("no email should be sent when issuing fails")
	}
}

func TestService_RequestMagicLink_SendError(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{user: activeUser(identity.RolePressOfficer)}
	tokens := &mockTokens{raw: "RAWTOKEN"}
	links := &mockLinks{err: errors.New("smtp down")}

	err := newService(repo, tokens, &mockSessions{}, links).
		RequestMagicLink(context.Background(), "press@example.test")
	if err == nil {
		t.Fatal("expected send error to surface, got nil")
	}
}

func TestService_Redeem_AdminToken(t *testing.T) {
	t.Parallel()

	tokens := &mockTokens{redeemTok: identity.MagicLinkToken{
		UserID:  "user-1",
		OrgID:   "org-1",
		Role:    identity.RoleSeasonAdmin,
		Purpose: identity.PurposeAdminLogin,
	}}
	sessions := &mockSessions{id: "sess-1"}

	got, err := newService(&mockRepo{}, tokens, sessions, &mockLinks{}).
		Redeem(context.Background(), "RAWTOKEN")
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}

	if got.ID != "sess-1" || got.Kind != identity.KindAdmin || got.Role != identity.RoleSeasonAdmin {
		t.Errorf("session = %+v, want admin session sess-1", got)
	}

	if sessions.created.OrgID != "org-1" {
		t.Errorf("created session OrgID = %q, want org-1", sessions.created.OrgID)
	}
}

func TestService_Redeem_ConsumerToken(t *testing.T) {
	t.Parallel()

	tokens := &mockTokens{redeemTok: identity.MagicLinkToken{
		UserID:  "user-2",
		OrgID:   "org-2",
		Role:    identity.RoleConsumer,
		Purpose: identity.PurposeConsumerGrant,
		Scope:   "event-9",
	}}
	sessions := &mockSessions{id: "sess-2"}

	got, err := newService(&mockRepo{}, tokens, sessions, &mockLinks{}).
		Redeem(context.Background(), "RAWTOKEN")
	if err != nil {
		t.Fatalf("Redeem: %v", err)
	}

	if got.Kind != identity.KindConsumer || got.Scope != "event-9" {
		t.Errorf("session = %+v, want consumer session scoped to event-9", got)
	}
}

func TestService_Redeem_InvalidToken(t *testing.T) {
	t.Parallel()

	tokens := &mockTokens{redeemErr: identity.ErrTokenInvalidOrUsed}
	sessions := &mockSessions{}

	_, err := newService(&mockRepo{}, tokens, sessions, &mockLinks{}).
		Redeem(context.Background(), "USED")
	if !errors.Is(err, identity.ErrTokenInvalidOrUsed) {
		t.Errorf("Redeem error = %v, want ErrTokenInvalidOrUsed", err)
	}

	if sessions.createCalled {
		t.Error("no session should be created for an invalid token")
	}
}

func TestService_Redeem_CreateError(t *testing.T) {
	t.Parallel()

	tokens := &mockTokens{redeemTok: identity.MagicLinkToken{Purpose: identity.PurposeAdminLogin}}
	sessions := &mockSessions{createErr: errors.New("redis down")}

	_, err := newService(&mockRepo{}, tokens, sessions, &mockLinks{}).
		Redeem(context.Background(), "RAWTOKEN")
	if err == nil {
		t.Fatal("expected session-create error to surface, got nil")
	}
}

func TestService_Logout(t *testing.T) {
	t.Parallel()

	sessions := &mockSessions{}

	if err := newService(&mockRepo{}, &mockTokens{}, sessions, &mockLinks{}).
		Logout(context.Background(), "sess-1"); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if sessions.deleted != "sess-1" {
		t.Errorf("deleted session = %q, want sess-1", sessions.deleted)
	}
}
