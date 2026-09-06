package render

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/store"
)

// av-v991 at the render surface: share_state_mode decides whose rows get
// inlined, and the answer is the artifact's rather than this handler's.
//
// The mode is written with SQL of the test's own because it is deliberately not
// caller-writable through the Store (av-lrae; av-6xjd owns the PATCH surface),
// and these tests are about what the value MEANS, not about who may set it. A
// second connection to the same file is safe — the store runs in WAL mode.
func newModedRenderer(t *testing.T, id, body, mode string) (*Renderer, *store.SQLiteStore) {
	t.Helper()
	rd, st, path := newTestRendererAt(t, id, body)
	if mode == store.ShareStateOwn {
		return rd, st
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(context.Background(),
		"UPDATE artifacts SET share_state_mode = ? WHERE id = ?", mode, id); err != nil {
		t.Fatal(err)
	}
	return rd, st
}

// renderFor serves /a/:id under a token naming a viewer who is not the owner —
// the shape av-awr4's recipient page mints (the owner authorizes, the viewer
// selects). No grant is needed for the token to verify: the render surface's
// authorization is the signature, and the app origin is where the grant was
// checked before minting.
func renderFor(t *testing.T, rd *Renderer, id string, viewerID int64) string {
	t.Helper()
	tok, err := testTokens.MintClaims(id, rendertoken.Claims{OwnerID: 1, ViewerID: viewerID})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	rd.ServeArtifact(w, rawRequest("/a/"+id+"?"+rendertoken.Param+"="+tok, id))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// 'shared' is one board: whoever is looking gets the owner's rows, which is
// what makes a two-player artifact a game rather than two solitaires.
func TestSharedModeInlinesTheOwnersRowsForEveryViewer(t *testing.T) {
	rd, st := newModedRenderer(t, "abc", "<html><head></head><body>hi</body></html>", store.ShareStateShared)
	ctx := context.Background()

	if err := st.SetState(ctx, 1, "abc", 1, "position", "the board"); err != nil {
		t.Fatal(err)
	}
	// Rows this viewer holds under their own id must be ignored in this mode,
	// or the two modes would silently blend into a third.
	if err := st.SetState(ctx, 1, "abc", store.ViewerID(otherViewer), "position", "SHOULD-NOT-APPEAR"); err != nil {
		t.Fatal(err)
	}

	doc := renderFor(t, rd, "abc", otherViewer)
	if !strings.Contains(doc, `"position":"the board"`) {
		t.Fatalf("a 'shared' artifact must inline the owner's rows for every viewer: %s", doc)
	}
	if strings.Contains(doc, "SHOULD-NOT-APPEAR") {
		t.Fatalf("'shared' must not fall back to the viewer's own rows: %s", doc)
	}
}

// The control for the test above, and the reason the mode has a default: with
// no opt-in, av-q0ub's per-viewer isolation is exactly what it was.
func TestOwnModeStillInlinesTheViewersOwnRows(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	if err := st.SetState(ctx, 1, "abc", 1, "note", "OWNER-ONLY"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(ctx, 1, "abc", store.ViewerID(otherViewer), "note", "the recipient's"); err != nil {
		t.Fatal(err)
	}

	doc := renderFor(t, rd, "abc", otherViewer)
	if !strings.Contains(doc, `"note":"the recipient's"`) {
		t.Fatalf("the default mode inlines the viewer's own rows: %s", doc)
	}
	if strings.Contains(doc, "OWNER-ONLY") {
		t.Fatalf("the owner's rows reached a document rendered for another principal: %s", doc)
	}
}

// A widget is a view of the same state, so it reads the same board. A tile on
// the owner's shelf showing the other player's move is the accidental-but-good
// consequence the epic wrote down deliberately.
func TestASharedArtifactsWidgetRendersTheSameBoard(t *testing.T) {
	rd, st := newModedRenderer(t, "abc", "<html><head></head><body>hi</body></html>", store.ShareStateShared)
	ctx := context.Background()

	if err := rd.cfg.Blob.Put(ctx, "abc-widget", strings.NewReader("<html><head></head><body>w</body></html>")); err != nil {
		t.Fatal(err)
	}
	if err := st.SetWidgetBlobID(ctx, 1, "abc", "abc-widget"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(ctx, 1, "abc", 1, "position", "the board"); err != nil {
		t.Fatal(err)
	}

	tok, err := testTokens.MintClaims("abc", rendertoken.Claims{OwnerID: 1, ViewerID: otherViewer})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	rd.ServeWidget(w, rawRequest("/w/abc?"+rendertoken.Param+"="+tok, "abc"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"position":"the board"`) {
		t.Fatalf("a widget must read the same rows its artifact does: %s", w.Body.String())
	}
}

// An anonymous viewer is unaffected by the mode, and this is the assertion that
// keeps it that way. 'shared' says whose rows a viewer is on; it does not say
// that a stranger with no principal becomes a viewer, and reading it that way
// would publish the owner's data to every visitor of a public instance.
func TestSharedModeInlinesNothingForAnAnonymousViewer(t *testing.T) {
	rd, st := newModedRenderer(t, "abc", "<html><head></head><body>hi</body></html>", store.ShareStateShared)
	ctx := context.Background()

	if err := st.SetState(ctx, 1, "abc", 1, "position", "MY-PRIVATE-BOARD"); err != nil {
		t.Fatal(err)
	}

	tok := testTokens.MintAnonymous("abc", 1)
	w := httptest.NewRecorder()
	rd.ServeArtifact(w, rawRequest("/a/abc?"+rendertoken.Param+"="+tok, "abc"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "MY-PRIVATE-BOARD") {
		t.Fatalf("a viewer with no principal is on no board: %s", w.Body.String())
	}
}

// The clear() fix (av-v991), asserted on the shipped bytes rather than trusted.
// `store = {}` rebinds the local and leaves the resync writing into an object
// nothing reads — latent while nothing re-read the cache, and load-bearing the
// moment something does.
func TestClearDeletesTheCachesKeysRatherThanRebindingIt(t *testing.T) {
	rd, _ := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	doc := serve(t, rd, "abc")

	if strings.Contains(doc, "store = {};") {
		t.Fatalf("clear() must not rebind the cache the resync writes into: %s", doc)
	}
	if !strings.Contains(doc, "Object.keys(store).forEach(function(k) { delete store[k]; });") {
		t.Fatalf("clear() must delete the keys in place: %s", doc)
	}
}

// The resync receiver is framed-only and principal-only, the same two
// conditions persistState carries — a top-level document has no host to hear
// from, and an anonymous one has no rows to keep in step.
func TestTheResyncReceiverIsShippedInTheRenderedDocument(t *testing.T) {
	rd, _ := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	doc := serve(t, rd, "abc")

	for _, want := range []string{
		"__avStateSync",
		"__avStateSynced",
		"function applyStateSync(",
		"new StorageEvent('storage'",
		"if (window.parent !== window && !ANONYMOUS) {",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("the render document must carry %q: %s", want, doc)
		}
	}
	// storageArea is typed Storage? in WebIDL, so handing it the shim's plain
	// object throws before the event exists ("Failed to convert value to
	// 'Storage'", measured in Chromium 149) — and a storage event that is never
	// dispatched is worth far less than a field nobody reads. The check is for
	// the property, not the word: the comment above it in the preamble explains
	// exactly this and must not fail its own assertion.
	if strings.Contains(doc, "storageArea:") {
		t.Fatalf("StorageEvent must be constructed without storageArea: %s", doc)
	}
}
