package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateRenderPolicyPathsCannotCarryCachedDataAfterReset(t *testing.T) {
	ro, _, id, _ := resetSafetyFixture(t)
	mintRequest := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(withPrincipal(t.Context(), Principal{OwnerID: 1, Kind: PrincipalServiceToken}))
	urls := ro.renderURLs(mintRequest)
	oldURLs := []string{urls.artifact(id), urls.widget(id), urls.version(id, 1)}
	for _, target := range oldURLs {
		assert.Contains(t, target, "/p/")
	}
	before, err := url.Parse(oldURLs[0])
	require.NoError(t, err)
	a := mustArtifact(t, ro, id)
	w := doJSON(t, ro, http.MethodPatch, "/api/artifacts/"+id, map[string]string{"body": "<h1>Updated connected app</h1>", "expected_source_revision": store.ContentRevision(a.SourceBlobID)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	after, err := url.Parse(ro.renderURLs(mintRequest).artifact(id))
	require.NoError(t, err)
	assert.Equal(t, before.Path, after.Path, "agent source edits must preserve the already-enforced data-policy path")
	authority, err := ro.relayPolicy(t.Context(), 1, mustArtifact(t, ro, id))
	require.NoError(t, err)
	assert.NotContains(t, oldURLs[0], authority, "the app's API authority must not reach the frame")
	w = doJSON(t, ro, http.MethodDelete, "/api/artifacts/"+id+"/connection-data", nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	for _, oldURL := range oldURLs {
		u, err := url.Parse(oldURL)
		require.NoError(t, err)
		q := u.Query()
		q.Set("balance", "PRIVATE-OLD-CACHE")
		u.RawQuery = q.Encode()
		// Fragments can carry cached data too, but browsers never send them
		// on the wire. The signed path must be rejected before either kind of
		// carried data could reach a newly widened document.
		u.Fragment = ""
		req := httptest.NewRequest(http.MethodGet, u.String(), nil)
		req.Header.Set("Sec-Fetch-Dest", "iframe")
		w = httptest.NewRecorder()
		ro.RenderHandler().ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code, oldURL+": "+w.Body.String())
		assert.NotContains(t, w.Body.String(), "PRIVATE-OLD-CACHE")
	}
	newURL := ro.renderURLs(mintRequest).artifact(id)
	assert.NotContains(t, newURL, "/p/", "only a new app-authorized render may use the released policy")
}

func TestArtifactFramesCannotMintFreshRenderCredentials(t *testing.T) {
	ro := newTestRouter(t)
	id := createOwnArtifact(t, ro)
	for _, suffix := range []string{"/open", "/widget/open"} {
		for _, site := range []string{"", "same-site", "cross-site", "none", "same-origin"} {
			req := httptest.NewRequest(http.MethodGet, "/artifacts/"+id+suffix+"?balance=PRIVATE#PRIVATE", nil)
			req.Header.Set("Sec-Fetch-Dest", "iframe")
			if site != "" {
				req.Header.Set("Sec-Fetch-Site", site)
			}
			w := httptest.NewRecorder()
			ro.ServeHTTP(w, req)
			if site == "same-origin" {
				assert.Equal(t, http.StatusFound, w.Code, suffix)
				assert.NotContains(t, w.Header().Get("Location"), "PRIVATE")
			} else {
				assert.Equal(t, http.StatusForbidden, w.Code, suffix+" "+site)
				assert.Empty(t, w.Header().Get("Location"))
			}
		}
	}
}

func mustArtifact(t *testing.T, ro *Router, id string) *store.Artifact {
	t.Helper()
	a, err := ro.cfg.Store.GetArtifact(t.Context(), 1, id)
	require.NoError(t, err)
	require.NotNil(t, a)
	return a
}
