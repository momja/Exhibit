package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectOpenPromptForEveryViewer(t *testing.T) {
	a := &store.Artifact{ID: "art-1", OwnerID: 1, Title: "A tool", Tier: store.Tier1}
	for _, readOnly := range []bool{false, true} {
		urls := testRenderURLs("https://render.example.com")
		urls.viewerID = 42
		creds := testPageCreds
		creds.ReadOnly = readOnly
		page, err := renderDetailPage(a, urls, creds, nil)
		require.NoError(t, err)
		assert.Contains(t, page, `data-artifact="art-1" data-viewer="42" data-open-url="/artifacts/art-1/open"`)
		assert.Contains(t, page, `role="dialog" aria-modal="true" aria-labelledby="direct-open-title" aria-describedby="direct-open-body"`)
		assert.Contains(t, page, `Open outside the preview sandbox?</h2>`)
		assert.Contains(t, page, `This tool will run as a regular web page. It can download files, navigate to external sites, and use browser features restricted in the preview, subject to your browser’s normal controls.</p>`)
		assert.Contains(t, page, `<input type="checkbox" id="direct-open-remember"> Do not ask again for this artifact`)
		assert.Contains(t, page, `id="direct-open-cancel">Cancel</button>`)
		assert.Contains(t, page, `id="direct-open-confirm">Open directly</button>`)
		for _, id := range []string{"direct-open-toolbar", "direct-open-banner"} {
			assert.Contains(t, page, `id="`+id+`"`)
		}
		assert.NotContains(t, page, `href="/artifacts/art-1/open"`, "no link can bypass confirmation through modified clicks")
		assert.Contains(t, page, `<script type="module" src="`+assetURL("gallery/direct-open.js")+`"></script>`)

		urls.viewerID = 43
		other, err := renderDetailPage(a, urls, creds, nil)
		require.NoError(t, err)
		assert.Contains(t, other, `data-viewer="43"`)
		urls.anonymous = true
		anonymous, err := renderDetailPage(a, urls, creds, nil)
		require.NoError(t, err)
		assert.Contains(t, anonymous, `data-viewer="anonymous"`)
	}
}

func TestFramedOpenRemainsFreshTokenRedirect(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Frame reload")
	req := httptest.NewRequest(http.MethodGet, "/artifacts/"+id+"/open?r=123", nil)
	req.Header.Set("Sec-Fetch-Dest", "iframe")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Header().Get("Location"), "http://render.test/a/"+id+"?")
	assert.NotContains(t, w.Body.String(), "direct-open-modal")
}

func TestUnconnectedDirectOpenStillMintsFreshAccess(t *testing.T) {
	ro := newTestRouter(t)
	id := createOwnArtifact(t, ro)
	w := doJSON(t, ro, http.MethodGet, "/artifacts/"+id+"/open?direct=1&r=123", nil)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Header().Get("Location"), "http://render.test/a/"+id+"?")
	assert.NotContains(t, w.Header().Get("Location"), "direct=")
}

func TestDirectOpenRejectsCurrentPrivatePoliciesFromStalePages(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()
	id := createOwnArtifact(t, ro)
	service := connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.Connections = service
	ro.cfg.ConnectionPolicy = service
	ro.cfg.Relay.Enabled = true
	stalePage := getPage(t, ro, "/artifacts/"+id)
	assert.NotContains(t, stalePage, `id="direct-open-toolbar" disabled`)
	c, err := service.Create(ctx, 1, connectors.CreateRequest{Name: "Private account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "private-key"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	assert.Contains(t, getPage(t, ro, "/artifacts/"+id), `id="direct-open-toolbar" disabled`)

	checkDirectAndFramedOpens := func() {
		t.Helper()
		// Missing metadata models HTTP t3code; document models HTTPS. A
		// stale app page supplies a valid app referrer in either case.
		for _, destination := range []string{"", "document"} {
			req := httptest.NewRequest(http.MethodGet, "/artifacts/"+id+"/open?direct=1", nil)
			req.Header.Set("Referer", ro.cfg.AppOrigin+"/artifacts/"+id)
			req.Header.Set("Sec-Fetch-Dest", destination)
			w := httptest.NewRecorder()
			ro.ServeHTTP(w, req)
			assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			assert.Empty(t, w.Header().Get("Location"))
		}
		for _, destination := range []string{"", "iframe"} {
			req := httptest.NewRequest(http.MethodGet, "/artifacts/"+id+"/open?r=123", nil)
			req.Header.Set("Sec-Fetch-Dest", destination)
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			w := httptest.NewRecorder()
			ro.ServeHTTP(w, req)
			assert.Equal(t, http.StatusFound, w.Code, w.Body.String())
			assert.Contains(t, w.Header().Get("Location"), "http://render.test/a/"+id+"/p/")
		}
	}
	checkDirectAndFramedOpens()
	// Saved-data restrictions survive removal of credential authority and
	// disabled acquisition/relay flags; stale pages still cannot bypass them.
	require.NoError(t, service.Ungrant(ctx, 1, id, c.ID))
	require.NoError(t, service.Revoke(ctx, 1, c.ID))
	ro.cfg.Connections = nil
	ro.cfg.Relay.Enabled = false
	checkDirectAndFramedOpens()
}
