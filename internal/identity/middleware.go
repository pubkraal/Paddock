package identity

import (
	"context"
	"net/http"
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
				UserID: sess.UserID,
				OrgID:  sess.OrgID,
				Role:   sess.Role,
				Kind:   sess.Kind,
				Scope:  sess.Scope,
			}

			next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), id)))
		})
	}
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
