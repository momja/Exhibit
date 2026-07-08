package api

// In-process API integration tests for the snapshot ingest flow
// (exhibit-lwb.6): they drive POST /api/artifacts through the real chi router,
// store, and blob backend, with an httptest server standing in for the source
// site. This is the httptest-level "e2e" the story asks for — deliberately NOT
// the Playwright browser-automation suite tracked by the separate epic av-f3cp,
// which this story does not depend on.
//
// Coverage: a URL ingest with snapshot on must produce a self-contained
// artifact plus a report, partial per-asset failures must never abort the
// ingest, and the <base href> fallback must cover the snapshot-off and
// snapshot-failed paths.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/artifact-viewer/artifact-viewer/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withUnguardedFetcher swaps the ingest snapshot fetcher for one without the
// SSRF dial guard, which would otherwise refuse the loopback addresses the
// httptest fixture servers listen on.
func withUnguardedFetcher(t *testing.T) {
	t.Helper()
	orig := newSnapshotFetcher
	newSnapshotFetcher = func(pageURL string) (*snapshot.Fetcher, error) {
		return snapshot.NewFetcherForTests(pageURL, snapshot.DefaultLimits())
	}
	t.Cleanup(func() { newSnapshotFetcher = orig })
}

// snapshotFixturePage references every asset class the snapshot must vendor —
// a relative stylesheet (whose nested @import chain carries a font and an
// image), a script, an image — plus one deliberately missing image so partial
// failure is exercised.
const snapshotFixturePage = `<!DOCTYPE html><html><head>
<title>Snapshot Fixture</title>
<link rel="stylesheet" href="css/main.css">
<script src="js/app.js"></script>
</head><body>
<img src="img/logo.png">
<img src="img/missing.png">
</body></html>`

// snapshotFixtureFiles are the fetchable assets behind snapshotFixture, keyed
// by path. img/missing.png is deliberately absent (404s).
var snapshotFixtureFiles = map[string]struct {
	contentType string
	body        string
}{
	"/page.html":             {"text/html", snapshotFixturePage},
	"/css/main.css":          {"text/css", "@import \"sub/nested.css\";\nbody{background:url(../img/bg.png)}"},
	"/css/sub/nested.css":    {"text/css", "@font-face{font-family:Fx;src:url(fonts/f.woff2)}"},
	"/css/sub/fonts/f.woff2": {"font/woff2", "WOFF2-bytes"},
	"/js/app.js":             {"application/javascript", `console.log("app")`},
	"/img/logo.png":          {"image/png", "PNG-logo-bytes"},
	"/img/bg.png":            {"image/png", "PNG-bg-bytes"},
}

func snapshotFixture(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := snapshotFixtureFiles[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", f.contentType)
		io.WriteString(w, f.body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// postArtifact POSTs a create request and decodes the typed response.
func postArtifact(t *testing.T, r *Router, payload map[string]any) (*httptest.ResponseRecorder, createArtifactResponse) {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp createArtifactResponse
	if w.Code == http.StatusCreated {
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	}
	return w, resp
}

func fixtureB64(path string) string {
	return base64.StdEncoding.EncodeToString([]byte(snapshotFixtureFiles[path].body))
}

func TestSnapshotIngestEndToEnd(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := snapshotFixture(t)
	pageURL := srv.URL + "/page.html"

	w, resp := postArtifact(t, r, map[string]any{
		"url": pageURL, "snapshot": true, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "Snapshot Fixture", resp.Artifact.Title)

	rep := resp.Snapshot
	require.NotNil(t, rep)
	assert.True(t, rep.Applied)
	assert.Empty(t, rep.Error)

	// Every fetchable asset was vendored, including the ones only reachable
	// through the nested @import chain (main.css → nested.css → font).
	vendoredPaths := []string{
		"/css/main.css", "/css/sub/fonts/f.woff2", "/css/sub/nested.css",
		"/img/bg.png", "/img/logo.png", "/js/app.js",
	}
	wantURLs := make([]string, len(vendoredPaths))
	var wantBytes int64
	for i, p := range vendoredPaths {
		wantURLs[i] = srv.URL + p
		wantBytes += int64(len(snapshotFixtureFiles[p].body))
	}
	assert.Equal(t, wantURLs, rep.VendoredURLs)
	assert.Equal(t, wantBytes, rep.VendoredBytes)

	// The one missing asset is a recorded failure, not an ingest error.
	require.Len(t, rep.Failures, 1)
	fail := rep.Failures[0]
	assert.Equal(t, "img/missing.png", fail.Ref)
	assert.Equal(t, srv.URL+"/img/missing.png", fail.URL)
	assert.Equal(t, string(snapshot.ErrHTTPStatus), fail.Kind)

	// Its surviving reference resolves to the source origin, which surfaces
	// as a residual origin in both the report and the footprint — but is
	// never written to the allowlist (approval stays explicit, spec §6.2).
	assert.Equal(t, []string{srv.URL}, rep.ResidualOrigins)
	assert.Equal(t, []string{srv.URL}, resp.NetworkFootprint)
	assert.Empty(t, resp.Artifact.NetworkAllowlist)

	body := getArtifactBody(t, r, resp.Artifact.ID)
	// Vendored: image → data: URI, script → inline JS, stylesheet link →
	// inline <style> with the import chain folded in (font + image as data:).
	assert.Contains(t, body, "data:image/png;base64,"+fixtureB64("/img/logo.png"))
	assert.Contains(t, body, `console.log("app")`)
	assert.NotContains(t, body, "css/main.css")
	assert.NotContains(t, body, "js/app.js")
	assert.NotContains(t, body, "@import")
	assert.Contains(t, body, "data:font/woff2;base64,"+fixtureB64("/css/sub/fonts/f.woff2"))
	assert.Contains(t, body, "data:image/png;base64,"+fixtureB64("/img/bg.png"))
	// The failed asset's reference survives verbatim, and the injected
	// <base href> keeps it resolving against the source site.
	assert.Contains(t, body, `src="img/missing.png"`)
	assert.Contains(t, body, `<base href="`+pageURL+`">`)
}

func TestURLIngestWithoutSnapshotInjectsBase(t *testing.T) {
	r := newTestRouter(t)
	srv := snapshotFixture(t)
	pageURL := srv.URL + "/page.html"

	w, resp := postArtifact(t, r, map[string]any{
		"url": pageURL, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Nil(t, resp.Snapshot)

	// Option A: the body keeps its relative references and gains only the
	// <base href> so they resolve against the source site at render time.
	body := getArtifactBody(t, r, resp.Artifact.ID)
	assert.Contains(t, body, `<base href="`+pageURL+`">`)
	assert.Contains(t, body, `href="css/main.css"`)
	assert.Contains(t, body, `src="js/app.js"`)
	assert.Contains(t, body, `src="img/logo.png"`)

	// The base-aware scan reports where those references really point.
	assert.Equal(t, []string{srv.URL}, resp.NetworkFootprint)
	assert.Empty(t, resp.Artifact.NetworkAllowlist)
}

func TestSnapshotFailureFallsBackToBase(t *testing.T) {
	orig := newSnapshotFetcher
	newSnapshotFetcher = func(string) (*snapshot.Fetcher, error) {
		return nil, errors.New("fetcher construction failed")
	}
	t.Cleanup(func() { newSnapshotFetcher = orig })

	r := newTestRouter(t)
	srv := snapshotFixture(t)
	pageURL := srv.URL + "/page.html"

	w, resp := postArtifact(t, r, map[string]any{
		"url": pageURL, "snapshot": true, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, "a failed snapshot must not abort the ingest")

	rep := resp.Snapshot
	require.NotNil(t, rep)
	assert.False(t, rep.Applied)
	assert.Contains(t, rep.Error, "fetcher construction failed")
	assert.Empty(t, rep.VendoredURLs)

	// The original body is stored with the <base href> fallback.
	body := getArtifactBody(t, r, resp.Artifact.ID)
	assert.Contains(t, body, `<base href="`+pageURL+`">`)
	assert.Contains(t, body, `href="css/main.css"`)
}

func TestSnapshotRequiresURL(t *testing.T) {
	r := newTestRouter(t)

	w, _ := postArtifact(t, r, map[string]any{
		"title": "Pasted", "body": "<html></html>", "snapshot": true,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "snapshot requires a source url")
}

func TestPasteIngestGetsNoBaseTag(t *testing.T) {
	r := newTestRouter(t)

	const src = `<html><head><title>P</title></head><body>pasted</body></html>`
	id := createArtifact(t, r, map[string]any{
		"title": "P", "body": src, "network_allowlist": []string{},
	})
	assert.Equal(t, src, getArtifactBody(t, r, id))
}
