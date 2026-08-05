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
// its IdP changes.
type User struct {
	ID         int64     `json:"id"`
	ExternalID string    `json:"external_id"`
	Email      string    `json:"email"`
	CreatedAt  time.Time `json:"created_at"`
}

// Session is one logged-in browser. Its ID is opaque random bytes handed to
// the browser in a cookie and looked up here on every request — a row, not a
// signed token, so deleting it revokes access on the next request instead of
// whenever a token would have expired.
type Session struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
