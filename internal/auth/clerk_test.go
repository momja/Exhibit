package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	jose "github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/stretchr/testify/require"
)

func TestClerkVerifiesBrowserSession(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwk := jose.JSONWebKey{Key: &key.PublicKey, KeyID: "exhibit-clerk-test", Algorithm: "RS256", Use: "sig"}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk_test_example", r.Header.Get("Authorization"))
		require.Equal(t, "/jwks", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}))
	}))
	defer backend.Close()
	config := &clerk.ClientConfig{}
	config.Key = clerk.String("sk_test_example")
	config.URL = clerk.String(backend.URL)
	c, err := newClerk("pk_test_"+base64.RawStdEncoding.EncodeToString([]byte("example.clerk.accounts.dev$")), "https://app.test", config)
	require.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", jwk.KeyID))
	require.NoError(t, err)
	tests := []struct {
		name, issuer, party, status string
		expired                     bool
		want                        string
	}{
		{"valid", c.FrontendAPI, "https://app.test", "active", false, "clerk:user_example"},
		{"wrong origin", c.FrontendAPI, "https://attacker.test", "active", false, ""},
		{"missing origin", c.FrontendAPI, "", "active", false, ""},
		{"wrong issuer", "https://other.clerk.accounts.dev", "https://app.test", "active", false, ""},
		{"pending session", c.FrontendAPI, "https://app.test", "pending", false, ""},
		{"expired", c.FrontendAPI, "https://app.test", "active", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiry := time.Now().Add(time.Minute)
			if tt.expired {
				expiry = time.Now().Add(-time.Minute)
			}
			token, err := jwt.Signed(signer).Claims(map[string]any{"iss": tt.issuer, "sub": "user_example", "sid": "sess_example", "azp": tt.party, "sts": tt.status, "exp": expiry.Unix(), "iat": time.Now().Add(-2 * time.Minute).Unix()}).CompactSerialize()
			require.NoError(t, err)
			r := httptest.NewRequest("GET", "/", nil)
			r.AddCookie(&http.Cookie{Name: "__session", Value: token})
			called := 0
			c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called++
				require.Equal(t, tt.want, c.Subject(r))
				if tt.name == "valid" {
					require.InDelta(t, time.Minute.Seconds(), c.SessionExpiresIn(r).Seconds(), 2)
				}
				if tt.expired {
					require.Zero(t, c.SessionExpiresIn(r))
				}
			})).ServeHTTP(httptest.NewRecorder(), r)
			require.Equal(t, 1, called)
		})
	}
	r := httptest.NewRequest("POST", "/api/artifacts", nil)
	r.AddCookie(&http.Cookie{Name: "__session", Value: "invalid"})
	r.Header.Set("Origin", "https://attacker.test")
	w := httptest.NewRecorder()
	c.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("cross-site request reached handler") })).ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestClerkIncompleteConfigurationFails(t *testing.T) {
	_, err := NewClerk("", "", "https://app.test", "https://render.test")
	require.Error(t, err)
	_, err = NewClerk("pk_test_invalid", "sk_test_example", "https://app.test", "https://render.test")
	require.Error(t, err)
}

func TestClerkRequiresCookieIsolation(t *testing.T) {
	_, err := NewClerk("pk_test_example", "sk_test_example", "http://localhost:8080", "http://localhost:8081")
	require.ErrorContains(t, err, "cookies are not isolated by port")
}
