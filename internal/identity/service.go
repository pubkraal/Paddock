package identity

import (
	"context"
	"errors"
	"net/url"
	"time"
)

// The Service depends on these narrow interfaces (defined at the consumer) so
// handlers and tests inject the concrete stores or mocks.
type (
	repository interface {
		Lookup(ctx context.Context, email string) (User, error)
	}

	tokenStore interface {
		Issue(ctx context.Context, tok MagicLinkToken) (string, error)
		Redeem(ctx context.Context, raw string) (MagicLinkToken, error)
	}

	sessionStore interface {
		Create(ctx context.Context, s Session) (string, error)
		Delete(ctx context.Context, id string) error
	}

	linkSender interface {
		SendMagicLink(ctx context.Context, to, link string) error
	}
)

// Service is the identity application layer: request a magic link, redeem it for
// a session, and log out.
type Service struct {
	repo     repository
	tokens   tokenStore
	sessions sessionStore
	links    linkSender
	baseURL  string
	now      func() time.Time
}

// NewService wires the Service. now is injected so token timestamps are
// deterministic in tests.
func NewService(
	repo repository, tokens tokenStore, sessions sessionStore, links linkSender,
	baseURL string, now func() time.Time,
) *Service {
	return &Service{repo: repo, tokens: tokens, sessions: sessions, links: links, baseURL: baseURL, now: now}
}

// RequestMagicLink resolves the email and, for an active user, issues a
// single-use token and emails the link. It returns nil whether or not the email
// exists or is active — the caller must not reveal that (anti-enumeration,
// ADR-0012). Only a genuine infrastructure failure for an existing active user
// surfaces as an error.
func (s *Service) RequestMagicLink(ctx context.Context, email string) error {
	user, err := s.repo.Lookup(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		return nil
	}

	if err != nil {
		return err
	}

	if !user.IsActive() {
		return nil
	}

	purpose := PurposeAdminLogin
	if user.Role == RoleConsumer {
		purpose = PurposeConsumerGrant
	}

	raw, err := s.tokens.Issue(ctx, MagicLinkToken{
		UserID:   user.ID,
		OrgID:    user.OrgID,
		Role:     user.Role,
		Purpose:  purpose,
		IssuedAt: s.now().UTC(),
	})
	if err != nil {
		return err
	}

	link := s.baseURL + "/auth/redeem?token=" + url.QueryEscape(raw)

	return s.links.SendMagicLink(ctx, email, link)
}

// Redeem consumes a magic-link token and creates the session it grants. The
// token's purpose determines the session kind: a durable admin session or a
// scoped consumer grant (ADR-0013).
func (s *Service) Redeem(ctx context.Context, raw string) (Session, error) {
	tok, err := s.tokens.Redeem(ctx, raw)
	if err != nil {
		return Session{}, err
	}

	kind := KindAdmin
	if tok.Purpose == PurposeConsumerGrant {
		kind = KindConsumer
	}

	sess := Session{
		UserID: tok.UserID,
		OrgID:  tok.OrgID,
		Role:   tok.Role,
		Kind:   kind,
		Scope:  tok.Scope,
	}

	id, err := s.sessions.Create(ctx, sess)
	if err != nil {
		return Session{}, err
	}

	sess.ID = id

	return sess, nil
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}
