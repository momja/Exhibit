// Package auth is the seam between Exhibit and whatever proves who a visitor
// is (av-30rj).
//
// The whole vendor surface is two methods, because the identity provider is a
// login-time concern only: a browser is sent to the provider, comes back with
// a code, and that code is exchanged exactly once for an Identity. From there
// on the request is authenticated by a session this service owns — see the
// session layer in internal/api.
//
// Verifying a provider-signed token on every request would be the other
// shape, and it is the wrong one for a server-rendered app: it puts a network
// check on the request path, and it makes logout impossible, because a signed
// token stays valid until its TTL no matter what the provider (or the user)
// later decides. Owning the session fixes both.
//
// Because the exchange happens once, adding a provider is writing a
// constructor. Nothing downstream — cookies, sessions, owner_id — knows which
// provider produced the identity it is serving.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// Identity is everything this service takes from a provider: a stable subject
// and an address.
//
// ExternalID is the provider's own subject claim. It is unique within that
// provider and meaningless outside it, which is why Email travels with it —
// an instance that changes provider has no other way to recognize a returning
// person, and adding the field afterwards is far more expensive than carrying
// it from the start.
type Identity struct {
	ExternalID string
	Email      string
}

// IdentityProvider is the seam. Implementations do the provider-specific work
// of building an authorization URL and redeeming the code that comes back;
// everything else in the system is provider-agnostic.
//
// state is the caller's CSRF token, echoed back to the callback. verifier is
// the PKCE code verifier: AuthURL derives the challenge from it and Exchange
// presents it, so the caller never has to know which challenge method the
// provider negotiated.
type IdentityProvider interface {
	AuthURL(state, verifier string) string
	Exchange(ctx context.Context, code, verifier string) (*Identity, error)
}

// randomString returns n bytes of crypto-random data, base64url-encoded
// without padding — the encoding PKCE and opaque session ids both want.
func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewState mints the CSRF token round-tripped through the provider.
func NewState() (string, error) { return randomString(32) }

// NewVerifier mints a PKCE code verifier. 32 random bytes encode to 43
// characters, the minimum RFC 7636 permits and the length every provider
// accepts.
func NewVerifier() (string, error) { return randomString(32) }

// NewSessionID mints the opaque value that goes in the session cookie. It is
// random rather than derived: the cookie should say nothing about the user it
// belongs to, and 32 bytes is far past guessing range.
func NewSessionID() (string, error) { return randomString(32) }

// S256Challenge is the PKCE challenge for a verifier. Exported because a
// provider implementation that does not use x/oauth2 still needs it, and a
// test double asserting the flow wants to check it.
func S256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
