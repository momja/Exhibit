package render

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/store"
)

// frameAncestors returns the frame-ancestors directive value, failing if it is
// absent. Absence is not a neutral state for this directive: with no
// frame-ancestors at all, every origin on the web may frame the document.
func frameAncestors(t *testing.T, csp string) string {
	t.Helper()
	v, ok := directive(t, csp, "frame-ancestors")
	if !ok {
		t.Fatalf("no frame-ancestors directive in CSP — every origin could frame this document: %q", csp)
	}
	return v
}

// serveShare renders a share once and returns the response. The request it
// builds carries no credential at all, which is the point of the route: the
// share row is the authorization.
func serveShare(t *testing.T, rd *Renderer, shareID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/s/"+shareID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shareID", shareID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	rd.ServeShare(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return w
}

// buildCSP decides nothing about framing: it names the app origin when the
// caller names nobody, and otherwise emits exactly what it was handed. The
// default matters because every caller but the share route relies on it.
func TestBuildCSPFrameAncestorsDefaultsToAppOriginAlone(t *testing.T) {
	const appOrigin = "https://app.example.com"

	if fa := frameAncestors(t, buildCSP(nil, appOrigin, "")); fa != appOrigin {
		t.Fatalf("with no framers named, frame-ancestors must be exactly the app origin, got %q", fa)
	}
	// An empty slice is the same thing arriving by the other route, and must
	// produce the identical header rather than, say, a trailing space.
	withList := buildCSP([]string{"https://api.example.com"}, appOrigin, "", []string{}...)
	without := buildCSP([]string{"https://api.example.com"}, appOrigin, "")
	if withList != without {
		t.Fatalf("an empty framer list must change nothing:\n got %q\nwant %q", withList, without)
	}
}

// Given a list, buildCSP emits it verbatim and in order. Order carries no
// meaning to CSP — the directive is a set — but preserving it keeps the header
// legible beside the environment variable that produced it.
func TestBuildCSPFrameAncestorsNamesExactlyWhatItIsGiven(t *testing.T) {
	const appOrigin = "https://app.example.com"

	fa := frameAncestors(t, buildCSP(nil, appOrigin, "",
		appOrigin, "https://landing.example", "http://localhost:3000"))
	want := appOrigin + " https://landing.example http://localhost:3000"
	if fa != want {
		t.Fatalf("frame-ancestors\n got %q\nwant %q", fa, want)
	}
	// A caller may hand over the wildcard, and it must survive as the whole
	// directive rather than being combined with anything.
	if fa := frameAncestors(t, buildCSP(nil, appOrigin, "", "*")); fa != "*" {
		t.Fatalf("frame-ancestors must be exactly the wildcard the caller passed, got %q", fa)
	}
	// Framing is not egress: naming a site as a framer must not put it in any
	// directive governing what the artifact can reach.
	if cs := connectSrc(t, buildCSP(nil, appOrigin, "", "https://landing.example")); strings.Contains(cs, "landing.example") {
		t.Fatalf("an embed origin must not leak into connect-src: %q", cs)
	}
}

// The share route's policy in isolation (av-q3iy). Unset is open, because a
// share is a public link and refusing to be embedded contradicts that; set is
// a restriction, and the app origin survives it so an operator cannot lock
// their own gallery out by naming their landing page.
func TestShareFrameAncestors(t *testing.T) {
	const appOrigin = "https://app.example.com"

	if got := shareFrameAncestors(appOrigin, nil); len(got) != 1 || got[0] != "*" {
		t.Fatalf("an unconfigured instance must leave a share open to any framer, got %v", got)
	}
	if got := shareFrameAncestors(appOrigin, []string{}); len(got) != 1 || got[0] != "*" {
		t.Fatalf("an empty configured set is the same as none, got %v", got)
	}

	got := shareFrameAncestors(appOrigin, []string{"https://landing.example", "https://docs.example"})
	want := []string{appOrigin, "https://landing.example", "https://docs.example"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("configured framers\n got %v\nwant %v", got, want)
	}

	// "My gallery and nobody else" is written by naming the app origin, and
	// that is the documented way to close framing entirely (security.md §1.8).
	// It must produce the app origin once, not twice.
	if got := shareFrameAncestors(appOrigin, []string{appOrigin}); len(got) != 1 || got[0] != appOrigin {
		t.Fatalf("naming the app origin must close framing to it alone, got %v", got)
	}
}

// The behaviour change, pinned at the route (av-q3iy): an instance that has
// configured nothing serves a share any site may frame. This is the assertion
// most likely to be regressed by a later "tighten the default" instinct, so it
// is exact — `*` and nothing else — rather than a containment check that a
// policy naming the app origin as well would also pass.
func TestServeShareWithoutEmbedOriginsIsFramableByAnyone(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	if err := st.CreateShare(context.Background(), 1,
		&store.Share{ID: "sh1", ArtifactID: "abc", Public: true}); err != nil {
		t.Fatal(err)
	}

	csp := serveShare(t, rd, "sh1").Header().Get("Content-Security-Policy")
	if fa := frameAncestors(t, csp); fa != "*" {
		t.Fatalf("an unconfigured share must be framable by anyone, got frame-ancestors %q", fa)
	}
	// The wildcard is this directive's and no other: nothing about who may
	// frame a share says anything about what the artifact inside it may reach.
	if cs := connectSrc(t, csp); strings.Contains(cs, "*") {
		t.Fatalf("an open frame-ancestors must not widen connect-src: %q", cs)
	}
}

// The route-level half of the restriction: with EMBED_ORIGINS set, a share
// names the app origin and the configured sites, and the wildcard is gone —
// which is the entire point of setting it. (The refusal itself is the
// browser's; what this surface owes is a header that does not name the site.)
func TestServeShareHonorsConfiguredEmbedOrigins(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	rd.cfg.EmbedOrigins = []string{"https://landing.example", "https://docs.example"}
	if err := st.CreateShare(context.Background(), 1,
		&store.Share{ID: "sh1", ArtifactID: "abc", Public: true}); err != nil {
		t.Fatal(err)
	}

	fa := frameAncestors(t, serveShare(t, rd, "sh1").Header().Get("Content-Security-Policy"))
	want := "https://app.test https://landing.example https://docs.example"
	if fa != want {
		t.Fatalf("share frame-ancestors\n got %q\nwant %q", fa, want)
	}
	if strings.Contains(fa, "*") {
		t.Fatalf("a configured instance must not leave the wildcard in %q", fa)
	}
	if strings.Contains(fa, "https://evil.example") {
		t.Fatalf("an origin nobody configured must not appear in %q", fa)
	}
}

// The other half of the default, and the one a mistake would be quiet about:
// opening the *share* must not open the token-gated renders. With nothing
// configured — the state of every instance — /a/:id and /w/:id stay framed by
// the app origin alone, because they name a viewer and inline that viewer's
// state.
func TestTokenGatedRendersAreNotOpenedByTheDefault(t *testing.T) {
	rd := newWidgetRenderer(t, "abc",
		"<html><head></head><body>ARTIFACT-BODY</body></html>",
		"<html><head></head><body>WIDGET-BODY</body></html>")

	w := httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/abc", "abc", 1))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fa := frameAncestors(t, w.Header().Get("Content-Security-Policy")); fa != "https://app.test" {
		t.Fatalf("/a/:id must be framed by the app alone with nothing configured, got %q", fa)
	}

	if fa := frameAncestors(t, serveWidget(t, rd, "abc").Header().Get("Content-Security-Policy")); fa != "https://app.test" {
		t.Fatalf("/w/:id must be framed by the app alone with nothing configured, got %q", fa)
	}
}

// The /a/:id decision, enforced rather than merely written down. A render
// document reached by token names a principal and inlines that principal's
// state; a share names nobody and is already public. So EMBED_ORIGINS decides
// the share alone and must leave the token-gated routes exactly as tight as
// they were — including the widget route, whose authority is a strict subset
// of its artifact's.
func TestEmbedOriginsDoNotWidenTokenGatedRenders(t *testing.T) {
	rd := newWidgetRenderer(t, "abc",
		"<html><head></head><body>ARTIFACT-BODY</body></html>",
		"<html><head></head><body>WIDGET-BODY</body></html>")
	rd.cfg.EmbedOrigins = []string{"https://landing.example"}

	w := httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/abc", "abc", 1))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fa := frameAncestors(t, w.Header().Get("Content-Security-Policy")); fa != "https://app.test" {
		t.Fatalf("/a/:id must stay framed by the app alone — it inlines the viewer's state — got %q", fa)
	}

	if fa := frameAncestors(t, serveWidget(t, rd, "abc").Header().Get("Content-Security-Policy")); fa != "https://app.test" {
		t.Fatalf("/w/:id must stay framed by the app alone, got %q", fa)
	}
}
