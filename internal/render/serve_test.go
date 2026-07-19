package render

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/store"
)

// newTestRenderer builds a Renderer backed by a temp SQLite store + FS blob,
// with one artifact whose body is the given HTML.
func newTestRenderer(t *testing.T, id, body string) *Renderer {
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

	return New(Config{Store: st, Blob: bl, AppOrigin: "https://app.test", RenderOrigin: "https://render.test"})
}

// The render doc inlines live state and a per-artifact CSP, so it must never be
// cached — a stale cached doc is exactly what caused an iframe to keep running
// an old shim after a redeploy.
func TestServeArtifactIsNotCacheable(t *testing.T) {
	rd := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")

	req := httptest.NewRequest("GET", "/a/abc", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("artifactID", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	rd.ServeArtifact(w, req)

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
