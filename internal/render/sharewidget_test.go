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

// newShareWidgetFixture builds a Renderer holding one artifact that has a body,
// a widget, owner state, an allowlist, and both device approvals — plus an
// anonymous link and a grant naming a recipient. The approvals are deliberate:
// the shared widget must deny both devices whatever the artifact holds, so the
// fixture approves both to make that subtraction observable.
func newShareWidgetFixture(t *testing.T) (*Renderer, *store.SQLiteStore, string, string) {
	t.Helper()

	dbf, err := os.CreateTemp(t.TempDir(), "sharewidget-*.db")
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
	ctx := context.Background()
	if err := bl.Put(ctx, "abc-body", strings.NewReader("<html><head></head><body>ARTIFACT-BODY</body></html>")); err != nil {
		t.Fatal(err)
	}
	if err := bl.Put(ctx, "abc-widget", strings.NewReader("<html><head></head><body>WIDGET-BODY</body></html>")); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(ctx, &store.Artifact{
		ID: "abc", OwnerID: 1, Title: "t", SourceBlobID: "abc-body", Tier: 1,
		WidgetBlobID:       "abc-widget",
		NetworkAllowlist:   []string{"https://api.example.com"},
		CameraApproved:     true,
		MicrophoneApproved: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(ctx, 1, "abc", 1, "runs", `[{"km":5}]`); err != nil {
		t.Fatal(err)
	}

	recipient, err := st.UpsertUser(ctx, "sub-recipient", "recipient@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateShare(ctx, 1, &store.Share{ID: "the-link", ArtifactID: "abc"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateShare(ctx, 1, &store.Share{
		ID: "the-grant", ArtifactID: "abc", RecipientID: &recipient.ID}); err != nil {
		t.Fatal(err)
	}

	rd := New(Config{
		Store: st, Blob: bl,
		AppOrigin: "https://app.test", RenderOrigin: "https://render.test",
		Tokens: testTokens,
	})
	return rd, st, "the-link", "the-grant"
}

// serveShareWidgetRaw fetches /s/:id/widget the way whoever holds the URL
// would — no credential, no token, nothing but the id — mirroring
// sharegrant_test.go's serveShareRaw for the artifact route.
func serveShareWidgetRaw(t *testing.T, rd *Renderer, shareID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/s/"+shareID+"/widget", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shareID", shareID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	rd.ServeShareWidget(w, req)
	return w
}

// The shared widget is ServeShare and ServeWidget composed: the widget blob
// renders, with the owner's state inlined — the same document the share's
// artifact inlines, glanceable. Read-only by construction: the widget preamble
// short-circuits the write-through, so there is no host here that could
// persist one anyway.
func TestShareWidgetServesTheWidgetWithOwnerState(t *testing.T) {
	rd, _, link, _ := newShareWidgetFixture(t)

	w := serveShareWidgetRaw(t, rd, link)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "WIDGET-BODY") || strings.Contains(body, "ARTIFACT-BODY") {
		t.Fatalf("shared widget must serve the widget body, not the artifact body: %s", body)
	}
	if !strings.Contains(body, `"runs"`) {
		t.Fatalf("shared widget must inline the owner's state, like the shared artifact: %s", body)
	}
	if !strings.Contains(body, "var WIDGET = true;") {
		t.Fatalf("shared widget must render in widget mode: %s", body)
	}
	if strings.Contains(body, "__avDownload") {
		t.Fatalf("shared widget must install no capability bridges: %s", body)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("shared widget doc must be Cache-Control: no-store, got %q", cc)
	}
}

// One allowlist, one CSP — and the share's framing, not /w/:id's. A shared
// widget's point is embedding off the gallery, so it carries the same
// frame-ancestors the shared artifact does. Devices stay denied whatever the
// artifact holds: a tile draws unattended, where no device prompt could belong.
func TestShareWidgetUsesArtifactCSPAndShareFraming(t *testing.T) {
	rd, _, link, _ := newShareWidgetFixture(t)

	w := serveShareWidgetRaw(t, rd, link)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src blob: data: https://api.example.com") {
		t.Fatalf("shared widget CSP must be built from the artifact's allowlist, got %q", csp)
	}
	if fa := frameAncestors(t, csp); fa != "*" {
		t.Fatalf("shared widget must carry the share framing policy, got %q", fa)
	}
	if pp := w.Header().Get("Permissions-Policy"); pp != "camera=(), microphone=()" {
		t.Fatalf("shared widget must deny both devices despite the artifact's approvals, got %q", pp)
	}
}

// The widget route resolves the same link row as the artifact route, so it
// inherits the same refusal: a grant's id is not a URL here either, and
// revoking the link revokes the tile in full.
func TestShareWidgetGrantIsNotAURL(t *testing.T) {
	rd, st, link, grant := newShareWidgetFixture(t)
	ctx := context.Background()

	if w := serveShareWidgetRaw(t, rd, link); w.Code != 200 {
		t.Fatalf("the anonymous link must serve the widget, got %d: %s", w.Code, w.Body.String())
	}
	got := serveShareWidgetRaw(t, rd, grant)
	unknown := serveShareWidgetRaw(t, rd, "no-such-share")
	if got.Code != 404 {
		t.Fatalf("a grant id must not serve the widget at /s/, got %d: %s", got.Code, got.Body.String())
	}
	if got.Code != unknown.Code || got.Body.String() != unknown.Body.String() {
		t.Fatalf("a grant id must answer exactly what an unissued id answers: %d %q vs %d %q",
			got.Code, got.Body.String(), unknown.Code, unknown.Body.String())
	}

	if err := st.DeleteShare(ctx, 1, link); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{link, grant} {
		if w := serveShareWidgetRaw(t, rd, id); w.Code != 404 {
			t.Fatalf("nothing may serve %q publicly once the link is revoked, got %d", id, w.Code)
		}
	}
}

// No widget of its own: the link serves the default tile (av-cp7j), the same
// monogram the gallery card shows, rather than a 404 an embed would display
// verbatim. It is a static document, so its policy is narrower than a widget
// render's: inline style and nothing else, and no artifact bytes or state.
func TestShareWidgetServesTheDefaultTileWithoutWidget(t *testing.T) {
	rd, _, _, _ := newShareWidgetFixture(t)

	// The fixture's artifact has a widget; an artifact without one shares the
	// same store but a different row.
	ctx := context.Background()
	bl := rd.cfg.Blob
	if err := bl.Put(ctx, "plain-body", strings.NewReader("<html><head></head><body>plain</body></html>")); err != nil {
		t.Fatal(err)
	}
	if err := rd.cfg.Store.PutArtifact(ctx, &store.Artifact{
		ID: "plain", OwnerID: 1, Title: "Run Log", SourceBlobID: "plain-body", Tier: 1,
		NetworkAllowlist: []string{"https://api.example.com"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := rd.cfg.Store.SetState(ctx, 1, "plain", 1, "runs", `SECRET-STATE`); err != nil {
		t.Fatal(err)
	}
	if err := rd.cfg.Store.CreateShare(ctx, 1, &store.Share{ID: "plain-link", ArtifactID: "plain"}); err != nil {
		t.Fatal(err)
	}

	w := serveShareWidgetRaw(t, rd, "plain-link")
	if w.Code != 200 {
		t.Fatalf("expected the default tile, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, ">RL</span>") {
		t.Errorf("the tile must carry the artifact's monogram:\n%s", body)
	}
	for _, leak := range []string{"plain</body>", "SECRET-STATE", "<script"} {
		if strings.Contains(body, leak) {
			t.Errorf("the default tile must not contain %q", leak)
		}
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors") {
		t.Errorf("the tile permits inline style and nothing else, got %q", csp)
	}
	if strings.Contains(csp, "api.example.com") {
		t.Errorf("a static tile needs none of the artifact's allowlist, got %q", csp)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}
