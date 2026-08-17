package render

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/store"
)

// av-q0ub, at the render surface. Reads are inlined at render time (they must
// be: localStorage is synchronous and artifacts read it before any await), so
// "whose state is inlined" is decided here, once, by the principal the render
// token carries — and it is decided in the served bytes, which is what these
// tests read.

// otherViewer is a principal that is not the token's. Its rows are planted
// through the Store, since no route mints a non-owner viewer yet; planting them
// is how the render path gets asked the question before av-7k7b makes it
// routine.
const otherViewer int64 = 42

// AC#3: the inlined cache is the token's principal's state, not the artifact's.
// The distinction is invisible while one principal owns everything, so the test
// plants a second viewer's rows on the same artifact — under keys the artifact
// itself also uses — and requires them to be absent from the document.
func TestRenderInlinesOnlyTheTokenPrincipalsState(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	if err := st.SetState(ctx, 1, "abc", 1, "note", "mine"); err != nil {
		t.Fatal(err)
	}
	// Same key, different viewer — plus a key only they hold, so a leak shows
	// up whether the bug is "wrong row wins" or "every row is inlined".
	if err := st.SetState(ctx, 1, "abc", store.ViewerID(otherViewer), "note", "SHOULD-NOT-APPEAR"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(ctx, 1, "abc", store.ViewerID(otherViewer), "theirs", "ALSO-SHOULD-NOT-APPEAR"); err != nil {
		t.Fatal(err)
	}

	doc := serve(t, rd, "abc")

	if !strings.Contains(doc, `"note":"mine"`) {
		t.Fatalf("the principal's own state must be inlined: %s", doc)
	}
	for _, leaked := range []string{"SHOULD-NOT-APPEAR", "ALSO-SHOULD-NOT-APPEAR", `"theirs"`} {
		if strings.Contains(doc, leaked) {
			t.Fatalf("another viewer's state reached the document (%q): %s", leaked, doc)
		}
	}
}

// A principal with no rows of their own gets an empty cache, not the owner's.
// This is the same assertion from the other side, and it is the one that
// matters for a share opened by a stranger: absent state must render as absent,
// never as somebody else's.
func TestRenderInlinesNothingForAPrincipalWithNoState(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	// Only the *other* viewer has state here; the token's principal (1) has none.
	if err := st.SetState(ctx, 1, "abc", store.ViewerID(otherViewer), "note", "SHOULD-NOT-APPEAR"); err != nil {
		t.Fatal(err)
	}

	doc := serve(t, rd, "abc")

	if !strings.Contains(doc, "var cache = {};") {
		t.Fatalf("a principal with no rows must get an empty cache: %s", doc)
	}
	if strings.Contains(doc, "SHOULD-NOT-APPEAR") {
		t.Fatalf("state was inlined for the wrong principal: %s", doc)
	}
}

// A share carries no token and has no principal of its own, so it renders the
// artifact *as its owner sees it* (architecture.md §7). av-q0ub gives that
// sentence a mechanism — ServeShare passes a.OwnerID as the principal — and
// this pins it: the owner's rows appear, another viewer's do not.
func TestShareInlinesTheOwnersState(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	if err := st.SetState(ctx, 1, "abc", 1, "note", "the owner's"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(ctx, 1, "abc", store.ViewerID(otherViewer), "note", "SHOULD-NOT-APPEAR"); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateShare(ctx, 1, &store.Share{ID: "sh1", ArtifactID: "abc", Public: true}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/s/sh1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shareID", "sh1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	rd.ServeShare(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	doc := w.Body.String()
	if !strings.Contains(doc, `"note":"the owner's"`) {
		t.Fatalf("a share must inline the owner's state: %s", doc)
	}
	if strings.Contains(doc, "SHOULD-NOT-APPEAR") {
		t.Fatalf("a share must not inline some other viewer's state: %s", doc)
	}
}

// AC#5 at the render surface. A render is what a device *loads*, so the
// cross-device promise is: what one device wrote through the host bridge is in
// the next device's document. Two serves of the same artifact under the same
// principal stand in for two devices — there is nothing else distinguishing
// them, which is exactly the property the principal split must not have broken.
func TestASecondDeviceRenderInlinesTheFirstDevicesWrites(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	// iPhone: the host frame's write-through, which is all a device's write is.
	if err := st.SetState(ctx, 1, "abc", 1, "runs", `[{"km":5}]`); err != nil {
		t.Fatal(err)
	}

	// Mac: a fresh render, same principal.
	if doc := serve(t, rd, "abc"); !strings.Contains(doc, `"runs":"[{\"km\":5}]"`) {
		t.Fatalf("the second device's render must inline the first's write: %s", doc)
	}

	// Mac writes back; the iPhone's next load shows the update, not a stale
	// per-device copy.
	if err := st.SetState(ctx, 1, "abc", 1, "runs", `[{"km":5},{"km":8}]`); err != nil {
		t.Fatal(err)
	}
	doc := serve(t, rd, "abc")
	if !strings.Contains(doc, `{\"km\":8}`) {
		t.Fatalf("the first device's reload must inline the second's write: %s", doc)
	}
	if strings.Count(doc, `"runs"`) != 1 {
		t.Fatalf("one user's key must inline once, not once per device: %s", doc)
	}
}
