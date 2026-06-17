package identity

import "context"

type ctxKey int

const identityKey ctxKey = iota

// Identity is the authenticated caller, derived from their session and injected
// into the request context by Authenticate. Handlers read it to gate access and
// to scope tenant queries via postgres.WithOrg using OrgID.
type Identity struct {
	UserID    string
	OrgID     string
	Role      Role
	Kind      Kind
	Scope     string
	CSRFToken string
}

func withIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFromContext returns the authenticated caller, or false if the request
// is unauthenticated.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)

	return id, ok
}
