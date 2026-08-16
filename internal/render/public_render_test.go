package render

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
)

// av-wmp6, at the render surface. Public mode publishes a library to visitors
// with no credential; it must not publish what is *in* the tools. A run
// tracker's runs are the owner's data, and a state-driven widget would put them
// on the gallery grid without even a click.
//
// The mechanism is the render token's anonymous claim: a visitor's frames are
// minted for nobody, and a document rendered for nobody inlines nobody's state.
// These tests read the served bytes, because the served bytes are the leak.

// serveAnonymous renders the artifact for a viewer with no identity — the
// public-instance visitor's request, as their page's iframe would issue it.
func serveAnonymous(t *testing.T, rd *Renderer, id string) string {
	t.Helper()
	w := httptest.NewRecorder()
	rd.ServeArtifact(w, rawRequest(
		"/a/"+id+"?"+rendertoken.Param+"="+testTokens.MintAnonymous(id, 1), id))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// The one that matters: the same artifact, the same owner, two viewers. The
// owner sees their runs; a stranger reading the public library sees a tool that
// has never been used.
func TestAnonymousRenderInlinesNoState(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	if err := st.SetState(context.Background(), 1, "abc", 1, "runs", "MY-PRIVATE-RUNS"); err != nil {
		t.Fatal(err)
	}

	anon := serveAnonymous(t, rd, "abc")
	if strings.Contains(anon, "MY-PRIVATE-RUNS") {
		t.Fatalf("a public visitor's render inlined the owner's state: %s", anon)
	}
	if !strings.Contains(anon, "var cache = {};") {
		t.Fatalf("an anonymous render must boot with an empty cache: %s", anon)
	}
	// The artifact itself still renders — this subtracts state, not the tool.
	if !strings.Contains(anon, "<body>hi</body>") {
		t.Fatalf("an anonymous render must still serve the artifact: %s", anon)
	}

	// The other side of the same assertion, so a test that passes because
	// state inlining broke everywhere would fail here.
	if owner := serve(t, rd, "abc"); !strings.Contains(owner, "MY-PRIVATE-RUNS") {
		t.Fatalf("the owner's own render must still inline their state: %s", owner)
	}
}

// A widget tile is the worse half of the leak — it renders unattended on the
// grid, so state would be published to anyone who loaded the gallery page. It
// takes the same token and must reach the same answer.
func TestAnonymousWidgetRenderInlinesNoState(t *testing.T) {
	// newWidgetRenderer seeds one artifact, its tile, and a state key under
	// owner 1 — the run tracker whose runs must not reach the grid.
	rd := newWidgetRenderer(t, "abc",
		"<html><head></head><body>hi</body></html>",
		"<html><body>tile</body></html>")
	if err := rd.cfg.Store.SetState(context.Background(), 1, "abc", 1, "runs", "MY-PRIVATE-RUNS"); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	rd.ServeWidget(w, rawRequest(
		"/w/abc?"+rendertoken.Param+"="+testTokens.MintAnonymous("abc", 1), "abc"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "MY-PRIVATE-RUNS") {
		t.Fatalf("a public card's widget tile inlined the owner's state: %s", w.Body.String())
	}

	// The other side of the same assertion, so a test that passes because
	// state inlining broke everywhere (for widgets too) would fail here.
	owner := httptest.NewRecorder()
	rd.ServeWidget(owner, rawRequest(
		"/w/abc?"+rendertoken.Param+"="+testTokens.Mint("abc", 1), "abc"))
	if owner.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", owner.Code, owner.Body.String())
	}
	if !strings.Contains(owner.Body.String(), "MY-PRIVATE-RUNS") {
		t.Fatalf("the owner's own widget render must still inline their state: %s", owner.Body.String())
	}
}

// The write side of the same fact. Mutating routes stay authenticated in public
// mode, so a visitor's write-through would 401 into the host frame's swallowed
// .catch: the value would sit in the in-memory cache, the tool would look like
// it saved, and the write would be gone on reload. Not persisting is the honest
// version of what is already true.
func TestAnonymousPreambleDoesNotPersist(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, false, true)

	if !strings.Contains(doc, "var ANONYMOUS = true;") {
		t.Fatalf("an anonymous preamble must declare itself: %s", doc)
	}
	if !strings.Contains(doc, "if (ANONYMOUS) return;") {
		t.Fatalf("an anonymous preamble must short-circuit the state write-through: %s", doc)
	}
	// Storage still *works* in the frame — an artifact that reads and writes
	// localStorage runs normally, its writes simply do not outlive the render.
	if !strings.Contains(doc, "'localStorage'") {
		t.Fatalf("an anonymous preamble must still install storage: %s", doc)
	}

	// An ordinary render is unchanged: it declares itself identified and keeps
	// writing through.
	if owner := injectPreamble("<head></head>", "abc", "https://app.test", nil, false, false); !strings.Contains(owner, "var ANONYMOUS = false;") {
		t.Fatalf("an ordinary render must persist as before: %s", owner)
	}
}

// A share is deliberately NOT this: `/s/:id` still inlines the owner's state
// (TestShareInlinesTheOwnersState, state_principal_test.go), because it
// publishes one artifact by a decision its owner made about that artifact,
// where public mode flips a whole library with one env var. Same mechanism,
// different blast radius, different default.
