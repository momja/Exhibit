package api

// In-process API integration tests for out-of-line *markup* assets (av-oz40).
//
// The runtime pass covers payloads a page fetches from JavaScript; this covers
// what the markup walker sees — images, stylesheets, fonts. Both end in the
// same table and the same route, but they get there differently: the runtime
// pass leaves the document alone and is redirected at render, while these have
// their references rewritten at ingest, because nothing loads an <img src>
// through window.fetch and so there is nothing to intercept.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bigPNG is comfortably over InlineDataURIMaxBytes, so it is the case that
// leaves the document; tinyPNG is under it and must stay inline.
func bigPNG() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("x"), 32<<10)...)
}
func tinyPNG() []byte { return append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("y"), 64)...) }

const imagePage = `<!DOCTYPE html><html><head><title>Gallery</title></head>
<body>
  <img src="/img/big.png" alt="big">
  <img src="/img/tiny.png" alt="tiny">
</body></html>`

func imageFixture(t *testing.T) *httptest.Server {
	t.Helper()
	files := map[string]struct {
		contentType string
		body        []byte
	}{
		"/page.html":    {"text/html", []byte(imagePage)},
		"/img/big.png":  {"image/png", bigPNG()},
		"/img/tiny.png": {"image/png", tinyPNG()},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", f.contentType)
		_, _ = w.Write(f.body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The headline claim: a heavy image leaves the body, a light one does not, and
// the artifact still renders both.
func TestSnapshotIngestExternalizesLargeMarkupAssets(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := imageFixture(t)

	w, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := resp.Artifact.ID

	assets, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, id)
	require.NoError(t, err)
	require.Len(t, assets, 1, "only the over-threshold image leaves the document")
	assert.Equal(t, srv.URL+"/img/big.png", assets[0].SourceURL)

	body := storedBody(t, r, id)
	// The big one is referenced by URL...
	assert.Contains(t, body, "/a/"+id+"/assets/"+assets[0].ID)
	// ...and its bytes are not in the document. base64 of the payload would
	// appear here if it had been inlined.
	assert.NotContains(t, body, strings.Repeat("eHh4", 20))
	// The small one stays inline: one saved request beats a few hundred bytes.
	assert.Contains(t, body, "data:image/png;base64,")

	// And the size invariant this ticket exists for: the body is no longer a
	// function of its payloads' size.
	assert.Less(t, len(body), 8<<10,
		"a page with a 32 KiB image must not produce a body that carries it")
}

// The reference the walker wrote must actually resolve, with the right bytes
// and type, through the same route the runtime pass's payloads use.
func TestExternalizedMarkupAssetIsServedAndRenders(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := imageFixture(t)

	_, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	id := resp.Artifact.ID
	assets, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, id)
	require.NoError(t, err)
	require.Len(t, assets, 1)

	got := renderGet(t, r, "/a/"+id+"/assets/"+assets[0].ID)
	require.Equal(t, http.StatusOK, got.Code)
	assert.Equal(t, bigPNG(), got.Body.Bytes())
	assert.Equal(t, "image/png", got.Header().Get("Content-Type"))

	// img-src has to carry the asset path, not just connect-src: an <img> is
	// loaded by the element, not by fetch, so a policy that only covered
	// connect-src would leave the image silently blocked.
	doc := renderGet(t, r, "/a/"+id+"?"+rendertoken.Param+"="+r.tokens.Mint(id, defaultOwnerID))
	require.Equal(t, http.StatusOK, doc.Code)
	csp := doc.Header().Get("Content-Security-Policy")
	base := "/a/" + id + "/assets/"
	for _, directive := range []string{"img-src", "font-src", "style-src", "media-src", "connect-src", "script-src"} {
		section := cspDirective(t, csp, directive)
		assert.Contains(t, section, base, "%s must permit the artifact's own assets", directive)
	}
	// Still nothing for the user to approve.
	assert.Empty(t, resp.Artifact.NetworkAllowlist)
}

// One image shown three times is one stored asset, not three copies.
func TestRepeatedMarkupReferenceStoresOneAsset(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)

	payload := bigPNG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/page.html" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<!DOCTYPE html><html><body>
			  <img src="/a.png"><img src="/a.png"><img src="/a.png">
			</body></html>`)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	_, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	assets, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, resp.Artifact.ID)
	require.NoError(t, err)
	assert.Len(t, assets, 1, "the same source URL is one asset however often it appears")

	body := storedBody(t, r, resp.Artifact.ID)
	assert.Equal(t, 3, strings.Count(body, "/assets/"+assets[0].ID),
		"all three references point at the one stored copy")
}

// cspDirective returns one directive's source list from a policy string.
func cspDirective(t *testing.T, csp, name string) string {
	t.Helper()
	for _, part := range strings.Split(csp, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+" ") {
			return part
		}
	}
	t.Fatalf("policy has no %s directive: %s", name, csp)
	return ""
}

// A page whose only external reference was vendored comes back with an *empty*
// footprint — architecture.md §3.2's claim, which av-oz40 had quietly made
// false.
//
// The runtime pass leaves the body alone, so av-20fk never hit this. A markup
// asset over the inlining threshold is rewritten in place to its asset route,
// so the scanner then finds a URL on the render origin where a third-party CDN
// used to be, and reported it as an origin to approve.
func TestVendoredMarkupAssetLeavesAnEmptyFootprint(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)

	big := string(bytes.Repeat([]byte("P"), snapshot.InlineDataURIMaxBytes+1))
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/big.png") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, big)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>T</title></head><body><img src="big.png"></body></html>`)
	}))
	defer site.Close()

	w := doJSON(t, r, "POST", "/api/artifacts", map[string]any{
		"url": site.URL + "/index.html", "snapshot": true,
	})
	require.Equal(t, http.StatusCreated, w.Code)

	var resp struct {
		Artifact         struct{ ID string } `json:"artifact"`
		NetworkFootprint []string            `json:"network_footprint"`
		Snapshot         struct {
			ResidualOrigins []string `json:"residual_origins"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Empty(t, resp.NetworkFootprint,
		"the asset's bytes are served from this artifact's own path, which the render surface "+
			"grants as a system source — it is not a decision the user has to make")
	assert.Empty(t, resp.Snapshot.ResidualOrigins,
		"and the snapshot report says the same, since a fully vendored page contacts nobody")
}

// The reason the footprint must not offer it: an allow row for the render
// origin widens every CSP directive from this artifact's own asset path to the
// whole origin, which is reach over every *other* artifact's render document
// and assets. So the write path drops it however it arrives, not just the UI
// that no longer asks.
func TestTheRenderOriginNeverReachesTheAllowlist(t *testing.T) {
	r := newTestRouter(t)
	// https, because origin.NormalizeOrigin refuses plaintext http off
	// loopback (av-i7hd) and the create would 400 before this rule was
	// reached — passing for the wrong reason. The *footprint* side of the
	// same rule is exercised on the router's own plaintext origin above,
	// which is why the comparison canonicalizes rather than normalizes.
	r.cfg.RenderOrigin = "https://render.test"

	// On create...
	w := doJSON(t, r, "POST", "/api/artifacts", map[string]any{
		"title": "A", "body": "<html><body>a</body></html>",
		"network_allowlist": []string{r.cfg.RenderOrigin, "https://api.example.test"},
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Artifact struct {
			ID               string   `json:"id"`
			NetworkAllowlist []string `json:"network_allowlist"`
		} `json:"artifact"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, []string{"https://api.example.test"}, created.Artifact.NetworkAllowlist)

	// ...and on edit.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+created.Artifact.ID, map[string]any{
		"network_allowlist": []string{r.cfg.RenderOrigin},
	})
	require.Equal(t, http.StatusOK, w.Code)

	a, err := r.cfg.Store.GetArtifact(context.Background(), 1, created.Artifact.ID)
	require.NoError(t, err)
	assert.Empty(t, a.NetworkAllowlist, "no row a user could add, and none they could revoke")
}

// The comparison is on canonical spelling, not on string equality: an operator
// writes RENDER_ORIGIN by hand, and the scanner produces its own spelling of
// the same origin. A mismatch here would silently restore the behaviour above.
func TestRenderOriginIsMatchedOnSpellingNotBytes(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.RenderOrigin = "HTTPS://Render.Test.:443/"

	kept := r.withoutRenderOrigin([]string{
		"https://render.test",
		"https://render.test:443",
		"https://other.test",
		"https://render.test.evil.test", // a different origin that merely looks like it
	})
	assert.Equal(t, []string{"https://other.test", "https://render.test.evil.test"}, kept)
}
