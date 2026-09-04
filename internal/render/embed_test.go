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

// The default is the whole compatibility guarantee of av-6nbo: an instance that
// has configured nothing emits the policy it emitted before third-party
// embedding existed, byte for byte. The variadic parameter is what makes that
// structural rather than a property to re-check, and this pins it.
func TestBuildCSPFrameAncestorsDefaultsToAppOriginAlone(t *testing.T) {
	const appOrigin = "https://app.example.com"

	if fa := frameAncestors(t, buildCSP(nil, appOrigin, "")); fa != appOrigin {
		t.Fatalf("with nothing configured, frame-ancestors must be exactly the app origin, got %q", fa)
	}
	// An empty configured set is the same thing arriving by the other route
	// (an operator who set EMBED_ORIGINS to an empty string), and must produce
	// the identical header rather than, say, a trailing space.
	withList := buildCSP([]string{"https://api.example.com"}, appOrigin, "", []string{}...)
	without := buildCSP([]string{"https://api.example.com"}, appOrigin, "")
	if withList != without {
		t.Fatalf("an empty embed list must change nothing:\n got %q\nwant %q", withList, without)
	}
}

// One configured origin: the app origin still comes first, because the app's
// own pages must never lose the ability to frame a document they own.
func TestBuildCSPFrameAncestorsWithOneEmbedOrigin(t *testing.T) {
	const appOrigin = "https://app.example.com"

	fa := frameAncestors(t, buildCSP(nil, appOrigin, "", "https://landing.example"))
	if fa != appOrigin+" https://landing.example" {
		t.Fatalf("frame-ancestors %q must name the app origin then the configured one", fa)
	}
}

// Several, in the order configured. Order carries no meaning to CSP — the
// directive is a set — but preserving it keeps the header legible beside the
// environment variable that produced it.
func TestBuildCSPFrameAncestorsWithSeveralEmbedOrigins(t *testing.T) {
	const appOrigin = "https://app.example.com"

	fa := frameAncestors(t, buildCSP(nil, appOrigin, "",
		"https://landing.example", "https://docs.example", "http://localhost:3000"))
	want := appOrigin + " https://landing.example https://docs.example http://localhost:3000"
	if fa != want {
		t.Fatalf("frame-ancestors\n got %q\nwant %q", fa, want)
	}
	// Framing is not egress: naming a site as a framer must not put it in any
	// directive governing what the artifact can reach.
	if cs := connectSrc(t, buildCSP(nil, appOrigin, "", "https://landing.example")); strings.Contains(cs, "landing.example") {
		t.Fatalf("an embed origin must not leak into connect-src: %q", cs)
	}
}

// The route-level half of the decision: a share is the one document a
// configured site may frame, and an origin nobody configured is not named in
// the header the browser enforces. (The refusal itself is the browser's; what
// this surface owes is a header that does not name that origin.)
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
	if strings.Contains(fa, "https://evil.example") {
		t.Fatalf("an origin nobody configured must not appear in %q", fa)
	}
}

// A share on an instance that configured nothing is framed by the app and
// nothing else — the behaviour every existing deployment keeps.
func TestServeShareWithoutEmbedOriginsIsUnchanged(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	if err := st.CreateShare(context.Background(), 1,
		&store.Share{ID: "sh1", ArtifactID: "abc", Public: true}); err != nil {
		t.Fatal(err)
	}

	if fa := frameAncestors(t, serveShare(t, rd, "sh1").Header().Get("Content-Security-Policy")); fa != "https://app.test" {
		t.Fatalf("an unconfigured instance must keep frame-ancestors at the app origin, got %q", fa)
	}
}

// The /a/:id decision, enforced rather than merely written down. A render
// document reached by token names a principal and inlines that principal's
// state; a share names nobody and is already public. So configuring an embed
// origin widens the share and must leave the token-gated routes exactly as
// tight as they were — including the widget route, whose authority is a strict
// subset of its artifact's.
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
