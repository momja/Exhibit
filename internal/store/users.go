package store

import "time"

// User is one identity that has logged in to this instance (av-30rj).
//
// ID is the integer every other table already means by `owner_id`; nothing
// outside this table ever references a provider-specific identifier, so
// changing identity provider is a re-link of these rows rather than a
// migration of the whole schema.
//
// ExternalID is the provider's subject claim — unique per provider, and
// meaningless across providers, which is exactly why Email is stored too: it
// is the portable key that lets an instance recognize a returning person after
// its IdP changes. For a local account (av-rzvf) there is no provider, so
// ExternalID is derived from the login name instead — see auth.LocalExternalID.
//
// The stored password hash is deliberately *not* a field here. A User travels
// widely — page view models, log attributes, future JSON — and the one caller
// that needs the hash is the login path, which asks for it by name through
// LookupLocalCredential. HasPassword carries the only thing every other caller
// wants to know, which is whether this account can log in with a password at
// all.
type User struct {
	ID         int64     `json:"id"`
	ExternalID string    `json:"external_id"`
	Email      string    `json:"email"`
	CreatedAt  time.Time `json:"created_at"`
	// IsAdmin marks the person who administers the instance. The first user
	// on an instance gets it; see sqlite_users.go for why that is applied at
	// insert time rather than checked by a caller.
	IsAdmin bool `json:"is_admin"`
	// HasPassword distinguishes a local account from an OIDC identity — the
	// two live in this one table and differ only by which columns are
	// populated, which is what keeps them in one owner_id space.
	HasPassword bool `json:"has_password"`
	// Disabled is an admin's "this account may not sign in" (av-utap),
	// derived from a nullable `disabled_at` so it applies to an OIDC identity
	// as readily as to a local one — migration 017 says why that mattered
	// enough to earn a column.
	//
	// It is not the whole of the mechanism. Refusing a *login* only stops the
	// next one; the sessions already issued are what a person is actually
	// using, so SetUserDisabled deletes those rows in the same transaction.
	// This field is the durable half, the deleted sessions the immediate one.
	Disabled bool `json:"disabled"`
	// Entitlement is what this owner is allowed (av-2p8z) — as *stored*, so
	// a nil limit here means "none of their own", not "none at all".
	// Resolving it against the instance's default is one function elsewhere
	// (api.Router.resolveAllowance); nothing should read these fields to
	// decide whether to refuse something.
	//
	// It travels with the User for the same reason IsAdmin does: the admin
	// directory renders a row per account and would otherwise issue a query
	// per row. Unlike the password hash, none of it is a credential.
	Entitlement Entitlement `json:"entitlement"`
}

// NewLocalUser is CreateLocalUser's argument: the three strings that name a
// local account. A struct rather than three positional string parameters,
// because ExternalID, Email, and PasswordHash are same-typed values in a row —
// exactly the shape a caller can transpose without the compiler catching it.
type NewLocalUser struct {
	ExternalID   string
	Email        string
	PasswordHash string
}

// Session is one logged-in browser. Its ID is opaque random bytes handed to
// the browser in a cookie and looked up here on every request — a row, not a
// signed token, so deleting it revokes access on the next request instead of
// whenever a token would have expired.
type Session struct {
	// ID is the bearer credential itself — the exact value the cookie carries
	// — so it is excluded from default JSON marshaling. Nothing today
	// marshals a Session directly, but a Session reachable from a future admin
	// endpoint or debug log line should not be one struct-literal accident
	// away from writing session ids into a response or a log.
	ID        string    `json:"-"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
