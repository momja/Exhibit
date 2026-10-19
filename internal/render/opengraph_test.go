package render

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/store"
)

var genericPNG = []byte("generic-card-png")

// fakeSnapshotter stands in for the browser: it records what it was asked to
// draw and answers with fixed bytes, or an error.
type fakeSnapshotter struct {
	calls atomic.Int32
	mu    sync.Mutex
	urls  []string
	png   []byte
	err   error
}

func (f *fakeSnapshotter) Snapshot(_ context.Context, pageURL string) ([]byte, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.urls = append(f.urls, pageURL)
	f.mu.Unlock()
	return f.png, f.err
}

// newPreviewRenderer is a renderer with one artifact titled title, its
// anonymous link "the-link", a grant "the-grant" and the generic card, and the
// snapshotter given (nil for none).
func newPreviewRenderer(t *testing.T, title string, snap Snapshotter) (*Renderer, *store.SQLiteStore) {
	t.Helper()
	rd, st := newTestRenderer(t, "other", "<html></html>")
	ctx := context.Background()
	if err := rd.cfg.Blob.Put(ctx, "abc-blob", strings.NewReader(
		"<html><head><title>own</title></head><body>shared</body></html>")); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(ctx, &store.Artifact{
		ID: "abc", OwnerID: 1, Title: title, SourceBlobID: "abc-blob", Tier: 1,
	}); err != nil {
		t.Fatal(err)
	}
	recipient, err := st.UpsertUser(ctx, "sub-recipient", "recipient@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateShare(ctx, 1, &store.Share{ID: "the-link", ArtifactID: "abc"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateShare(ctx, 1, &store.Share{ID: "the-grant", ArtifactID: "abc", RecipientID: &recipient.ID}); err != nil {
		t.Fatal(err)
	}
	rd.cfg.PreviewImagePNG = genericPNG
	rd.cfg.Snapshotter = snap
	rd.previews = newPreviewCache(snap)
	return rd, st
}

func servePreviewImage(t *testing.T, rd *Renderer, shareID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/s/"+shareID+"/og.png", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shareID", shareID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	rd.ServeSharePreviewImage(w, req)
	return w
}

// A live link's document names the artifact and points at its own preview
// image, ahead of anything else in <head> — a crawler that stops reading early
// must still have read these.
func TestALiveLinkCarriesThePreviewTags(t *testing.T) {
	rd, _ := newPreviewRenderer(t, "Run Log", nil)

	w := serveShareRaw(t, rd, "the-link")
	if w.Code != 200 {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	body := w.Body.String()
	for _, want := range []string{
		`<meta property="og:type" content="website">`,
		`<meta property="og:title" content="Run Log">`,
		`<meta property="og:description" content="` + previewDescription + `">`,
		`<meta property="og:url" content="https://render.test/s/the-link">`,
		`<meta property="og:image" content="https://render.test/s/the-link/og.png">`,
		`<meta property="og:image:type" content="image/png">`,
		`<meta property="og:image:width" content="1056">`,
		`<meta property="og:image:height" content="528">`,
		`<meta name="twitter:card" content="summary_large_image">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("share document lacks %s", want)
		}
	}
	tags, preamble, own := strings.Index(body, `og:title`), strings.Index(body, "<script"), strings.Index(body, "<title>own")
	if tags < 0 || tags > preamble || tags > own {
		t.Fatalf("the preview tags must come first in <head>: tags@%d preamble@%d artifact@%d", tags, preamble, own)
	}
}

// The tags belong to the share route alone. The token-gated document is never
// crawled, and carrying them there would put a share URL into a page that has
// no share.
func TestOnlyTheShareDocumentCarriesPreviewTags(t *testing.T) {
	rd, _ := newPreviewRenderer(t, "Run Log", nil)
	if body := serve(t, rd, "abc"); strings.Contains(body, "og:") {
		t.Fatal("the owner's own render must not carry link-preview tags")
	}
}

// The title is the owner's text, or a fetched page's. It is an attribute value
// here and must stay one.
func TestPreviewTagsEscapeTheTitle(t *testing.T) {
	rd, _ := newPreviewRenderer(t, `"><script>alert(1)</script>`, nil)
	body := serveShareRaw(t, rd, "the-link").Body.String()
	if strings.Contains(body, `"><script>alert(1)`) {
		t.Fatal("the title escaped its attribute")
	}
	if !strings.Contains(body, `content="&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;"`) {
		t.Fatal("the escaped title is missing from og:title")
	}
}

// Every id that is not a live link — a grant's, a revoked link's, one never
// issued — answers the same 404, carrying Exhibit's generic tags and nothing
// that came from an artifact. Identical bytes, or the preview of a pasted id
// would say which kind of dead it is.
func TestDeadLinksCarryOnlyGenericTagsAndAreIndistinguishable(t *testing.T) {
	rd, st := newPreviewRenderer(t, "Secret Ledger", nil)
	// Revoking the live link is what makes a revoked id; a grant and an
	// unissued id stand beside it.
	before := serveShareRaw(t, rd, "the-link")
	if before.Code != 200 {
		t.Fatalf("the link must serve before it is revoked, got %d", before.Code)
	}
	if err := st.DeleteShare(context.Background(), 1, "the-link"); err != nil {
		t.Fatal(err)
	}

	var first string
	for _, id := range []string{"the-grant", "the-link", "never-issued"} {
		w := serveShareRaw(t, rd, id)
		if w.Code != 404 {
			t.Fatalf("%s: got %d, want 404", id, w.Code)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("%s: Cache-Control = %q, want no-store", id, cc)
		}
		body := w.Body.String()
		if strings.Contains(body, "Secret Ledger") || strings.Contains(body, "/s/") {
			t.Fatalf("%s: a dead link's page carries artifact-derived metadata: %s", id, body)
		}
		for _, want := range []string{
			`<meta property="og:title" content="Exhibit">`,
			`<meta property="og:image" content="https://render.test/og.png">`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: lacks %s", id, want)
			}
		}
		if first == "" {
			first = body
		} else if body != first {
			t.Fatalf("%s answered differently from the-grant:\n%s\nvs\n%s", id, body, first)
		}
	}
}

// The live link's image is its widget, drawn through the share's own widget
// URL — never the artifact document — and drawn once for a burst of unfurls.
func TestPreviewImageIsTheSharedWidgetSnapshotted(t *testing.T) {
	snap := &fakeSnapshotter{png: []byte("widget-png")}
	rd, _ := newPreviewRenderer(t, "Run Log", snap)

	for range 3 {
		w := servePreviewImage(t, rd, "the-link")
		if w.Code != 200 || w.Body.String() != "widget-png" {
			t.Fatalf("got %d %q", w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); ct != "image/png" {
			t.Fatalf("Content-Type = %q", ct)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", cc)
		}
	}
	if n := snap.calls.Load(); n != 1 {
		t.Fatalf("three unfurls drew %d snapshots, want 1", n)
	}
	if got := snap.urls[0]; got != "https://render.test/s/the-link/widget" {
		t.Fatalf("snapshotted %q, want the shared widget", got)
	}
}

// Revoking the link ends its picture on the next request, though the snapshot
// is still in the cache: the share row is resolved first, every time.
func TestARevokedLinkServesNoCachedSnapshot(t *testing.T) {
	snap := &fakeSnapshotter{png: []byte("widget-png")}
	rd, st := newPreviewRenderer(t, "Run Log", snap)
	if w := servePreviewImage(t, rd, "the-link"); w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
	if err := st.DeleteShare(context.Background(), 1, "the-link"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"the-link", "the-grant", "never-issued"} {
		w := servePreviewImage(t, rd, id)
		if w.Code != 404 || strings.Contains(w.Body.String(), "widget-png") {
			t.Fatalf("%s: got %d %q, want a plain 404", id, w.Code, w.Body.String())
		}
	}
}

// No browser, or a browser that failed, is the generic card — and a failure
// is remembered like a success, so a widget that cannot be drawn is not
// retried on every unfurl.
func TestPreviewImageFallsBackToTheGenericCard(t *testing.T) {
	rd, _ := newPreviewRenderer(t, "Run Log", nil)
	if w := servePreviewImage(t, rd, "the-link"); w.Code != 200 || w.Body.String() != string(genericPNG) {
		t.Fatalf("with no snapshotter: got %d %q", w.Code, w.Body.String())
	}

	failing := &fakeSnapshotter{err: errors.New("browser gone")}
	rd, _ = newPreviewRenderer(t, "Run Log", failing)
	for range 2 {
		if w := servePreviewImage(t, rd, "the-link"); w.Code != 200 || w.Body.String() != string(genericPNG) {
			t.Fatalf("with a failing snapshotter: got %d %q", w.Code, w.Body.String())
		}
	}
	if n := failing.calls.Load(); n != 1 {
		t.Fatalf("a failed snapshot was retried: %d calls", n)
	}
}

// A snapshot answers for its link for previewTTL and is redrawn after, so the
// picture follows the widget.
func TestPreviewSnapshotsExpire(t *testing.T) {
	snap := &fakeSnapshotter{png: []byte("widget-png")}
	c := newPreviewCache(snap)
	now := time.Unix(1_800_000_000, 0)
	c.now = func() time.Time { return now }

	c.get(context.Background(), "k", "https://render.test/s/k/widget")
	now = now.Add(previewTTL - time.Second)
	c.get(context.Background(), "k", "https://render.test/s/k/widget")
	if n := snap.calls.Load(); n != 1 {
		t.Fatalf("redrawn within the TTL: %d calls", n)
	}
	now = now.Add(2 * time.Second)
	c.get(context.Background(), "k", "https://render.test/s/k/widget")
	if n := snap.calls.Load(); n != 2 {
		t.Fatalf("not redrawn after the TTL: %d calls", n)
	}
}

// The cache holds a bounded number of pictures however many links are
// unfurled.
func TestPreviewCacheIsBounded(t *testing.T) {
	c := newPreviewCache(&fakeSnapshotter{png: []byte("p")})
	for i := range previewCacheEntries + 50 {
		c.get(context.Background(), "k"+string(rune('a'+i%26))+strings.Repeat("x", i), "u")
	}
	if n := len(c.entries); n > previewCacheEntries {
		t.Fatalf("cache holds %d entries, cap is %d", n, previewCacheEntries)
	}
}

// The generic card is static artwork, served to anyone.
func TestGenericPreviewImage(t *testing.T) {
	rd, _ := newPreviewRenderer(t, "Run Log", nil)
	w := httptest.NewRecorder()
	rd.ServePreviewImage(w, httptest.NewRequest("GET", "/og.png", nil))
	if w.Code != 200 || w.Body.String() != string(genericPNG) || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("got %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}

	bare := New(Config{})
	w = httptest.NewRecorder()
	bare.ServePreviewImage(w, httptest.NewRequest("GET", "/og.png", nil))
	if w.Code != 404 {
		t.Fatalf("with no card configured: got %d, want 404", w.Code)
	}
}
