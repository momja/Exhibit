package render

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/store"
)

// newTestRenderer builds a Renderer backed by a temp SQLite store + FS blob,
// with one artifact whose body is the given HTML. The store is returned too,
// so tests can mutate state the way the host bridge would (SetState/DeleteState/
// ClearState) and then re-serve to observe what the next render inlines —
// standing in for "reload" since there is no browser in these tests.
func newTestRenderer(t *testing.T, id, body string) (*Renderer, *store.SQLiteStore) {
	t.Helper()

	dbf, err := os.CreateTemp(t.TempDir(), "render-*.db")
	if err != nil {
		t.Fatal(err)
	}
	dbf.Close()
	st, err := store.OpenSQLite(dbf.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	bl, err := blob.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobID := id + "-blob"
	if err := bl.Put(context.Background(), blobID, strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(context.Background(), &store.Artifact{
		ID: id, OwnerID: 1, Title: "t", SourceBlobID: blobID, Tier: 1,
	}); err != nil {
		t.Fatal(err)
	}

	rd := New(Config{
		Store: st, Blob: bl,
		AppOrigin: "https://app.test", RenderOrigin: "https://render.test",
		Tokens: testTokens,
	})
	return rd, st
}

// serve renders the artifact once and returns the response body — the
// render-time inlined state a fresh page load (or reload) would see. It carries
// the same (artifact, owner)-scoped token the app origin mints into a frame's
// src (av-c5aq); token_test.go covers what happens without one.
func serve(t *testing.T, rd *Renderer, id string) string {
	t.Helper()
	w := httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/"+id, id, 1))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// The render doc inlines live state and a per-artifact CSP, so it must never be
// cached — a stale cached doc is exactly what caused an iframe to keep running
// an old shim after a redeploy.
func TestServeArtifactIsNotCacheable(t *testing.T) {
	rd, _ := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")

	w := httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/abc", "abc", 1))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("render doc must be Cache-Control: no-store, got %q", cc)
	}
	if !strings.Contains(w.Body.String(), "window.parent.postMessage") {
		t.Fatalf("shim not injected into served doc")
	}
}

// av-st7c AC 1/4: clear() must reach the server, not just empty the in-memory
// cache — the original bug looked like a successful wipe until the next
// render re-inlined every key. This exercises the same path the host bridge's
// 'clear' op drives (Store.ClearState) and asserts a reload shows no state.
func TestClearThenReloadDoesNotResurrectState(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	if err := st.SetState(ctx, 1, "abc", "a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(ctx, 1, "abc", "b", "2"); err != nil {
		t.Fatal(err)
	}
	before := serve(t, rd, "abc")
	if !strings.Contains(before, `"a":"1"`) || !strings.Contains(before, `"b":"2"`) {
		t.Fatalf("expected both keys inlined before clear: %s", before)
	}

	if err := st.ClearState(ctx, 1, "abc"); err != nil {
		t.Fatal(err)
	}

	after := serve(t, rd, "abc")
	if strings.Contains(after, `"a"`) || strings.Contains(after, `"b"`) {
		t.Fatalf("state resurrected after clear+reload: %s", after)
	}
	if !strings.Contains(after, "var cache = {};") {
		t.Fatalf("expected an empty inlined cache after clear, got: %s", after)
	}
}

// av-ms3r AC 1/3/5: removeItem must delete the row, not tombstone it as ”. A
// reload's inlined cache must not contain the key at all — getItem(k) reads
// straight off that cache (Object.prototype.hasOwnProperty), so an absent key
// there is exactly what makes getItem return null instead of ”. Absence from
// the cache also means it can't surface in Object.keys(cache), which is what
// length and key(n) enumerate.
func TestRemoveItemThenReloadKeyIsGone(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	if err := st.SetState(ctx, 1, "abc", "gone", "x"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(ctx, 1, "abc", "stays", "y"); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteState(ctx, 1, "abc", "gone"); err != nil {
		t.Fatal(err)
	}

	after := serve(t, rd, "abc")
	if strings.Contains(after, `"gone"`) {
		t.Fatalf("removed key still inlined after reload — tombstone regression: %s", after)
	}
	if !strings.Contains(after, `"stays":"y"`) {
		t.Fatalf("unrelated key must survive the delete: %s", after)
	}
}

// av-ms3r AC 2/5 (regression guard): an explicit empty string is a legitimate
// value and must remain distinguishable from a delete — the fix must not
// reintroduce a ” sentinel in the other direction by treating stored empty
// strings as absent.
func TestSetItemEmptyStringThenReloadStaysEmptyString(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	if err := st.SetState(ctx, 1, "abc", "note", ""); err != nil {
		t.Fatal(err)
	}

	after := serve(t, rd, "abc")
	if !strings.Contains(after, `"note":""`) {
		t.Fatalf("an intentionally empty value must survive a reload as '', not be dropped: %s", after)
	}
}
