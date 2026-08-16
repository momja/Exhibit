package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIDP is a minimal but spec-shaped OIDC provider: a discovery document, a
// JWKS, and a token endpoint that issues a signed id_token. It exists so the
// PKCE flow is exercised end to end without a network or a vendor account —
// the same reason discovery is the configuration surface in the first place.
type fakeIDP struct {
	server *httptest.Server
	key    *rsa.PrivateKey

	clientID string
	subject  string
	email    string

	// recorded from the token request, so a test can assert the verifier
	// actually travelled and matched the challenge.
	gotVerifier string
	gotCode     string
	// challenge the authorization request advertised, set by the test.
	expectChallenge string
	// nextIDToken, when set, is returned by /token instead of a freshly
	// signed, well-formed token — how a test hands the exchange a token the
	// verifier should reject.
	nextIDToken string
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	idp := &fakeIDP{key: key, clientID: "exhibit-test", subject: "user-abc", email: "person@example.test"}
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                idp.server.URL,
			"authorization_endpoint":                idp.server.URL + "/authorize",
			"token_endpoint":                        idp.server.URL + "/token",
			"jwks_uri":                              idp.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := key.Public().(*rsa.PublicKey)
		writeJSON(w, map[string]any{"keys": []map[string]any{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": "test-key",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		idp.gotCode = r.Form.Get("code")
		idp.gotVerifier = r.Form.Get("code_verifier")
		if idp.expectChallenge != "" && S256Challenge(idp.gotVerifier) != idp.expectChallenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		idToken := idp.nextIDToken
		if idToken == "" {
			idToken = idp.signIDToken(t)
		}
		writeJSON(w, map[string]any{
			"access_token": "at",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
	})

	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	return idp
}

// signIDToken mints an RS256 JWT by hand rather than pulling in a JOSE
// library: the whole token is three base64url segments and one signature, and
// keeping it inline makes what the verifier actually checks visible.
func (f *fakeIDP) signIDToken(t *testing.T) string {
	return f.signIDTokenCustom(t, f.key, nil)
}

// signIDTokenCustom mints a token with the given signing key and claim
// overrides, so tests can hand the verifier a token it should reject: one
// signed by a key absent from the published JWKS, or carrying a foreign
// audience or an already-expired exp.
func (f *fakeIDP) signIDTokenCustom(t *testing.T, key *rsa.PrivateKey, claimOverrides map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": "test-key"}
	now := time.Now()
	claims := map[string]any{
		"iss":   f.server.URL,
		"aud":   f.clientID,
		"sub":   f.subject,
		"email": f.email,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	for k, v := range claimOverrides {
		claims[k] = v
	}
	segment := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signing := segment(header) + "." + segment(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	require.NoError(t, err)
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestOIDCProviderDiscoversAndExchanges(t *testing.T) {
	idp := newFakeIDP(t)

	// Discovery only: the issuer is the sole endpoint configured.
	provider, err := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer:       idp.server.URL,
		ClientID:     idp.clientID,
		ClientSecret: "shh",
		RedirectURL:  "https://app.test/auth/callback",
	})
	require.NoError(t, err)

	verifier, err := NewVerifier()
	require.NoError(t, err)
	state, err := NewState()
	require.NoError(t, err)

	authURL, err := url.Parse(provider.AuthURL(state, verifier))
	require.NoError(t, err)
	q := authURL.Query()
	assert.Equal(t, idp.server.URL+"/authorize", authURL.Scheme+"://"+authURL.Host+authURL.Path)
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, state, q.Get("state"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, S256Challenge(verifier), q.Get("code_challenge"),
		"the challenge must be derived from the verifier the caller holds")
	assert.Contains(t, q.Get("scope"), "openid")

	// The provider will reject an exchange whose verifier does not hash to
	// the challenge it saw, so this asserts PKCE end to end.
	idp.expectChallenge = q.Get("code_challenge")

	identity, err := provider.Exchange(context.Background(), "auth-code-123", verifier)
	require.NoError(t, err)
	assert.Equal(t, idp.subject, identity.ExternalID)
	assert.Equal(t, idp.email, identity.Email)
	assert.Equal(t, "auth-code-123", idp.gotCode)
	assert.Equal(t, verifier, idp.gotVerifier)
}

func TestOIDCProviderRejectsWrongVerifier(t *testing.T) {
	idp := newFakeIDP(t)
	provider, err := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer:      idp.server.URL,
		ClientID:    idp.clientID,
		RedirectURL: "https://app.test/auth/callback",
	})
	require.NoError(t, err)

	verifier, err := NewVerifier()
	require.NoError(t, err)
	idp.expectChallenge = S256Challenge(verifier)

	other, err := NewVerifier()
	require.NoError(t, err)
	_, err = provider.Exchange(context.Background(), "code", other)
	assert.Error(t, err, "a code redeemed with the wrong verifier must not yield an identity")
}

func TestOIDCProviderRejectsForeignAudience(t *testing.T) {
	idp := newFakeIDP(t)
	provider, err := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer:      idp.server.URL,
		ClientID:    idp.clientID,
		RedirectURL: "https://app.test/auth/callback",
	})
	require.NoError(t, err)

	verifier, err := NewVerifier()
	require.NoError(t, err)
	idp.expectChallenge = S256Challenge(verifier)
	idp.nextIDToken = idp.signIDTokenCustom(t, idp.key, map[string]any{"aud": "some-other-client"})

	_, err = provider.Exchange(context.Background(), "code", verifier)
	assert.Error(t, err, "an id_token minted for a different client must not verify")
}

func TestOIDCProviderRejectsExpiredToken(t *testing.T) {
	idp := newFakeIDP(t)
	provider, err := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer:      idp.server.URL,
		ClientID:    idp.clientID,
		RedirectURL: "https://app.test/auth/callback",
	})
	require.NoError(t, err)

	verifier, err := NewVerifier()
	require.NoError(t, err)
	idp.expectChallenge = S256Challenge(verifier)
	idp.nextIDToken = idp.signIDTokenCustom(t, idp.key, map[string]any{
		"exp": time.Now().Add(-time.Hour).Unix(),
	})

	_, err = provider.Exchange(context.Background(), "code", verifier)
	assert.Error(t, err, "an expired id_token must not verify")
}

func TestOIDCProviderRejectsUnknownSigningKey(t *testing.T) {
	idp := newFakeIDP(t)
	provider, err := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer:      idp.server.URL,
		ClientID:    idp.clientID,
		RedirectURL: "https://app.test/auth/callback",
	})
	require.NoError(t, err)

	verifier, err := NewVerifier()
	require.NoError(t, err)
	idp.expectChallenge = S256Challenge(verifier)

	forgedKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	idp.nextIDToken = idp.signIDTokenCustom(t, forgedKey, nil)

	_, err = provider.Exchange(context.Background(), "code", verifier)
	assert.Error(t, err, "a token signed by a key absent from the published JWKS must not verify")
}

func TestOIDCProviderNeedsConfiguration(t *testing.T) {
	_, err := NewOIDCProvider(context.Background(), OIDCConfig{ClientID: "x", RedirectURL: "y"})
	assert.Error(t, err, "no issuer")
	_, err = NewOIDCProvider(context.Background(), OIDCConfig{Issuer: "https://idp.test", RedirectURL: "y"})
	assert.Error(t, err, "no client id")
	_, err = NewOIDCProvider(context.Background(), OIDCConfig{Issuer: "https://idp.test", ClientID: "x"})
	assert.Error(t, err, "no redirect url")
}

func TestS256ChallengeIsStable(t *testing.T) {
	// RFC 7636 appendix B's worked example.
	assert.Equal(t,
		"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		S256Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"))
}

func TestRandomValuesAreDistinct(t *testing.T) {
	a, err := NewSessionID()
	require.NoError(t, err)
	b, err := NewSessionID()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	assert.GreaterOrEqual(t, len(a), 43, "32 random bytes, base64url-encoded")
}
