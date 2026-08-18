package render

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/blob/blobtest"
	"github.com/momja/Exhibit/internal/store"
)

// av-52ll. The render surface is the read path that matters most — it is the
// one a visitor waits on — and it is also the one whose 404 depends on Get
// failing at Get rather than mid-response. Both are asserted here against a
// real bucket; it skips without one (see blobtest).

func TestServeArtifactFromABucket(t *testing.T) {
	bl := blobtest.S3OrSkip(t)
	ctx := context.Background()

	dbf, err := os.CreateTemp(t.TempDir(), "render-s3-*.db")
	if err != nil {
		t.Fatal(err)
	}
	dbf.Close()
	st, err := store.OpenSQLite(dbf.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	const id, blobID = "bucket-artifact", "bucket-artifact-blob"
	const body = "<html><head></head><body><h1>served from a bucket</h1></body></html>"
	if err := bl.Put(ctx, blobID, strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(ctx, &store.Artifact{
		ID: id, OwnerID: 1, Title: "t", SourceBlobID: blobID, Tier: 1,
	}); err != nil {
		t.Fatal(err)
	}

	rd := New(Config{
		Store: st, Blob: bl,
		AppOrigin: "https://app.test", RenderOrigin: "https://render.test",
		Tokens: testTokens,
	})

	w := httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/"+id, id, 1))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "served from a bucket") {
		t.Error("the served document must carry the artifact body read from the bucket")
	}
	// And the preamble still wraps it: the backend swap must be invisible above
	// the Blob interface, including to the security envelope.
	if got := w.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("the per-artifact CSP must be set regardless of which backend holds the body")
	}

	// A row whose object is missing is a 404 from the body lookup, not a 500
	// half-way through a 200 — which only holds because Get fails at Get.
	if err := bl.Delete(ctx, blobID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/"+id, id, 1))
	if w.Code != 404 {
		t.Fatalf("expected 404 for an artifact whose object is gone, got %d", w.Code)
	}
}
