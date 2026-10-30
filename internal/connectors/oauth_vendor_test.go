package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestGitHubOAuthPKCEBoundStateRefreshRotationAndRevoke(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var exchanges, refreshes, revocations atomic.Int32
	var challenge string
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "operator-client", r.Form.Get("client_id"))
			assert.Equal(t, "operator-secret", r.Form.Get("client_secret"))
			if r.Form.Get("grant_type") == "authorization_code" {
				exchanges.Add(1)
				assert.Equal(t, "test-code", r.Form.Get("code"))
				digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
				assert.Equal(t, challenge, base64.RawURLEncoding.EncodeToString(digest[:]))
				fmt.Fprint(w, `{"access_token":"github-first-token-3749","token_type":"bearer","refresh_token":"github-first-refresh-7682","expires_in":3600}`)
			} else {
				refreshes.Add(1)
				assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
				assert.Equal(t, "github-first-refresh-7682", r.Form.Get("refresh_token"))
				fmt.Fprint(w, `{"access_token":"github-rotated-token-8251","token_type":"bearer","refresh_token":"github-rotated-refresh-1094","expires_in":3600}`)
			}
		case "/user":
			assert.Equal(t, "Bearer github-rotated-token-8251", r.Header.Get("Authorization"))
			fmt.Fprint(w, `{"login":"octocat","echo":"github-rotated-token-8251"}`)
		case "/revoke":
			revocations.Add(1)
			assert.Equal(t, http.MethodDelete, r.Method)
			u, p, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "operator-client", u)
			assert.Equal(t, "operator-secret", p)
			var input map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
			assert.Equal(t, "github-rotated-token-8251", input["access_token"])
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected OAuth endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer api.Close()
	s.cfg.OAuthEnabled = true
	s.cfg.GitHub = OAuthProvider{ClientID: "operator-client", ClientSecret: "operator-secret", AuthURL: api.URL + "/authorize", TokenURL: api.URL + "/token", APIOrigin: api.URL, RevokeURL: api.URL + "/revoke", Scopes: []string{"read:user"}}
	begin := func() *url.URL {
		destination, err := s.BeginOAuth(ctx, 1, "login-session:browser-cookie", "GitHub personal", "https://app.example.test/callback")
		require.NoError(t, err)
		u, err := url.Parse(destination)
		require.NoError(t, err)
		return u
	}
	bad := begin()
	_, err := s.CompleteOAuth(ctx, 2, "login-session:browser-cookie", bad.Query().Get("state"), "test-code", UnlimitedLimits())
	require.Error(t, err)
	assert.Zero(t, exchanges.Load())
	bad = begin()
	_, err = s.CompleteOAuth(ctx, 1, "another-session:browser-cookie", bad.Query().Get("state"), "test-code", UnlimitedLimits())
	require.Error(t, err)
	assert.Zero(t, exchanges.Load())
	good := begin()
	assert.Equal(t, "S256", good.Query().Get("code_challenge_method"))
	challenge = good.Query().Get("code_challenge")
	require.NotEmpty(t, challenge)
	c, err := s.CompleteOAuth(ctx, 1, "login-session:browser-cookie", good.Query().Get("state"), "test-code", UnlimitedLimits())
	require.NoError(t, err)
	assert.Equal(t, "oauth", c.Kind)
	assert.Equal(t, int32(1), exchanges.Load())
	_, err = s.CompleteOAuth(ctx, 1, "login-session:browser-cookie", good.Query().Get("state"), "test-code", UnlimitedLimits())
	assert.Error(t, err)
	rec, err := s.cfg.Store.GetConnection(ctx, 1, c.ID)
	require.NoError(t, err)
	_, credential, err := s.readCredential(rec)
	require.NoError(t, err)
	var token oauth2.Token
	require.NoError(t, json.Unmarshal(credential.OAuthToken, &token))
	token.Expiry = time.Now().Add(-time.Hour)
	credential.OAuthToken, _ = json.Marshal(token)
	raw, _ := json.Marshal(credential)
	sealed, err := s.cfg.Box.Encrypt(string(raw))
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.UpdateConnectionSecret(ctx, 1, c.ID, sealed))
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	resp, err := doRequest(t, s, 1, testGate(t, api.URL), api.URL+"/user")
	require.NoError(t, err)
	assert.Contains(t, string(resp.Body), "octocat")
	assert.NotContains(t, string(resp.Body), "github-rotated-token-8251")
	assert.Equal(t, int32(1), refreshes.Load())
	rec, err = s.cfg.Store.GetConnection(ctx, 1, c.ID)
	require.NoError(t, err)
	_, credential, err = s.readCredential(rec)
	require.NoError(t, err)
	assert.Contains(t, string(credential.OAuthToken), "github-rotated-refresh-1094")
	assert.NotContains(t, rec.EncryptedSecret, "github-rotated")
	require.NoError(t, s.Revoke(ctx, 1, c.ID))
	assert.Equal(t, int32(1), revocations.Load())
	list, err := s.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestManagedOAuthRemainsInsideViewerGrantsGateAndIdlePruning(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var deleted atomic.Int32
	var authorized atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorized.Add(1)
		assert.Equal(t, "Bearer nango-access-secret-5783", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"result":"working","echo":"nango-access-secret-5783","refresh":"nango-refresh-secret-1648"}`)
	}))
	defer provider.Close()
	managed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer nango-operator-secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/connect/sessions":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "tenant:exhibit:1", body["end_user"].(map[string]any)["id"])
			fmt.Fprint(w, `{"data":{"token":"session-not-provider-secret","connect_link":"https://connect.nango.dev/?session_token=session-not-provider-secret","expires_at":"2026-10-22T08:00:00Z"}}`)
		case strings.HasPrefix(r.URL.Path, "/connections/"):
			assert.Equal(t, "slack", r.URL.Query().Get("provider_config_key"))
			if r.Method == http.MethodDelete {
				deleted.Add(1)
				w.WriteHeader(204)
				return
			}
			tag := "tenant:exhibit:1"
			id := strings.TrimPrefix(r.URL.Path, "/connections/")
			if id == "someone-elses" {
				tag = "tenant:exhibit:2"
			}
			fmt.Fprintf(w, `{"connection_id":%q,"provider_config_key":"slack","tags":{"exhibit_owner":%q},"end_user":{"id":%q},"credentials":{"type":"OAUTH2","access_token":"nango-access-secret-5783","refresh_token":"nango-refresh-secret-1648"}}`, id, tag, tag)
		default:
			t.Errorf("unexpected managed endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer managed.Close()
	s.cfg.VendorEnabled = true
	s.cfg.Vendor = VendorConfig{BaseURL: managed.URL, SecretKey: "nango-operator-secret", UserNamespace: "tenant", IdleDays: 30, Providers: map[string]VendorProvider{"slack": {Name: "Slack", Origins: []string{provider.URL}}}}
	session, err := s.BeginVendor(ctx, 1, "slack", UnlimitedLimits())
	require.NoError(t, err)
	assert.Contains(t, session.URL, "connect.nango.dev")
	_, err = s.Create(ctx, 1, CreateRequest{Name: "Someone else", Recipe: "nango", VendorProvider: "slack", VendorConnectionID: "someone-elses"}, UnlimitedLimits())
	assert.ErrorIs(t, err, ErrInvalid)
	c, err := s.Create(ctx, 1, CreateRequest{Name: "My Slack", Recipe: "nango", VendorProvider: "slack", VendorConnectionID: "mine"}, UnlimitedLimits())
	require.NoError(t, err)
	assert.Equal(t, "vendor", c.Kind)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	response, err := doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/conversations.list")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), "working")
	assert.NotContains(t, string(response.Body), "nango-access-secret-5783")
	assert.NotContains(t, string(response.Body), "nango-refresh-secret-1648")
	assert.Equal(t, int32(1), authorized.Load())
	_, err = doRequest(t, s, 1, testGate(t, provider.URL), "https://unapproved.example.test/data")
	var denied *egress.DeniedError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, "https://unapproved.example.test", denied.Origin)
	s.cfg.Now = func() time.Time { return time.Now().Add(45 * 24 * time.Hour) }
	n, err := s.PruneIdle(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, int32(1), deleted.Load())
	n, err = s.PruneIdle(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
	list, err := s.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestManagedCompletionPollsBoundFlowTagsAndIsIdempotent(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var flowID string
	var ready atomic.Bool
	var polls atomic.Int32
	managed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/connect/sessions":
			var body struct {
				Tags map[string]string `json:"tags"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			flowID = body.Tags["exhibit_flow"]
			require.NotEmpty(t, flowID)
			fmt.Fprint(w, `{"data":{"token":"session-token","connect_link":"https://connect.nango.dev/?session_token=session-token"}}`)
		case "/connections":
			polls.Add(1)
			assert.Equal(t, flowID, r.URL.Query().Get("tags[exhibit_flow]"))
			assert.Equal(t, "tenant:exhibit:1", r.URL.Query().Get("tags[end_user_id]"))
			if !ready.Load() {
				fmt.Fprint(w, `{"connections":[]}`)
				return
			}
			fmt.Fprintf(w, `{"connections":[{"connection_id":"new-managed","provider_config_key":"slack","tags":{"exhibit_flow":%q,"exhibit_owner":"tenant:exhibit:1","end_user_id":"tenant:exhibit:1"},"errors":[]}]}`, flowID)
		case "/connections/new-managed":
			fmt.Fprintf(w, `{"connection_id":"new-managed","provider_config_key":"slack","tags":{"exhibit_flow":%q,"exhibit_owner":"tenant:exhibit:1","end_user_id":"tenant:exhibit:1"},"end_user":{"id":"tenant:exhibit:1"},"credentials":{"access_token":"secret-never-returned"}}`, flowID)
		default:
			w.WriteHeader(404)
		}
	}))
	defer managed.Close()
	s.cfg.VendorEnabled = true
	s.cfg.Vendor = VendorConfig{BaseURL: managed.URL, SecretKey: "operator-secret", UserNamespace: "tenant", Providers: map[string]VendorProvider{"slack": {Name: "Slack", Origins: []string{"https://slack.com"}}}}
	session, err := s.BeginVendorBound(ctx, 1, "slack", UnlimitedLimits(), "login-session:browser-flow")
	require.NoError(t, err)
	assert.Equal(t, flowID, session.ID)
	_, _, err = s.CompleteVendor(ctx, 2, "login-session:browser-flow", session.ID, "Slack", UnlimitedLimits())
	assert.ErrorIs(t, err, ErrInvalid)
	_, _, err = s.CompleteVendor(ctx, 1, "other-browser", session.ID, "Slack", UnlimitedLimits())
	assert.ErrorIs(t, err, ErrInvalid)
	assert.Zero(t, polls.Load())
	c, done, err := s.CompleteVendor(ctx, 1, "login-session:browser-flow", session.ID, "Slack", UnlimitedLimits())
	require.NoError(t, err)
	assert.False(t, done)
	assert.Nil(t, c)
	ready.Store(true)
	c, done, err = s.CompleteVendor(ctx, 1, "login-session:browser-flow", session.ID, "My Slack", UnlimitedLimits())
	require.NoError(t, err)
	assert.True(t, done)
	require.NotNil(t, c)
	assert.Equal(t, "My Slack", c.Name)
	savedID := c.ID
	c, done, err = s.CompleteVendor(ctx, 1, "login-session:browser-flow", session.ID, "Duplicate?", UnlimitedLimits())
	require.NoError(t, err)
	assert.True(t, done)
	assert.Equal(t, savedID, c.ID)
	assert.Equal(t, int32(2), polls.Load())
	list, err := s.List(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	raw, _ := json.Marshal(c)
	assert.NotContains(t, string(raw), "secret-never-returned")
	assert.NotContains(t, string(raw), "new-managed")
}
