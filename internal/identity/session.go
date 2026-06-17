package identity

// Kind distinguishes a durable admin session from a scoped consumer grant
// (ADR-0013). Both are Redis-backed sessions; the kind and scope govern what the
// holder may reach.
type Kind string

// The session kinds.
const (
	KindAdmin    Kind = "admin"
	KindConsumer Kind = "consumer"
)

// Session is an authenticated session. The ID is the opaque cookie value and the
// Redis key; expiry is the Redis TTL, not a field here, so the two never drift.
// Scope is opaque in Phase 1 (the event domain lands in Phase 2). CSRFToken is a
// per-session secret embedded in forms and verified on unsafe requests; unlike
// the session id it is safe to render into HTML (the id stays in an HttpOnly
// cookie).
type Session struct {
	ID        string
	UserID    string
	OrgID     string
	Role      Role
	Kind      Kind
	Scope     string
	CSRFToken string
}
