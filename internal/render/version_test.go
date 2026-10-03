package render

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/store"
)

// Looking at an earlier version of an artifact without restoring it. What these
// pin is that it is the artifact as it was — its code and the data it left —
// and that nothing about it can reach back: not the live document, not the live
// data, and not a real origin to keep anything in.

// newVersionedRenderer builds an artifact with three versions and data that
// moved between them:
//
//	v1  OLD-BODY     left behind  {score: 10, theme: dark}
//	v2  MIDDLE-BODY  left behind  {score: 50, theme: dark}
//	v3  NEW-BODY     live         {score: 99, extra: x}   (the head)
func newVersionedRenderer(t *testing.T) (*Renderer, *store.SQLiteStore) {
	t.Helper()
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>OLD-BODY</body></html>")
	ctx := context.Background()
	set := func(k, v string) {
		t.Helper()
		if err := st.SetState(ctx, 1, "abc", 1, k, v); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(blobID, body string) {
		t.Helper()
		if err := rd.cfg.Blob.Put(ctx, blobID, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := st.CommitVersion(ctx, 1, "abc", store.VersionChange{BodyBlobID: &blobID}); err != nil {
			t.Fatal(err)
		}
	}
	set("score", "10")
	set("theme", "dark")
	commit("abc-body-2", "<html><head></head><body>MIDDLE-BODY</body></html>")
	set("score", "50")
	commit("abc-body-3", "<html><head></head><body>NEW-BODY</body></html>")
	set("score", "99")
	set("extra", "x")
	return rd, st
}

// versionRequest is the request the render surface receives for one version: the
// chi params its handler reads, and a token minted for that version's scope.
func versionRequest(artifactID string, seq int, token string) *http.Request {
	req := httptest.NewRequest("GET", "/a/"+artifactID+"/versions/"+strconv.Itoa(seq)+"?"+rendertoken.Param+"="+token, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("artifactID", artifactID)
	rctx.URLParams.Add("seq", strconv.Itoa(seq))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func serveVersion(rd *Renderer, artifactID string, seq int, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	rd.ServeVersion(w, versionRequest(artifactID, seq, token))
	return w
}

func versionToken(artifactID string, seq int, owner int64) string {
	return testTokens.Mint(rendertoken.VersionScope(artifactID, seq), owner)
}

// The version is the artifact as it was: its body, and the data it left behind —
// not the data the artifact holds now, which is what the live document inlines.
func TestAVersionIsServedAsItWasWithTheDataItLeft(t *testing.T) {
	rd, _ := newVersionedRenderer(t)

	w := serveVersion(rd, "abc", 1, versionToken("abc", 1, 1))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "OLD-BODY") || strings.Contains(body, "NEW-BODY") {
		t.Fatalf("a version serves its own body, not the head's: %s", body)
	}
	if !strings.Contains(body, `"score":"10"`) || !strings.Contains(body, `"theme":"dark"`) {
		t.Fatalf("a version inlines the data it left behind: %s", body)
	}
	if strings.Contains(body, `"score":"99"`) || strings.Contains(body, `"extra"`) {
		t.Fatalf("the live data must not leak into a version: %s", body)
	}

	// A middle version carries its own snapshot, not its neighbour's.
	w = serveVersion(rd, "abc", 2, versionToken("abc", 2, 1))
	if !strings.Contains(w.Body.String(), `"score":"50"`) || !strings.Contains(w.Body.String(), "MIDDLE-BODY") {
		t.Fatalf("version 2 must show version 2: %s", w.Body.String())
	}
}

// What makes "no data persisted" true of the document itself, apart from any
// host: the shim does not write through, the response gives the document an
// opaque origin however it is loaded, and it holds no device.
func TestAVersionDocumentCannotPersistAnything(t *testing.T) {
	rd, st := newVersionedRenderer(t)
	// An artifact whose owner approved the camera: a version of it is still not
	// handed one, because that approval is about the artifact as it is now.
	if err := st.UpdateArtifact(context.Background(), 1, "abc", map[string]any{"camera_approved": true}); err != nil {
		t.Fatal(err)
	}

	w := serveVersion(rd, "abc", 1, versionToken("abc", 1, 1))
	body := w.Body.String()
	if !strings.Contains(body, "var VERSION_VIEW = true;") || !strings.Contains(body, "if (VERSION_VIEW) return;") {
		t.Fatalf("the shim must not write through in a version view: %s", body)
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "sandbox allow-scripts allow-forms") || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("a version document must be sandboxed by its own CSP, with no real origin: %q", csp)
	}
	if pp := w.Header().Get("Permissions-Policy"); !strings.Contains(pp, "camera=()") || !strings.Contains(pp, "microphone=()") {
		t.Fatalf("a version view is denied both devices, approved or not: %q", pp)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("a version document must not be cached, got %q", cc)
	}
	if !strings.Contains(csp, "frame-ancestors https://app.test") {
		t.Fatalf("only the app origin may frame it: %q", csp)
	}

	// The live document of the same artifact is none of those things, so the
	// assertions above are about the version and not about every document.
	live := httptest.NewRecorder()
	rd.ServeArtifact(live, renderRequest("/a/abc", "abc", 1))
	if strings.Contains(live.Header().Get("Content-Security-Policy"), "sandbox") ||
		!strings.Contains(live.Body.String(), "var VERSION_VIEW = false;") {
		t.Fatalf("the live document must be unchanged by version views: %q", live.Header().Get("Content-Security-Policy"))
	}
}

// A snapshot is how the data *was*; a resync posts in how it is now. A version
// view must never apply one, or it would show the present and call it the past.
func TestAVersionPreambleRefusesToBeResyncedOverItsSnapshot(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test",
		map[string]string{"k": "v"}, originPolicy{}, false, false, true, nil)
	if !strings.Contains(doc, "window.parent !== window && !ANONYMOUS && !VERSION_VIEW") {
		t.Fatalf("the resync listener must not be installed in a version view: %s", doc)
	}
	// Everything else about the preamble is the artifact's own: a version runs.
	if !strings.Contains(doc, "sessionStorage") || !strings.Contains(doc, "window.fetch = function(input, init)") {
		t.Fatalf("a version view is the full preamble, narrowed only in what it writes: %s", doc)
	}
}

// Tokens are scoped to a document. History holds what the live document never
// shows, so a token the artifact can read out of its own URL must not open it,
// and a version's token must not open the live document.
func TestATokenOpensItsOwnDocumentAndNoOther(t *testing.T) {
	rd, _ := newVersionedRenderer(t)
	liveToken := testTokens.Mint("abc", 1)

	for name, tc := range map[string]struct {
		got  *httptest.ResponseRecorder
		want int
	}{
		"version 1 with its own token":                       {serveVersion(rd, "abc", 1, versionToken("abc", 1, 1)), 200},
		"version 1 with the live token":                      {serveVersion(rd, "abc", 1, liveToken), 404},
		"version 1 with version 2's token":                   {serveVersion(rd, "abc", 1, versionToken("abc", 2, 1)), 404},
		"version 1 with no token":                            {serveVersion(rd, "abc", 1, ""), 404},
		"version 1 with someone else's token":                {serveVersion(rd, "abc", 1, versionToken("abc", 1, 2)), 404},
		"version 1 of another artifact with this one's":      {serveVersion(rd, "other", 1, versionToken("abc", 1, 1)), 404},
		"a version that was never made":                      {serveVersion(rd, "abc", 42, versionToken("abc", 42, 1)), 404},
		"version 0":                                          {serveVersion(rd, "abc", 0, versionToken("abc", 0, 1)), 404},
		"the current version, which has no earlier state":    {serveVersion(rd, "abc", 3, versionToken("abc", 3, 1)), 404},
		"the live document with a version's token":           {serveLive(rd, "abc", versionToken("abc", 1, 1)), 404},
		"the live document with its own token (the control)": {serveLive(rd, "abc", liveToken), 200},
	} {
		if tc.got.Code != tc.want {
			t.Errorf("%s: status %d, want %d", name, tc.got.Code, tc.want)
		}
	}
}

func serveLive(rd *Renderer, id, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	rd.ServeArtifact(w, rawRequest("/a/"+id+"?"+rendertoken.Param+"="+token, id))
	return w
}

// Version tokens are only ever minted for the owner, and the render surface holds
// that line itself: the page that offers this is the owner's, and a grant does not
// carry history. A token naming anybody else's principal, or nobody's, is refused
// even though its signature is good.
func TestAVersionIsOnlyServedToItsOwner(t *testing.T) {
	rd, _ := newVersionedRenderer(t)
	scope := rendertoken.VersionScope("abc", 1)

	for name, tok := range map[string]string{
		"a recipient (a principal other than the owner)": testTokens.MintViewer(scope, 1, 2),
		"nobody (a public visitor)":                      testTokens.MintAnonymous(scope, 1),
	} {
		w := serveVersion(rd, "abc", 1, tok)
		if w.Code != 404 || strings.Contains(w.Body.String(), "OLD-BODY") {
			t.Errorf("%s: status %d, body %q", name, w.Code, w.Body.String())
		}
	}
	if w := serveVersion(rd, "abc", 1, testTokens.MintViewer(scope, 1, 1)); w.Code != 200 {
		t.Fatalf("the owner as their own viewer is the ordinary token, got %d", w.Code)
	}
}

// Another tenant's artifact has no version to look at, and answers the 404 a
// nonexistent one does — a valid token for their id minted under the wrong owner
// must not be an oracle.
func TestAnotherOwnersVersionIsNotFound(t *testing.T) {
	rd, st := newVersionedRenderer(t)
	putSecondOwnerArtifact(t, rd, st, "theirs", "<html><head></head><body>THEIRS-1</body></html>")
	ctx := context.Background()
	if err := rd.cfg.Blob.Put(ctx, "theirs-2", strings.NewReader("<html><head></head><body>THEIRS-2</body></html>")); err != nil {
		t.Fatal(err)
	}
	b := "theirs-2"
	if _, err := st.CommitVersion(ctx, 2, "theirs", store.VersionChange{BodyBlobID: &b}); err != nil {
		t.Fatal(err)
	}

	w := serveVersion(rd, "theirs", 1, versionToken("theirs", 1, 1))
	if w.Code != 404 || strings.Contains(w.Body.String(), "THEIRS") {
		t.Fatalf("owner 1 must not see owner 2's history: %d %s", w.Code, w.Body.String())
	}
	if w := serveVersion(rd, "theirs", 1, versionToken("theirs", 1, 2)); w.Code != 200 {
		t.Fatalf("the control: the real owner opens their own version, got %d", w.Code)
	}
}
