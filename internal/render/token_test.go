package render

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/store"
)

// testTokens signs every credential the render tests present. Real deployments
// derive this key from the server secret; a test only needs the minting and the
// verifying side to agree.
var testTokens = rendertoken.NewRandomSigner()

// renderRequest builds the request the render surface actually receives: the
// chi URL param the handlers read, plus the (artifact, owner)-scoped token the
// app origin minted into the frame's src.
func renderRequest(path, artifactID string, ownerID int64) *http.Request {
	return rawRequest(path+"?"+rendertoken.Param+"="+testTokens.Mint(artifactID, ownerID), artifactID)
}

// rawRequest is the same plumbing with whatever query string the caller wants —
// including none, which is what an outsider who merely guessed an id would send.
func rawRequest(target, artifactID string) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("artifactID", artifactID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// putSecondOwnerArtifact adds an artifact belonging to owner 2 to an existing
// test store, so one Renderer can be asked for two tenants' documents. It
// writes its own body through the Renderer's blob store rather than reusing
// another artifact's blob id, so a cross-tenant leak would actually surface
// this artifact's distinct content instead of trivially passing.
func putSecondOwnerArtifact(t *testing.T, rd *Renderer, st *store.SQLiteStore, id, body string) {
	t.Helper()
	blobID := id + "-blob"
	if err := rd.cfg.Blob.Put(context.Background(), blobID, strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(context.Background(), &store.Artifact{
		ID: id, OwnerID: 2, Title: "theirs", SourceBlobID: blobID, Tier: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

// av-c5aq AC#1. Before this ticket the id WAS the credential: anyone who
// learned a UUID rendered the artifact and everything the render surface
// inlines into it, including its state. Knowing the id must no longer be
// enough.
func TestRenderRoutesRejectRequestsWithoutAToken(t *testing.T) {
	rd, _ := newTestRenderer(t, "abc", "<html><head></head><body>SECRET-BODY</body></html>")

	for _, tc := range []struct {
		name   string
		target string
		serve  func(http.ResponseWriter, *http.Request)
	}{
		{"artifact, no token", "/a/abc", rd.ServeArtifact},
		{"artifact, empty token", "/a/abc?t=", rd.ServeArtifact},
		{"artifact, garbage token", "/a/abc?t=not.a.token", rd.ServeArtifact},
		{"widget, no token", "/w/abc", rd.ServeWidget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.serve(w, rawRequest(tc.target, "abc"))
			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d", w.Code)
			}
			if strings.Contains(w.Body.String(), "SECRET-BODY") {
				t.Fatalf("served the artifact body without a token: %s", w.Body.String())
			}
		})
	}
}

// av-c5aq AC#1, the cross-tenant half: a perfectly valid token proves who the
// requester is, and that answer must not be another owner's artifact.
func TestValidTokenDoesNotRenderAnotherOwnersArtifact(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>MINE</body></html>")
	putSecondOwnerArtifact(t, rd, st, "theirs", "<html><head></head><body>THEIRS</body></html>")

	// Owner 1 holds a real token — minted for the very artifact they are
	// asking for — but the artifact belongs to owner 2.
	w := httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/theirs", "theirs", 1))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another owner's artifact, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "THEIRS") {
		t.Fatalf("cross-tenant read: %s", w.Body.String())
	}
}

// av-c5aq AC#2. The scope is the signature — the artifact id is mixed into the
// MAC rather than carried as a comparable field — so a token for A is not a
// token for B by construction, not by a check someone could delete.
func TestTokenForOneArtifactDoesNotRenderAnother(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>FIRST</body></html>")
	if err := st.PutArtifact(context.Background(), &store.Artifact{
		ID: "def", OwnerID: 1, Title: "second", SourceBlobID: "abc-blob", Tier: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// A token for "abc", replayed against "def" — same owner, same key, wrong
	// artifact.
	stolen := testTokens.Mint("abc", 1)
	w := httptest.NewRecorder()
	rd.ServeArtifact(w, rawRequest("/a/def?"+rendertoken.Param+"="+stolen, "def"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a token scoped to a different artifact, got %d", w.Code)
	}
}

// av-c5aq AC#3. Short expiry is what makes it acceptable for the artifact to
// read its own token out of location.href on a top-level open, so the deadline
// has to actually be enforced.
func TestExpiredTokenIsRejected(t *testing.T) {
	rd, _ := newTestRenderer(t, "abc", "<html><head></head><body>SECRET-BODY</body></html>")

	expired := testTokens.MintFor("abc", 1, -time.Minute)
	w := httptest.NewRecorder()
	rd.ServeArtifact(w, rawRequest("/a/abc?"+rendertoken.Param+"="+expired, "abc"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an expired token, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "SECRET-BODY") {
		t.Fatalf("expired token still rendered the artifact: %s", w.Body.String())
	}
}

// A Renderer with no Signer can verify nothing, so it must serve nothing. The
// permissive reading of "no key configured" is an open render origin, which is
// the hole this ticket closes.
func TestRendererWithoutASignerServesNothing(t *testing.T) {
	rd, _ := newTestRenderer(t, "abc", "<html><head></head><body>SECRET-BODY</body></html>")
	rd.cfg.Tokens = nil

	w := httptest.NewRecorder()
	rd.ServeArtifact(w, rawRequest("/a/abc?"+rendertoken.Param+"=anything", "abc"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with no signer configured, got %d", w.Code)
	}
}

// A valid token still renders — the gate must not be a wall.
func TestValidTokenRendersTheArtifact(t *testing.T) {
	rd, _ := newTestRenderer(t, "abc", "<html><head></head><body>SECRET-BODY</body></html>")

	w := httptest.NewRecorder()
	rd.ServeArtifact(w, renderRequest("/a/abc", "abc", 1))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with a valid token, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "SECRET-BODY") {
		t.Fatalf("valid token did not render the body: %s", w.Body.String())
	}
}
