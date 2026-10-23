package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNangoFlowAndConnectionKeepPinnedEngineWhenProviderConfigurationChanges(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var flowID string
	var injected, revoked, composioStarts atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		injected.Add(1)
		assert.Equal(t, "Bearer pinned-nango-access-1837", r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"source":"original provider","reflected":"pinned-nango-access-1837"}`)
	}))
	defer provider.Close()
	managed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/connect/sessions":
			assert.Equal(t, "Bearer nango-operator-key", r.Header.Get("Authorization"))
			var body struct {
				Tags map[string]string `json:"tags"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			flowID = body.Tags["exhibit_flow"]
			fmt.Fprint(w, `{"data":{"token":"session-token","connect_link":"https://connect.nango.dev/?session_token=session-token"}}`)
		case "/connections":
			assert.Equal(t, flowID, r.URL.Query().Get("tags[exhibit_flow]"))
			fmt.Fprintf(w, `{"connections":[{"connection_id":"pinned-nango","provider_config_key":"notes","tags":{"exhibit_flow":%q,"exhibit_owner":"tenant:exhibit:1","end_user_id":"tenant:exhibit:1"}}]}`, flowID)
		case "/connections/pinned-nango":
			assert.Equal(t, "notes", r.URL.Query().Get("provider_config_key"))
			if r.Method == http.MethodDelete {
				revoked.Add(1)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			fmt.Fprintf(w, `{"connection_id":"pinned-nango","provider_config_key":"notes","tags":{"exhibit_flow":%q,"exhibit_owner":"tenant:exhibit:1"},"end_user":{"id":"tenant:exhibit:1"},"credentials":{"type":"OAUTH2","access_token":"pinned-nango-access-1837","refresh_token":"pinned-nango-refresh-3492"}}`, flowID)
		case "/api/v3.1/connected_accounts/link":
			composioStarts.Add(1)
			assert.Equal(t, "composio-operator-key", r.Header.Get("x-api-key"))
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "ac_new-config", body["auth_config_id"])
			fmt.Fprintf(w, `{"connected_account_id":"ca_new-engine","redirect_url":"https://connect.composio.dev/link/new-engine","expires_at":%q}`, time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339))
		default:
			t.Errorf("unexpected vendor request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer managed.Close()
	s.cfg.VendorEnabled = true
	s.cfg.Vendor = VendorConfig{BaseURL: managed.URL, SecretKey: "nango-operator-key", ComposioBaseURL: managed.URL, ComposioAPIKey: "composio-operator-key", UserNamespace: "tenant", Providers: map[string]VendorProvider{"notes": {Name: "Notes", Origins: []string{provider.URL}, Engine: "nango"}}}
	session, err := s.BeginVendorBound(ctx, 1, "notes", UnlimitedLimits(), "pinned-browser")
	require.NoError(t, err)
	assert.Equal(t, "nango", session.Engine)
	assert.Equal(t, VendorInject, session.DeliveryMode)
	// Both the engine and reviewed origins are a snapshot of the started flow.
	s.cfg.Vendor.Providers["notes"] = VendorProvider{Name: "New Notes", Origins: []string{"https://new-provider.example.test"}, Engine: "composio", AuthConfigID: "ac_new-config"}
	connection, done, err := s.CompleteVendor(ctx, 1, "pinned-browser", session.ID, "Existing account", UnlimitedLimits())
	require.NoError(t, err)
	require.True(t, done)
	assert.Equal(t, "notes", connection.Provider)
	assert.Equal(t, "nango", connection.Engine)
	assert.Equal(t, VendorInject, connection.DeliveryMode)
	assert.True(t, connection.ReconnectRecommended)
	assert.Equal(t, []string{provider.URL}, connection.Origins)
	record, err := s.cfg.Store.GetConnection(ctx, 1, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, "nango", record.Engine)
	assert.Equal(t, "pinned-nango", record.ExternalRef)
	raw, err := json.Marshal(connection)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "pinned-nango")
	assert.NotContains(t, string(raw), "tenant:exhibit")
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	response, err := doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/data")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), "original provider")
	assert.NotContains(t, string(response.Body), "pinned-nango-access")
	assert.Equal(t, int32(1), injected.Load())
	_, err = doRequest(t, s, 1, testGate(t, "https://new-provider.example.test"), "https://new-provider.example.test/data")
	assert.ErrorIs(t, err, ErrEgress)
	fresh, err := s.BeginVendorBound(ctx, 1, "notes", UnlimitedLimits(), "new-browser")
	require.NoError(t, err)
	assert.Equal(t, "composio", fresh.Engine)
	assert.Equal(t, VendorForward, fresh.DeliveryMode)
	assert.Equal(t, int32(1), composioStarts.Load())
	require.NoError(t, s.Revoke(ctx, 1, connection.ID))
	assert.Equal(t, int32(1), revoked.Load(), "revocation follows the saved engine")
	counters, err := s.cfg.Store.VendorUsage(ctx, s.cfg.Now())
	require.NoError(t, err)
	byEngine := map[string]store.VendorUsage{}
	for _, counter := range counters {
		byEngine[counter.Engine] = counter
	}
	assert.Equal(t, int64(1), byEngine["nango"].InjectCalls)
	assert.Equal(t, int64(6), byEngine["nango"].ManagementCalls)
	assert.Zero(t, byEngine["nango"].StoredConnections)
	assert.Equal(t, int64(1), byEngine["composio"].ManagementCalls)
	assert.Zero(t, byEngine["composio"].ForwardCalls)
}

func TestVendorErrorsHaveStableCodesStatusAndNoUpstreamSecrets(t *testing.T) {
	for _, engine := range []string{"nango", "composio"} {
		for _, test := range []struct {
			status     int
			code       string
			httpStatus int
		}{
			{401, VendorNeedsReconnect, 409},
			{429, VendorRateLimited, 429},
			{403, VendorRefused, 403},
			{400, VendorRefused, 403},
			{503, VendorUpstreamError, 502},
		} {
			t.Run(fmt.Sprintf("%s/%d", engine, test.status), func(t *testing.T) {
				s, _ := testService(t)
				remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Retry-After", "90")
					w.WriteHeader(test.status)
					fmt.Fprint(w, `{"error":"sensitive-provider-message-and-token"}`)
				}))
				defer remote.Close()
				s.cfg.VendorEnabled = true
				s.cfg.Vendor = VendorConfig{BaseURL: remote.URL, SecretKey: "operator-secret-value", ComposioBaseURL: remote.URL, ComposioAPIKey: "operator-secret-value"}
				_, err := s.vendorAPIRequest(context.Background(), engine, "notes", http.MethodGet, "/test", nil, "management")
				require.Error(t, err)
				wrapped := fmt.Errorf("execute connection: %w", err)
				assert.Equal(t, test.code, ErrorCode(wrapped))
				assert.Equal(t, test.httpStatus, ErrorStatus(wrapped))
				assert.NotContains(t, err.Error(), "sensitive-provider")
				assert.NotContains(t, err.Error(), "operator-secret-value")
				assert.NotContains(t, err.Error(), remote.URL)
				var failure *VendorError
				require.ErrorAs(t, err, &failure)
				assert.Equal(t, "90", failure.RetryAfter)
			})
		}
	}
}

func TestVendorRefusesUnsafeRemoteReferencesBeforeManagementIO(t *testing.T) {
	s, _ := testService(t)
	s.cfg.VendorEnabled = true
	s.cfg.Vendor = VendorConfig{SecretKey: "operator-secret"}
	engine := nangoEngine{s: s}
	for _, ref := range []string{"", ".", "..", "../connections/other", "line\nbreak", strings.Repeat("a", 257)} {
		_, err := engine.connection(context.Background(), VendorConnectionSpec{Provider: "notes", OwnerTag: "tenant:exhibit:1", ExternalRef: ref})
		assert.Equal(t, VendorRefused, ErrorCode(err))
	}
}
