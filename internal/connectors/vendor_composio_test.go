package connectors

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These wire fixtures follow the current v3.1 reference, rather than the
// retired managed-OAuth initiate endpoint or masked credential schemas.
type composioFixtureState struct {
	AccountID, Owner, AuthConfig, Toolkit, Status   string
	Disabled, AuthDisabled                          bool
	LinkURL, LinkExpiry                             string
	AccountHTTP, ProxyHTTP, ProxyStatus, RevokeHTTP int
	ProxyHeaders                                    map[string]string
	ProxyData, BinaryData                           json.RawMessage
	ProxyWait                                       bool
}

type composioFixture struct {
	server                       *httptest.Server
	mu                           sync.Mutex
	state                        composioFixtureState
	requests                     []composioProxyRequest
	operations                   []string
	links, lookups, proxies      atomic.Int32
	revokes, deletes, extraCalls atomic.Int32
}

func newComposioFixture(t *testing.T, s *Service, origin string) *composioFixture {
	t.Helper()
	f := &composioFixture{state: composioFixtureState{
		Owner: "tenant:exhibit:1", AuthConfig: "ac_fixture", Toolkit: "notes", Status: "ACTIVE",
		LinkURL: "https://connect.composio.dev/link/ln_fixture", LinkExpiry: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		AccountHTTP: 200, ProxyHTTP: 200, ProxyStatus: 200, RevokeHTTP: 200,
		ProxyHeaders: map[string]string{"content-type": "application/json"}, ProxyData: json.RawMessage(`{"ok":true}`),
	}}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "composio-operator-private-key", r.Header.Get("x-api-key"))
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		state := f.state
		f.operations = append(f.operations, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/v3.1/connected_accounts/link":
			assert.Equal(t, http.MethodPost, r.Method)
			f.links.Add(1)
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, map[string]string{"auth_config_id": "ac_fixture", "user_id": "tenant:exhibit:1"}, body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"connected_account_id": "ca_fixture", "redirect_url": state.LinkURL, "expires_at": state.LinkExpiry, "link_token": "link-token-not-a-provider-token"})
		case r.URL.Path == "/api/v3.1/tools/execute/proxy":
			assert.Equal(t, http.MethodPost, r.Method)
			f.proxies.Add(1)
			var body composioProxyRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			f.mu.Lock()
			f.requests = append(f.requests, body)
			f.mu.Unlock()
			if state.ProxyWait {
				<-r.Context().Done()
				return
			}
			w.Header().Set("Retry-After", "11")
			w.WriteHeader(state.ProxyHTTP)
			if state.ProxyHTTP != 200 {
				fmt.Fprint(w, `{"error":"composio-operator-private-key must never escape errors"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": state.ProxyStatus, "headers": state.ProxyHeaders, "data": state.ProxyData, "binary_data": state.BinaryData})
		case strings.HasPrefix(r.URL.Path, "/api/v3.1/connected_accounts/"):
			switch {
			case strings.HasSuffix(r.URL.Path, "/revoke"):
				assert.Equal(t, http.MethodPost, r.Method)
				f.revokes.Add(1)
				w.WriteHeader(state.RevokeHTTP)
				fmt.Fprint(w, `{"connected_account":{"id":"ca_fixture","status":"REVOKED"},"revoked_tokens":["access_token"]}`)
			case r.Method == http.MethodDelete:
				f.deletes.Add(1)
				assert.Equal(t, "true", r.URL.Query().Get("revoke_on_delete"))
				fmt.Fprint(w, `{"success":true}`)
			default:
				assert.Equal(t, http.MethodGet, r.Method)
				f.lookups.Add(1)
				if state.AccountHTTP != http.StatusOK {
					w.WriteHeader(state.AccountHTTP)
					fmt.Fprint(w, `{"error":"account unavailable"}`)
					return
				}
				id := state.AccountID
				if id == "" {
					id = strings.TrimPrefix(r.URL.Path, "/api/v3.1/connected_accounts/")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": id, "user_id": state.Owner, "status": state.Status, "is_disabled": state.Disabled,
					"auth_config": map[string]any{"id": state.AuthConfig, "is_disabled": state.AuthDisabled},
					"toolkit":     map[string]string{"slug": state.Toolkit},
					"state":       map[string]any{"val": map[string]string{"access_token": "gho_...", "refresh_token": "REDACTED"}},
				})
			}
		default:
			f.extraCalls.Add(1)
			t.Errorf("unexpected Composio endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	s.cfg.VendorEnabled = true
	s.cfg.Vendor = VendorConfig{ComposioBaseURL: f.server.URL, ComposioAPIKey: "composio-operator-private-key", UserNamespace: "tenant", Providers: map[string]VendorProvider{
		"notes-cloud": {Name: "Cloud notes", Engine: "composio", AuthConfigID: "ac_fixture", Toolkit: "notes", Origins: []string{origin}},
	}}
	return f
}

func (f *composioFixture) change(update func(*composioFixtureState)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	update(&f.state)
}

func (f *composioFixture) lastRequest(t *testing.T) composioProxyRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(t, f.requests)
	return f.requests[len(f.requests)-1]
}

func completeComposio(t *testing.T, s *Service) *Connection {
	t.Helper()
	session, err := s.BeginVendorBound(context.Background(), 1, "notes-cloud", UnlimitedLimits(), "browser-session")
	require.NoError(t, err)
	connection, done, err := s.CompleteVendor(context.Background(), 1, "browser-session", session.ID, "Cloud notes", UnlimitedLimits())
	require.NoError(t, err)
	require.True(t, done)
	return connection
}

func assertComposioFailure(t *testing.T, err error, code string) {
	t.Helper()
	var failure *VendorError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "composio", failure.Engine)
	assert.Equal(t, code, failure.Code)
	assert.NotContains(t, err.Error(), "composio-operator-private-key")
}

func TestComposioHostedLinkCompletionBindsBrowserOwnerProviderAndAccount(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	f.change(func(state *composioFixtureState) { state.Status = "INITIATED" })
	ctx := context.Background()
	session, err := s.BeginVendorBound(ctx, 1, "notes-cloud", UnlimitedLimits(), "browser-session")
	require.NoError(t, err)
	assert.Equal(t, "https://connect.composio.dev/link/ln_fixture", session.URL)
	assert.Equal(t, "composio", session.Engine)
	assert.Equal(t, VendorForward, session.DeliveryMode)
	serialized, err := json.Marshal(session)
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), "ca_fixture")
	for _, other := range []struct {
		owner   int64
		binding string
	}{{2, "browser-session"}, {1, "different-browser"}} {
		_, done, err := s.CompleteVendor(ctx, other.owner, other.binding, session.ID, "Cloud notes", UnlimitedLimits())
		require.Error(t, err)
		assert.False(t, done)
	}
	assert.Zero(t, f.lookups.Load(), "a mismatched browser or owner never polls vendor accounts")
	_, done, err := s.CompleteVendor(ctx, 1, "browser-session", session.ID, "Cloud notes", UnlimitedLimits())
	require.NoError(t, err)
	assert.False(t, done)
	f.change(func(state *composioFixtureState) { state.Status = "ACTIVE" })
	connection, done, err := s.CompleteVendor(ctx, 1, "browser-session", session.ID, "Cloud notes", UnlimitedLimits())
	require.NoError(t, err)
	require.True(t, done)
	assert.Equal(t, "notes-cloud", connection.Provider)
	assert.Equal(t, "composio", connection.Engine)
	assert.Equal(t, VendorForward, connection.DeliveryMode)
	record, err := s.cfg.Store.GetConnection(ctx, 1, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, "composio", record.Engine)
	assert.Equal(t, "ca_fixture", record.ExternalRef)
	assert.NotContains(t, record.EncryptedSecret, "ca_fixture")
	serialized, err = json.Marshal(connection)
	require.NoError(t, err)
	for _, private := range []string{"ca_fixture", "gho_...", "REDACTED", "composio-operator-private-key"} {
		assert.NotContains(t, string(serialized), private)
	}
	_, done, err = s.CompleteVendor(ctx, 1, "browser-session", session.ID, "Idempotent", UnlimitedLimits())
	require.NoError(t, err)
	assert.True(t, done)
	assert.Equal(t, int32(2), f.lookups.Load(), "a saved flow is idempotent without another vendor call")
}

func TestComposioAccountBindingAndStatusCannotBeInferredFromMaskedCredentials(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*composioFixtureState)
		code   string
	}{{"other owner", func(v *composioFixtureState) { v.Owner = "tenant:exhibit:2" }, VendorRefused},
		{"other auth config", func(v *composioFixtureState) { v.AuthConfig = "ac_other" }, VendorRefused},
		{"other toolkit", func(v *composioFixtureState) { v.Toolkit = "calendar" }, VendorRefused},
		{"other account", func(v *composioFixtureState) { v.AccountID = "ca_other" }, VendorUpstreamError},
		{"expired", func(v *composioFixtureState) { v.Status = "EXPIRED" }, VendorNeedsReconnect},
		{"revoked", func(v *composioFixtureState) { v.Status = "REVOKED" }, VendorNeedsReconnect},
		{"failed", func(v *composioFixtureState) { v.Status = "FAILED" }, VendorNeedsReconnect},
		{"disabled", func(v *composioFixtureState) { v.Disabled = true }, VendorNeedsReconnect},
		{"disabled auth config", func(v *composioFixtureState) { v.AuthDisabled = true }, VendorNeedsReconnect}} {
		t.Run(scenario.name, func(t *testing.T) {
			s, _ := testService(t)
			f := newComposioFixture(t, s, "https://api.example.test")
			session, err := s.BeginVendorBound(context.Background(), 1, "notes-cloud", UnlimitedLimits(), "browser")
			require.NoError(t, err)
			f.change(scenario.change)
			_, done, err := s.CompleteVendor(context.Background(), 1, "browser", session.ID, "Cloud notes", UnlimitedLimits())
			assert.False(t, done)
			assertComposioFailure(t, err, scenario.code)
			connections, err := s.List(context.Background(), 1)
			require.NoError(t, err)
			assert.Empty(t, connections)
		})
	}
}

func TestComposioStatusAndHostedLinkExpiry(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	c := completeComposio(t, s)
	for _, scenario := range []struct{ remote, want string }{{"ACTIVE", "ok"}, {"EXPIRED", "needs_reconnect"}, {"INACTIVE", "needs_reconnect"}, {"REVOKED", "revoked"}, {"UNRECOGNIZED", "needs_reconnect"}} {
		f.change(func(v *composioFixtureState) { v.Status = scenario.remote })
		status, err := s.VendorStatus(context.Background(), 1, c.ID)
		require.NoError(t, err)
		assert.Equal(t, scenario.want, status)
	}
	f.change(func(v *composioFixtureState) { v.LinkExpiry = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339) })
	_, err := s.BeginVendorBound(context.Background(), 1, "notes-cloud", UnlimitedLimits(), "browser")
	assertComposioFailure(t, err, VendorNeedsReconnect)
	f.change(func(v *composioFixtureState) {
		v.LinkExpiry = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		v.LinkURL = "https://attacker.example.test/link/session"
	})
	_, err = s.BeginVendorBound(context.Background(), 1, "notes-cloud", UnlimitedLimits(), "browser")
	assertComposioFailure(t, err, VendorRefused)
	assert.Zero(t, f.extraCalls.Load())
}

func TestComposioForwardUsesExactRecipeAndSanitizedDocumentedWireFormat(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	p := s.cfg.Vendor.Providers["notes-cloud"]
	p.AllowWrites = true
	s.cfg.Vendor.Providers["notes-cloud"] = p
	c := completeComposio(t, s)
	_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	f.change(func(v *composioFixtureState) {
		v.ProxyHeaders = map[string]string{"Content-Type": "application/json", "Set-Cookie": "vendor-session=private", "Location": "https://attacker.example.test"}
	})
	req, err := http.NewRequest(http.MethodPost, "https://api.example.test/notes?order=recent", strings.NewReader(`{"title":"Hello"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer frame-forged-credential")
	req.Header.Set("X-Api-Key", "frame-forged-key")
	req.Header.Set("Cookie", "exhibit-login=secret")
	req.Header.Set("Origin", "https://app.example.test")
	response, err := s.Do(context.Background(), 1, "app", req, testGate(t, "https://api.example.test"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(response.Body))
	assert.Empty(t, response.Header.Get("Set-Cookie"))
	assert.Empty(t, response.Header.Get("Location"))
	forwarded := f.lastRequest(t)
	assert.Equal(t, "ca_fixture", forwarded.ConnectionID)
	assert.Equal(t, "tenant:exhibit:1", forwarded.UserID)
	assert.Equal(t, "https://api.example.test/notes?order=recent", forwarded.Endpoint)
	assert.Equal(t, http.MethodPost, forwarded.Method)
	assert.JSONEq(t, `{"title":"Hello"}`, string(forwarded.Body))
	assert.Nil(t, forwarded.BinaryBody)
	assert.ElementsMatch(t, []composioParameter{{Name: "Accept", Value: "application/json", In: "header"}, {Name: "Content-Type", Value: "application/json", In: "header"}}, forwarded.Parameters)
	before := f.proxies.Load()
	standalone := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-API-Key"))
		fmt.Fprint(w, "public API")
	}))
	defer standalone.Close()
	response, err = doRequest(t, s, 1, testGate(t, "https://api.example.test", standalone.URL), standalone.URL+"/notes")
	require.NoError(t, err)
	assert.Equal(t, "public API", string(response.Body))
	assert.Equal(t, before, f.proxies.Load(), "an independently approved API never reuses a vendor forwarder")
	_, err = doRequest(t, s, 1, testGate(t, "https://unapproved.example.test"), "https://api.example.test/notes")
	assert.ErrorIs(t, err, ErrEgress)
	assert.Equal(t, before, f.proxies.Load())
	assertComposioFailure(t, (composioEngine{s: s}).Authorize(context.Background(), req, VendorConnectionSpec{}, &credential{}), VendorRefused)
}

func TestComposioForwardBinaryBodyAndTextResult(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	p := s.cfg.Vendor.Providers["notes-cloud"]
	p.AllowWrites = true
	s.cfg.Vendor.Providers["notes-cloud"] = p
	c := completeComposio(t, s)
	_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	f.change(func(v *composioFixtureState) {
		v.ProxyData = json.RawMessage(`"plain text"`)
		v.ProxyHeaders = map[string]string{"Content-Type": "text/plain"}
	})
	data := []byte{0, 255, 17, 42}
	req, err := http.NewRequest(http.MethodPut, "https://api.example.test/file", bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/octet-stream")
	response, err := s.Do(context.Background(), 1, "app", req, testGate(t, "https://api.example.test"))
	require.NoError(t, err)
	assert.Equal(t, "plain text", string(response.Body))
	assert.Equal(t, &composioBinaryBody{Base64: base64.StdEncoding.EncodeToString(data), ContentType: "application/octet-stream"}, f.lastRequest(t).BinaryBody)
}

func TestComposioStringResultsPreserveJSONMediaTypes(t *testing.T) {
	for _, scenario := range []struct {
		contentType string
		json        bool
	}{{"application/json", true}, {"application/problem+json; charset=utf-8", true}, {"Application/Vnd.Exhibit+Json; charset=utf-8", true}, {"text/plain; charset=utf-8", false}} {
		t.Run(scenario.contentType, func(t *testing.T) {
			s, _ := testService(t)
			f := newComposioFixture(t, s, "https://api.example.test")
			c := completeComposio(t, s)
			_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
			require.NoError(t, err)
			f.change(func(v *composioFixtureState) {
				v.ProxyHeaders = map[string]string{"Content-Type": scenario.contentType}
				v.ProxyData = json.RawMessage(`"quoted \"result\""`)
			})
			response, err := doRequest(t, s, 1, testGate(t, "https://api.example.test"), "https://api.example.test/string")
			require.NoError(t, err)
			if scenario.json {
				var value string
				require.NoError(t, json.Unmarshal(response.Body, &value), "response.json() must receive a valid JSON string")
				assert.Equal(t, `quoted "result"`, value)
			} else {
				assert.Equal(t, `quoted "result"`, string(response.Body))
			}
		})
	}
}

func TestComposioOperatorKeyIsMaskedIndependentlyOfStandinsAndCacheKeyRotation(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	p := s.cfg.Vendor.Providers["notes-cloud"]
	p.CacheMinutes = 10
	s.cfg.Vendor.Providers["notes-cloud"] = p
	c := completeComposio(t, s)
	_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	handle, err := s.Standin(context.Background(), 1, "app", c.ID)
	require.NoError(t, err)
	f.change(func(v *composioFixtureState) {
		v.ProxyData = json.RawMessage(`{"operator":"composio-operator-private-key","safe":"cached"}`)
	})
	gate := testGate(t, "https://api.example.test")
	response, err := doRequest(t, s, 1, gate, "https://api.example.test/data")
	require.NoError(t, err)
	assert.NotContains(t, string(response.Body), "composio-operator-private-key")
	assert.NotContains(t, string(response.Body), handle.Value, "an operator key is never replaced by a viewer's provider stand-in")
	assert.Contains(t, string(response.Body), "credential redacted")
	s.cfg.Vendor.ComposioAPIKey = "rotated-composio-operator-private-key"
	response, err = doRequest(t, s, 1, gate, "https://api.example.test/data")
	require.NoError(t, err)
	assert.NotContains(t, string(response.Body), "composio-operator-private-key")
	assert.NotContains(t, string(response.Body), handle.Value)
	assert.Contains(t, string(response.Body), "cached")
	assert.Equal(t, int32(1), f.proxies.Load(), "cache carries the old redaction snapshot through key rotation")
}

func TestComposioErrorsCapsAndDownloadURLsDoNotExposeAnotherEgressPath(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		http, upstream int
		code           string
	}{{"vendor credential", 401, 200, VendorNeedsReconnect}, {"vendor rate limit", 429, 200, VendorRateLimited}, {"vendor refused", 403, 200, VendorRefused}, {"vendor failure", 503, 200, VendorUpstreamError},
		{"provider credential", 200, 401, VendorNeedsReconnect}, {"provider rate limit", 200, 429, VendorRateLimited}, {"provider refused", 200, 403, VendorRefused}, {"provider failure", 200, 503, VendorUpstreamError}} {
		t.Run(scenario.name, func(t *testing.T) {
			s, _ := testService(t)
			f := newComposioFixture(t, s, "https://api.example.test")
			c := completeComposio(t, s)
			_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
			require.NoError(t, err)
			f.change(func(v *composioFixtureState) {
				v.ProxyHTTP = scenario.http
				v.ProxyStatus = scenario.upstream
				v.ProxyHeaders = map[string]string{"Retry-After": "9"}
			})
			_, err = doRequest(t, s, 1, testGate(t, "https://api.example.test"), "https://api.example.test/data")
			assertComposioFailure(t, err, scenario.code)
		})
	}
	t.Run("download URL", func(t *testing.T) {
		s, _ := testService(t)
		f := newComposioFixture(t, s, "https://api.example.test")
		c := completeComposio(t, s)
		_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
		require.NoError(t, err)
		var downloads atomic.Int32
		download := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { downloads.Add(1) }))
		defer download.Close()
		f.change(func(v *composioFixtureState) {
			v.BinaryData = json.RawMessage(fmt.Sprintf(`{"url":%q,"size":12}`, download.URL+"/file"))
		})
		_, err = doRequest(t, s, 1, testGate(t, "https://api.example.test"), "https://api.example.test/file")
		assertComposioFailure(t, err, VendorRefused)
		assert.Zero(t, downloads.Load())
	})
	t.Run("response body cap", func(t *testing.T) {
		s, _ := testService(t)
		f := newComposioFixture(t, s, "https://api.example.test")
		c := completeComposio(t, s)
		_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
		require.NoError(t, err)
		data, err := json.Marshal(strings.Repeat("x", (4<<20)+1))
		require.NoError(t, err)
		f.change(func(v *composioFixtureState) { v.ProxyData = data })
		_, err = doRequest(t, s, 1, testGate(t, "https://api.example.test"), "https://api.example.test/large")
		assertComposioFailure(t, err, VendorUpstreamError)
	})
	t.Run("cancellation", func(t *testing.T) {
		s, _ := testService(t)
		f := newComposioFixture(t, s, "https://api.example.test")
		c := completeComposio(t, s)
		_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
		require.NoError(t, err)
		f.change(func(v *composioFixtureState) { v.ProxyWait = true })
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.test/slow", nil)
		require.NoError(t, err)
		_, err = s.Do(ctx, 1, "app", req, testGate(t, "https://api.example.test"))
		assertComposioFailure(t, err, VendorUpstreamError)
	})
}

func TestComposioRevocationUsesItsSavedEngineAndRetainsRecordsOnTransientFailure(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	c := completeComposio(t, s)
	p := s.cfg.Vendor.Providers["notes-cloud"]
	p.Engine = "nango"
	s.cfg.Vendor.Providers["notes-cloud"] = p
	f.change(func(v *composioFixtureState) { v.RevokeHTTP = 503 })
	err := s.Revoke(context.Background(), 1, c.ID)
	assertComposioFailure(t, err, VendorUpstreamError)
	_, err = s.cfg.Store.GetConnection(context.Background(), 1, c.ID)
	require.NoError(t, err)
	assert.Zero(t, f.deletes.Load())
	f.change(func(v *composioFixtureState) { v.RevokeHTTP = 200 })
	require.NoError(t, s.Revoke(context.Background(), 1, c.ID))
	assert.Equal(t, int32(2), f.revokes.Load())
	assert.Equal(t, int32(1), f.deletes.Load())
	_, err = s.cfg.Store.GetConnection(context.Background(), 1, c.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestComposioRevocationSupportsUnsupportedUpstreamRevokeAndAlreadyDeletedAccounts(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusConflict, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s, _ := testService(t)
			f := newComposioFixture(t, s, "https://api.example.test")
			c := completeComposio(t, s)
			f.change(func(v *composioFixtureState) { v.RevokeHTTP = status })
			require.NoError(t, s.Revoke(context.Background(), 1, c.ID))
			assert.Equal(t, int32(1), f.deletes.Load(), "delete the managed account when upstream revoke is unsupported or already complete")
		})
	}
	t.Run("deleted remotely", func(t *testing.T) {
		s, _ := testService(t)
		f := newComposioFixture(t, s, "https://api.example.test")
		c := completeComposio(t, s)
		f.change(func(v *composioFixtureState) { v.AccountHTTP = http.StatusNotFound })
		status, err := s.VendorStatus(context.Background(), 1, c.ID)
		require.NoError(t, err)
		assert.Equal(t, "revoked", status)
		require.NoError(t, s.Revoke(context.Background(), 1, c.ID))
		assert.Zero(t, f.revokes.Load())
		assert.Zero(t, f.deletes.Load())
		_, err = s.cfg.Store.GetConnection(context.Background(), 1, c.ID)
		assert.ErrorIs(t, err, store.ErrNotFound)
	})
}

func TestNativeNangoAndComposioShareCredentialQuotaAndCachePolicy(t *testing.T) {
	for _, kind := range []string{"native", "nango", "composio"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := testService(t)
			ctx := context.Background()
			var providerCalls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				providerCalls.Add(1)
				assert.Equal(t, "Bearer policy-provider-secret", r.Header.Get("Authorization"))
				fmt.Fprint(w, `{"ok":true}`)
			}))
			defer provider.Close()
			var standaloneCalls atomic.Int32
			standalone := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				standaloneCalls.Add(1)
				assert.Empty(t, r.Header.Get("Authorization"))
				assert.Empty(t, r.Header.Get("X-API-Key"))
				fmt.Fprint(w, `{"public":true}`)
			}))
			defer standalone.Close()
			var c *Connection
			var err error
			var actualCalls func() int32
			switch kind {
			case "native":
				c, err = s.Create(ctx, 1, CreateRequest{Name: "Policy API", Recipe: "bearer", Origin: provider.URL, Secret: "policy-provider-secret", DailyLimit: 1, CacheMinutes: 2}, UnlimitedLimits())
				actualCalls = providerCalls.Load
			case "nango":
				managed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "Bearer nango-operator-key", r.Header.Get("Authorization"))
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"connection_id":"policy-reference","provider_config_key":"policy-api","tags":{"exhibit_owner":"tenant:exhibit:1"},"end_user":{"id":"tenant:exhibit:1"},"credentials":{"type":"OAUTH2","access_token":"policy-provider-secret"}}`)
				}))
				defer managed.Close()
				s.cfg.VendorEnabled = true
				s.cfg.Vendor = VendorConfig{BaseURL: managed.URL, SecretKey: "nango-operator-key", UserNamespace: "tenant", Providers: map[string]VendorProvider{"policy-api": {Engine: "nango", Origins: []string{provider.URL}}}}
				c, err = s.saveVendor(ctx, 1, "Policy API", "policy-api", "tenant:exhibit:1", "policy-reference", VendorProvider{Name: "Policy API", Engine: "nango", Origins: []string{provider.URL}, DailyLimit: 1, CacheMinutes: 2}, UnlimitedLimits())
				actualCalls = providerCalls.Load
			case "composio":
				fixture := newComposioFixture(t, s, provider.URL)
				p := s.cfg.Vendor.Providers["notes-cloud"]
				p.DailyLimit, p.CacheMinutes = 1, 2
				s.cfg.Vendor.Providers["notes-cloud"] = p
				c = completeComposio(t, s)
				actualCalls = fixture.proxies.Load
			}
			require.NoError(t, err)
			_, err = s.Grant(ctx, 2, "app", GrantRequest{ConnectionID: c.ID})
			assert.ErrorIs(t, err, store.ErrNotFound, "a viewer cannot grant another person's connection")
			grant, err := s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
			require.NoError(t, err)
			assert.Empty(t, grant.PermittedEgress)
			assert.Contains(t, grant.Consent, "Credentials are sent only to: "+provider.URL)
			assert.Contains(t, grant.Consent, "independently approved origins")
			allowed, err := s.AllowedEgress(ctx, 1, "app", []string{provider.URL, standalone.URL})
			require.NoError(t, err)
			assert.Equal(t, []string{provider.URL, standalone.URL}, allowed)
			gate := testGate(t, provider.URL, standalone.URL)
			_, err = doRequest(t, s, 1, gate, standalone.URL+"/public")
			require.NoError(t, err)
			assert.Equal(t, int32(1), standaloneCalls.Load())
			assert.Zero(t, actualCalls())
			_, err = doRequest(t, s, 1, testGate(t, "https://unapproved.example.test"), provider.URL+"/data")
			assert.ErrorIs(t, err, ErrEgress)
			assert.Zero(t, actualCalls())
			for i := 0; i < 3; i++ {
				response, err := doRequest(t, s, 1, gate, provider.URL+"/data")
				require.NoError(t, err)
				assert.JSONEq(t, `{"ok":true}`, string(response.Body))
			}
			assert.Equal(t, int32(1), actualCalls(), "cache hits spend no additional provider request")
			usage, err := s.cfg.Store.VendorUsage(ctx, s.cfg.Now())
			require.NoError(t, err)
			if kind == "native" {
				assert.Empty(t, usage, "native requests incur no vendor accounting")
			} else {
				require.Len(t, usage, 1)
				assert.Equal(t, kind, usage[0].Engine)
				assert.EqualValues(t, 1, usage[0].StoredConnections)
				assert.Positive(t, usage[0].ManagementCalls)
				if kind == "nango" {
					assert.EqualValues(t, 1, usage[0].InjectCalls)
					assert.Zero(t, usage[0].ForwardCalls)
				} else {
					assert.EqualValues(t, 1, usage[0].ForwardCalls)
					assert.Zero(t, usage[0].InjectCalls)
				}
			}
			require.NoError(t, s.cfg.Store.PutArtifact(ctx, &store.Artifact{ID: "second-client", OwnerID: 1, Title: "Second", SourceBlobID: "body", Tier: store.Tier1}))
			_, err = s.Grant(ctx, 1, "second-client", GrantRequest{ConnectionID: c.ID})
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodGet, provider.URL+"/data", nil)
			require.NoError(t, err)
			_, err = s.Do(ctx, 1, "second-client", req, gate)
			require.NoError(t, err)
			assert.Equal(t, int32(1), actualCalls(), "another artifact reuses the connection's response")
			_, err = doRequest(t, s, 1, gate, provider.URL+"/data?next=page")
			assert.ErrorIs(t, err, store.ErrConnectionQuota)
			assert.Equal(t, int32(1), actualCalls(), "a spent quota is refused before native or vendor handoff")
			write, err := http.NewRequest(http.MethodPost, provider.URL+"/data", strings.NewReader(`{"write":true}`))
			require.NoError(t, err)
			_, err = s.Do(ctx, 1, "app", write, gate)
			assert.ErrorIs(t, err, ErrEgress)
			s.cfg.VendorEnabled = false
			_, err = doRequest(t, s, 1, gate, provider.URL+"/data")
			if kind == "native" {
				require.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrDisabled, "disabled vendor engines cannot release a cached response")
			}
			require.NoError(t, s.Ungrant(ctx, 1, "app", c.ID))
			_, err = doRequest(t, s, 1, gate, standalone.URL+"/public")
			require.NoError(t, err)
			assert.Equal(t, int32(2), standaloneCalls.Load(), "disconnection does not revoke independent origin approvals")
		})
	}
}

func TestNangoAndComposioWorkTogetherWithIndependentApprovedAPIs(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var nativeProviderCalls atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nativeProviderCalls.Add(1)
		assert.Equal(t, "Bearer simultaneous-nango-credential", r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"engine":"nango"}`)
	}))
	defer provider.Close()
	composio := newComposioFixture(t, s, "https://notes.example.test")
	composioConnection := completeComposio(t, s)
	nango := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer nango-operator-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"connection_id":"nango-reference","provider_config_key":"nango-notes","tags":{"exhibit_owner":"tenant:exhibit:1"},"end_user":{"id":"tenant:exhibit:1"},"credentials":{"type":"OAUTH2","access_token":"simultaneous-nango-credential"}}`)
	}))
	defer nango.Close()
	s.cfg.Vendor.BaseURL, s.cfg.Vendor.SecretKey = nango.URL, "nango-operator-key"
	s.cfg.Vendor.Providers["nango-notes"] = VendorProvider{Name: "Nango notes", Engine: "nango", Origins: []string{provider.URL}}
	nangoConnection, err := s.saveVendor(ctx, 1, "Nango notes", "nango-notes", "tenant:exhibit:1", "nango-reference", s.cfg.Vendor.Providers["nango-notes"], UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: composioConnection.ID})
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: nangoConnection.ID})
	require.NoError(t, err)
	var standaloneCalls atomic.Int32
	standalone := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		standaloneCalls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-API-Key"))
		fmt.Fprint(w, `{"standalone":true}`)
	}))
	defer standalone.Close()
	gate := testGate(t, provider.URL, "https://notes.example.test", standalone.URL)
	response, err := doRequest(t, s, 1, gate, provider.URL+"/notes")
	require.NoError(t, err)
	assert.JSONEq(t, `{"engine":"nango"}`, string(response.Body))
	response, err = doRequest(t, s, 1, gate, "https://notes.example.test/notes")
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(response.Body))
	response, err = doRequest(t, s, 1, gate, standalone.URL+"/public")
	require.NoError(t, err)
	assert.JSONEq(t, `{"standalone":true}`, string(response.Body))
	assert.Equal(t, int32(1), standaloneCalls.Load())
	assert.Equal(t, int32(1), nativeProviderCalls.Load())
	assert.Equal(t, int32(1), composio.proxies.Load())
	usage, err := s.cfg.Store.VendorUsage(ctx, s.cfg.Now())
	require.NoError(t, err)
	require.Len(t, usage, 2)
	assert.Equal(t, "composio", usage[0].Engine)
	assert.Equal(t, "nango", usage[1].Engine)
	assert.EqualValues(t, 1, usage[0].ForwardCalls)
	assert.EqualValues(t, 1, usage[1].InjectCalls)
}
