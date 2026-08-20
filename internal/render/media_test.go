package render

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/store"
)

// permissionsPolicy returns the value of one Permissions-Policy feature (e.g.
// "camera") from a header value, and whether the feature was named at all.
// Absence matters: an unnamed feature keeps the browser's default, which for
// camera and microphone is "allowed for same-origin documents" — the opposite
// of what a denied artifact needs.
func permissionsPolicy(t *testing.T, header, feature string) (string, bool) {
	t.Helper()
	for _, f := range strings.Split(header, ",") {
		f = strings.TrimSpace(f)
		if v, ok := strings.CutPrefix(f, feature+"="); ok {
			return v, true
		}
	}
	return "", false
}

// The header names exactly camera and microphone, and each is (self) only when
// the artifact's approval says so. Nothing else is named: this header answers
// one question, and every other Permissions-Policy feature keeps its default
// rather than acquiring a second policy surface beside the CSP.
func TestBuildPermissionsPolicy(t *testing.T) {
	cases := []struct {
		camera, microphone  bool
		wantCamera, wantMic string
	}{
		{false, false, "()", "()"},
		{true, false, "(self)", "()"},
		{false, true, "()", "(self)"},
		{true, true, "(self)", "(self)"},
	}
	for _, tc := range cases {
		pp := buildPermissionsPolicy(tc.camera, tc.microphone)
		cam, ok := permissionsPolicy(t, pp, "camera")
		if !ok || cam != tc.wantCamera {
			t.Fatalf("camera=%v,mic=%v: got %q, want camera=%s", tc.camera, tc.microphone, pp, tc.wantCamera)
		}
		mic, ok := permissionsPolicy(t, pp, "microphone")
		if !ok || mic != tc.wantMic {
			t.Fatalf("camera=%v,mic=%v: got %q, want microphone=%s", tc.camera, tc.microphone, pp, tc.wantMic)
		}
		// Two features, not three: an added feature here is a policy decision,
		// never a side effect.
		if n := strings.Count(pp, "="); n != 2 {
			t.Fatalf("Permissions-Policy must name camera and microphone only, got %q", pp)
		}
	}
}

// The whole reason this header exists: a browser permission is granted per
// *origin*, and every artifact shares one render origin. Without a per-document
// policy, a visitor who allowed the camera for one artifact opened directly has
// allowed it for every artifact on that origin. So an unapproved artifact must
// be served camera=() — a denial the browser enforces even when the origin's
// permission is already granted.
func TestServeArtifactDeniesDevicesUntilApproved(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()

	w := httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/abc", "abc", 1))
	pp := w.Header().Get("Permissions-Policy")
	if cam, _ := permissionsPolicy(t, pp, "camera"); cam != "()" {
		t.Fatalf("an unapproved artifact must be denied the camera, got %q", pp)
	}
	if mic, _ := permissionsPolicy(t, pp, "microphone"); mic != "()" {
		t.Fatalf("an unapproved artifact must be denied the microphone, got %q", pp)
	}

	// Approving one device must not hand over the other: two grants, not one.
	if err := st.UpdateArtifact(ctx, 1, "abc", map[string]any{"camera_approved": true}); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/abc", "abc", 1))
	pp = w.Header().Get("Permissions-Policy")
	if cam, _ := permissionsPolicy(t, pp, "camera"); cam != "(self)" {
		t.Fatalf("an approved camera must be permitted, got %q", pp)
	}
	if mic, _ := permissionsPolicy(t, pp, "microphone"); mic != "()" {
		t.Fatalf("approving the camera must not permit the microphone, got %q", pp)
	}
}

// A share publishes the artifact as its owner sees it, so it carries the
// owner's approvals. The visitor's own browser prompt is still the gate on
// their hardware — this only decides whether the artifact may ask at all.
func TestServeShareCarriesOwnerApprovals(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>hi</body></html>")
	ctx := context.Background()
	if err := st.UpdateArtifact(ctx, 1, "abc", map[string]any{"microphone_approved": true}); err != nil {
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
	pp := w.Header().Get("Permissions-Policy")
	if mic, _ := permissionsPolicy(t, pp, "microphone"); mic != "(self)" {
		t.Fatalf("a share must carry the owner's microphone approval, got %q", pp)
	}
	if cam, _ := permissionsPolicy(t, pp, "camera"); cam != "()" {
		t.Fatalf("a share must not widen what the owner approved, got %q", pp)
	}
}

// A widget's authority is a strict subset of its artifact's: it renders
// unattended in a card behind pointer-events:none, where there is no gesture to
// attribute a device prompt to. So an approved artifact's tile is still denied
// both devices.
func TestServeWidgetDeniesDevicesEvenWhenArtifactApproved(t *testing.T) {
	rd := newWidgetRenderer(t, "abc", "<html><head></head><body>tool</body></html>",
		"<html><head></head><body>tile</body></html>")
	ctx := context.Background()
	if err := rd.cfg.Store.UpdateArtifact(ctx, 1, "abc", map[string]any{
		"camera_approved":     true,
		"microphone_approved": true,
	}); err != nil {
		t.Fatal(err)
	}

	w := serveWidget(t, rd, "abc")
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	pp := w.Header().Get("Permissions-Policy")
	if cam, _ := permissionsPolicy(t, pp, "camera"); cam != "()" {
		t.Fatalf("a widget must be denied the camera whatever its artifact holds, got %q", pp)
	}
	if mic, _ := permissionsPolicy(t, pp, "microphone"); mic != "()" {
		t.Fatalf("a widget must be denied the microphone whatever its artifact holds, got %q", pp)
	}
}

// The media gate (av-mv3k): a capture device cannot be delivered into this
// frame by anyone — the opaque origin is refused one, and a MediaStreamTrack is
// not transferable — so the preamble replaces getUserMedia with a call that
// hands the decision to the host and then *settles*, rather than hanging on a
// stream that is never coming.
func TestShimInstallsMediaGate(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, false, false, nil)

	// The message shape the host's media listener validates, and the reply the
	// frame correlates by request id.
	for _, marker := range []string{"__avMedia", "__avMediaResult"} {
		if !strings.Contains(doc, marker) {
			t.Fatalf("shim missing the media gate message %s: %s", marker, doc)
		}
	}
	// The API surface is actually replaced, not merely referenced.
	if !strings.Contains(doc, "getUserMedia") || !strings.Contains(doc, "requestMedia") {
		t.Fatalf("shim must replace navigator.mediaDevices.getUserMedia: %s", doc)
	}
	// Requests are pinned to the app origin, never broadcast — same rule as
	// every other message the preamble sends.
	if !strings.Contains(doc, "API_ORIGIN") {
		t.Fatalf("media messages must be pinned to the app origin: %s", doc)
	}
	// Only the devices the constraints named are reported, so the host can
	// prompt for exactly those and persist only those.
	if !strings.Contains(doc, "wantsAudio") || !strings.Contains(doc, "wantsVideo") {
		t.Fatalf("shim must report which devices were asked for: %s", doc)
	}
	// Every reply rejects. Nothing here resolves a stream, and a future edit
	// that made it look like one would be claiming a capability the frame does
	// not have.
	if !strings.Contains(doc, "p.reject(new DOMException(") {
		t.Fatalf("the media gate must settle by rejecting: %s", doc)
	}
	if strings.Contains(doc, "new MediaStream(") {
		t.Fatalf("the frame must not fabricate a MediaStream: %s", doc)
	}
	// The constraints themselves stay in the frame: the host acquires nothing,
	// so sending them would be data travelling for no reader.
	if strings.Contains(doc, "constraints: plain") {
		t.Fatalf("constraints must not cross the boundary: %s", doc)
	}
}

// Framed-only, like every other preamble install: opened top-level the document has a
// real origin where getUserMedia works natively under the artifact's own
// Permissions-Policy header, and gating it there would break the one context
// where the capability is reachable.
func TestShimMediaGateIsFramedOnly(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, false, false, nil)
	guard := strings.Index(doc, "if (window.parent !== window) {")
	media := strings.Index(doc, "__avMedia")
	if guard < 0 || media < guard {
		t.Fatalf("the media gate must sit inside the framed guard: guard=%d media=%d", guard, media)
	}
}
