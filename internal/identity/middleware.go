package identity

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

// CSRFHeader is the request header carrying the synchronizer token; CSRFField is
// the equivalent form field for plain (non-HTMX) form posts.
const (
	CSRFHeader = "X-CSRF-Token"
	CSRFField  = "_csrf"
)

// SessionCookie is the name of the opaque session cookie.
const SessionCookie = "paddock_session"

// loginPath is where unauthenticated callers are sent.
const loginPath = "/login"

// sessionGetter is the slice of SessionStore that Authenticate needs.
type sessionGetter interface {
	Get(ctx context.Context, id string) (Session, error)
}

// Authenticate loads the session named by the cookie and injects the caller's
// Identity into the request context. A missing or unresolvable session redirects
// to the login page and does not call the next handler.
func Authenticate(store sessionGetter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookie)
			if err != nil {
				http.Redirect(w, r, loginPath, http.StatusSeeOther)

				return
			}

			sess, err := store.Get(r.Context(), cookie.Value)
			if err != nil {
				http.Redirect(w, r, loginPath, http.StatusSeeOther)

				return
			}

			id := Identity{
				UserID:    sess.UserID,
				OrgID:     sess.OrgID,
				Role:      sess.Role,
				Kind:      sess.Kind,
				Scope:     sess.Scope,
				CSRFToken: sess.CSRFToken,
			}

			next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), id)))
		})
	}
}

// RequireCSRF enforces a synchronizer token on state-changing requests
// (CWE-352). Safe methods pass through. For unsafe methods it compares the token
// from the X-CSRF-Token header — or, for non-multipart bodies, the _csrf form
// field — against the session's token in constant time. It deliberately never
// reads a multipart body (that is the size-capped handler's job), so HTMX
// uploads must send the token via the header. Chain after Authenticate.
func RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) {
			next.ServeHTTP(w, r)

			return
		}

		id, ok := IdentityFromContext(r.Context())
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)

			return
		}

		if !csrfTokenValid(r, id.CSRFToken) {
			http.Error(w, "invalid csrf token", http.StatusForbidden)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func safeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func csrfTokenValid(r *http.Request, want string) bool {
	if want == "" {
		return false
	}

	got := r.Header.Get(CSRFHeader)
	if got == "" && !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		got = r.PostFormValue(CSRFField)
	}

	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// RequireAdmin rejects callers who are not org-staff admins with 403. It must be
// chained after Authenticate.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFromContext(r.Context())
		if !ok || !id.Role.IsAdmin() {
			http.Error(w, "forbidden", http.StatusForbidden)

			return
		}

		next.ServeHTTP(w, r)
	})
}
