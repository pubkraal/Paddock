package identity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// The Handler depends on these narrow interfaces, defined at the consumer.
type (
	authService interface {
		RequestMagicLink(ctx context.Context, email string) error
		Redeem(ctx context.Context, raw string) (Session, error)
		Logout(ctx context.Context, sessionID string) error
	}

	userReader interface {
		GetUser(ctx context.Context, orgID, userID string) (User, error)
	}

	renderer interface {
		Render(w io.Writer, page string, data any) error
	}
)

// HandlerConfig wires a Handler. Dependencies are interfaces so cmd/web injects
// the concrete service/repository/renderer and tests inject mocks.
type HandlerConfig struct {
	Service      authService
	Users        userReader
	Renderer     renderer
	Logger       *slog.Logger
	CookieSecure bool
	SessionTTL   time.Duration
}

// Handler serves the Access screens and the magic-link flow.
type Handler struct {
	svc          authService
	users        userReader
	renderer     renderer
	logger       *slog.Logger
	cookieSecure bool
	sessionTTL   time.Duration
}

// NewHandler builds a Handler from its config.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		svc:          cfg.Service,
		users:        cfg.Users,
		renderer:     cfg.Renderer,
		logger:       cfg.Logger,
		cookieSecure: cfg.CookieSecure,
		sessionTTL:   cfg.SessionTTL,
	}
}

type loginData struct {
	Sent  bool
	Email string
}

type homeData struct {
	Email string
	Role  string
	OrgID string
}

// LoginPage renders the sign-in form.
func (h *Handler) LoginPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.render(r.Context(), w, "login", loginData{})
	}
}

// RequestLink issues a magic link for the submitted email and always responds
// with the same confirmation, regardless of whether the email exists or sending
// failed — the anti-enumeration boundary (ADR-0012). Infra failures are logged,
// not surfaced.
func (h *Handler) RequestLink() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.TrimSpace(r.FormValue("email"))

		if err := h.svc.RequestMagicLink(r.Context(), email); err != nil {
			h.logger.ErrorContext(r.Context(), "request magic link", "err", err)
		}

		data := loginData{Sent: true, Email: email}

		if isHTMX(r) {
			h.render(r.Context(), w, "link_sent", data)

			return
		}

		h.render(r.Context(), w, "login", data)
	}
}

// Redeem consumes a magic-link token, sets the session cookie, and redirects to
// the destination for the session kind. An invalid or used token renders the
// recovery page.
func (h *Handler) Redeem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := h.svc.Redeem(r.Context(), r.URL.Query().Get("token"))
		if errors.Is(err, ErrTokenInvalidOrUsed) {
			h.render(r.Context(), w, "redeem_invalid", nil)

			return
		}

		if err != nil {
			h.logger.ErrorContext(r.Context(), "redeem token", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)

			return
		}

		http.SetCookie(w, h.sessionCookie(sess.ID))

		dest := "/admin"
		if sess.Kind == KindConsumer {
			dest = "/portal"
		}

		http.Redirect(w, r, dest, http.StatusSeeOther)
	}
}

// Logout ends the session and clears the cookie.
func (h *Handler) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(SessionCookie); err == nil {
			if err := h.svc.Logout(r.Context(), cookie.Value); err != nil {
				h.logger.ErrorContext(r.Context(), "logout", "err", err)
			}
		}

		http.SetCookie(w, h.clearCookie())
		http.Redirect(w, r, loginPath, http.StatusSeeOther)
	}
}

// AdminHome is the org-staff landing; gate it with Authenticate + RequireAdmin.
func (h *Handler) AdminHome() http.HandlerFunc {
	return h.home("admin_home")
}

// PortalHome is the consumer landing; gate it with Authenticate.
func (h *Handler) PortalHome() http.HandlerFunc {
	return h.home("granted")
}

// home renders the landing page after a scoped read of the caller's own user
// row — proving the WithOrg path end-to-end in the serving tier.
func (h *Handler) home(page string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, loginPath, http.StatusSeeOther)

			return
		}

		user, err := h.users.GetUser(r.Context(), id.OrgID, id.UserID)
		if err != nil {
			h.logger.ErrorContext(r.Context(), "load user", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)

			return
		}

		h.render(r.Context(), w, page, homeData{Email: user.Email, Role: string(user.Role), OrgID: id.OrgID})
	}
}

func (h *Handler) render(ctx context.Context, w http.ResponseWriter, page string, data any) {
	var buf bytes.Buffer

	if err := h.renderer.Render(&buf, page, data); err != nil {
		h.logger.ErrorContext(ctx, "render page", "page", page, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (h *Handler) sessionCookie(id string) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionTTL.Seconds()),
	}
}

func (h *Handler) clearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
