package api

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const originPolicyAPI = "https://api.example.test"
const originPolicyCDN = "https://unpkg.com"

func newNetworkOriginPolicyFixture(t *testing.T, approved bool) (*Router, string, *connectors.Connection) {
	t.Helper()
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	allowlist := []string{originPolicyAPI}
	if approved {
		allowlist = append(allowlist, originPolicyCDN)
	}
	id := createArtifact(t, ro, map[string]any{
		"title": "Protected app", "body": `<script>fetch('https://corsproxy.io/', {mode:'no-cors'})</script>`,
		"network_allowlist": allowlist,
	})
	connection, err := ro.cfg.Connections.Create(t.Context(), 1, connectors.CreateRequest{
		Name: "Private source", Recipe: "bearer", Origin: originPolicyAPI, Secret: "private-source-secret-9274",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(t.Context(), 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	return ro, id, connection
}

func TestNetworkOriginDecisionCannotOverrideConnectionCustody(t *testing.T) {
	for _, mode := range []string{"active", "retained", "disabled"} {
		for _, approved := range []bool{false, true} {
			name := mode + "/unapproved"
			if approved {
				name = mode + "/approved"
			}
			t.Run(name, func(t *testing.T) {
				ro, id, connection := newNetworkOriginPolicyFixture(t, approved)
				ctx := t.Context()
				if mode != "active" {
					require.NoError(t, ro.cfg.Store.SetState(ctx, 1, id, 1, "saved", "private response"))
					require.NoError(t, ro.cfg.Connections.Revoke(ctx, 1, connection.ID))
				}
				if mode == "disabled" {
					ro.cfg.Relay.Enabled = false
					ro.cfg.Connections = nil
				}
				before, err := ro.cfg.Store.ListOriginDecisions(ctx, 1, id)
				require.NoError(t, err)
				policiesBefore, err := ro.cfg.ConnectionPolicy.DataPolicies(ctx, 1, id)
				require.NoError(t, err)
				w := doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/origins", map[string]any{
					"origin": "https://UNPKG.com:443/", "decision": "allow", "source": "runtime",
				})
				require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
				var result map[string]string
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				assert.Equal(t, "connection_data_policy", result["code"])
				assert.Contains(t, result["error"], "Review the allowed data destinations in Connections")
				after, err := ro.cfg.Store.ListOriginDecisions(ctx, 1, id)
				require.NoError(t, err)
				assert.Equal(t, before, after, "a refused allow never changes saved approvals")
				policiesAfter, err := ro.cfg.ConnectionPolicy.DataPolicies(ctx, 1, id)
				require.NoError(t, err)
				assert.Equal(t, policiesBefore, policiesAfter, "origin approval never widens data custody")
				assert.NotContains(t, w.Body.String(), connection.ID)
				assert.NotContains(t, w.Body.String(), "private-source-secret-9274")

				w = doJSON(t, ro, http.MethodGet, "/partials/network-warnings?artifact="+id+"&origin="+url.QueryEscape("https://UNPKG.com:443/"), nil)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				assert.Contains(t, w.Body.String(), "data-network-origin-reviewed")
				assert.Contains(t, w.Body.String(), "data-connection-data-blocked")
				assert.NotContains(t, w.Body.String(), "data-network-origin-readonly")
				assert.Contains(t, w.Body.String(), "<code>"+originPolicyCDN+"</code>")
				assert.Contains(t, w.Body.String(), "external stylesheets, fonts and scripts")
				assert.Contains(t, w.Body.String(), `href="/connections?artifact=`+id+`"`)
				assert.Contains(t, w.Body.String(), `data-network-warning="no_cors"`, "source warnings remain visible")
				assert.Equal(t, approved, strings.Contains(w.Body.String(), "Your origin approval is saved."))
				assert.NotContains(t, w.Body.String(), connection.ID)
				assert.NotContains(t, w.Body.String(), "Private source")
				assert.NotContains(t, w.Body.String(), "private-source-secret-9274")
			})
		}
	}
}

func TestNetworkOriginDecisionAllowsOnlyCurrentPermittedDestinations(t *testing.T) {
	ro, id, connection := newNetworkOriginPolicyFixture(t, false)
	ctx := t.Context()
	// A refusal decision remains available even while custody excludes an
	// origin; it only controls future prompts and grants no network access.
	w := doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": originPolicyCDN, "decision": "block", "source": "runtime",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	_, err := ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{
		ConnectionID: connection.ID, PermittedEgress: []string{originPolicyCDN},
	})
	require.NoError(t, err)
	w = doJSON(t, ro, http.MethodGet, "/partials/network-warnings?artifact="+id+"&origin="+url.QueryEscape(originPolicyCDN), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "data-network-origin-reviewed")
	assert.NotContains(t, w.Body.String(), "data-connection-data-blocked")
	assert.NotContains(t, w.Body.String(), "Review connection rules")
	w = doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": originPolicyCDN, "decision": "allow", "source": "runtime",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	allowed, err := ro.cfg.Store.AllowedOrigins(ctx, 1, id)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{originPolicyAPI, originPolicyCDN}, allowed)

	// Permission changes can leave the destination in the artifact's allowlist.
	// Rechecking at decision time prevents a stale prompt from releasing data.
	_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	// Retained custody is narrowed by subsequent grants, including saved copies.
	w = doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": originPolicyCDN, "decision": "allow",
	})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestNetworkOriginWarningsPreserveViewerIsolationAndSharedReads(t *testing.T) {
	in := newTwoOwnerInstance(t)
	ro := in.ro
	ctx := t.Context()
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	connection, err := ro.cfg.Connections.Create(ctx, in.ownerOne, connectors.CreateRequest{
		Name: "Owner private API", Recipe: "bearer", Origin: originPolicyAPI, Secret: "owners-private-secret",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(ctx, in.ownerOne, in.artifactOne, connectors.GrantRequest{
		ConnectionID: connection.ID, ShareWithRecipients: true,
	})
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, in.ownerOne, in.artifactOne, originPolicyCDN, store.DecisionAllow, "test"))
	path := "/partials/network-warnings?artifact=" + in.artifactOne + "&origin=" + url.QueryEscape(originPolicyCDN)
	w := in.get(t, path, in.cookieTwo)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	missing := in.get(t, "/partials/network-warnings?artifact=absent&origin="+url.QueryEscape(originPolicyCDN), in.cookieTwo)
	assert.Equal(t, missing.Body.String(), w.Body.String(), "a foreign artifact is indistinguishable from an absent artifact")
	require.NoError(t, ro.cfg.Store.CreateShare(ctx, in.ownerOne, &store.Share{
		ID: "origin-policy-share", ArtifactID: in.artifactOne, RecipientID: &in.ownerTwo,
	}))
	w = in.get(t, path, in.cookieTwo)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "data-network-origin-readonly")
	assert.Contains(t, w.Body.String(), "data-connection-data-blocked")
	assert.Contains(t, w.Body.String(), "Your origin approval is saved.")
	assert.NotContains(t, w.Body.String(), "Owner private API")
	assert.NotContains(t, w.Body.String(), connection.ID)
	assert.NotContains(t, w.Body.String(), "owners-private-secret")

	// Shared reads do not extend the source-scanning edit surface or ownership.
	w = in.get(t, "/partials/network-warnings?artifact="+in.artifactOne, in.cookieTwo)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Copies retained by a recipient remain protected after the owner revokes
	// the connection and explicitly clears the owner's saved data.
	s := ro.cfg.Store.(*store.SQLiteStore)
	require.NoError(t, s.RetainSharedConnectionDataPolicies(ctx, in.ownerTwo, in.artifactOne))
	require.NoError(t, ro.cfg.Connections.Revoke(ctx, in.ownerOne, connection.ID))
	require.NoError(t, ro.cfg.Connections.ClearData(ctx, in.ownerOne, in.artifactOne))
	w = in.get(t, path, in.cookieTwo)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "data-connection-data-blocked")
}

func TestNetworkOriginWarningsValidateInputsAndEscapeSettingsLink(t *testing.T) {
	ro, id, connection := newNetworkOriginPolicyFixture(t, true)
	for _, candidate := range []string{"https://unpkg.com/file.css", `https://unpkg.com/<img src=x onerror=alert(1)>`, "javascript:alert(1)"} {
		w := doJSON(t, ro, http.MethodGet, "/partials/network-warnings?artifact="+id+"&origin="+url.QueryEscape(candidate), nil)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "<img")
		assert.NotContains(t, w.Body.String(), "data-network-origin-reviewed")
	}
	const unusualID = `review&next=<img src=x onerror=alert(1)>`
	ctx := t.Context()
	require.NoError(t, ro.cfg.Blob.Put(ctx, "unusual-network-source", strings.NewReader("<html>Example</html>")))
	require.NoError(t, ro.cfg.Store.PutArtifact(ctx, &store.Artifact{
		ID: unusualID, OwnerID: 1, Title: "Unusual identifier", SourceBlobID: "unusual-network-source", Tier: store.Tier1,
	}))
	// A settings link is only shown for an actual custody restriction.
	_, err := ro.cfg.Connections.Grant(ctx, 1, unusualID, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	w := doJSON(t, ro, http.MethodGet, "/partials/network-warnings?artifact="+url.QueryEscape(unusualID)+"&origin="+url.QueryEscape(originPolicyCDN), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, html.UnescapeString(w.Body.String()), `href="/connections?artifact=`+url.QueryEscape(unusualID)+`"`)
	assert.NotContains(t, w.Body.String(), "<img")
}

func TestNetworkOriginPolicyFailureNeverApprovesOrRendersAnAllowableOrigin(t *testing.T) {
	ro := newTestRouter(t)
	id := createOwnArtifact(t, ro)
	policyStore, err := store.OpenSQLite(filepath.Join(t.TempDir(), "unavailable-policy.db"))
	require.NoError(t, err)
	ro.cfg.Connections = connectors.New(connectors.Config{Store: policyStore, Box: ro.cfg.Secrets})
	require.NoError(t, policyStore.Close())
	w := doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": originPolicyCDN, "decision": "allow",
	})
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Connection policy is unavailable")
	assert.NotContains(t, w.Body.String(), "database")
	decisions, err := ro.cfg.Store.ListOriginDecisions(context.Background(), 1, id)
	require.NoError(t, err)
	assert.Empty(t, decisions)
	w = doJSON(t, ro, http.MethodGet, "/partials/network-warnings?artifact="+id+"&origin="+url.QueryEscape(originPolicyCDN), nil)
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "data-network-origin-reviewed")
	assert.NotContains(t, w.Body.String(), "database")
}
