package render

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// serveShareRaw fetches /s/:id the way whoever holds the URL would — no
// credential, no token, nothing but the id — and returns whatever came back.
// embed_test.go's serveShare requires a 200, which is the right assertion for
// tests about a share that renders and the wrong one for tests about a share
// that must not.
func serveShareRaw(t *testing.T, rd *Renderer, shareID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/s/"+shareID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shareID", shareID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	rd.ServeShare(w, req)
	return w
}

// av-lrae: grants and anonymous links share one table, and /s/:id resolves a
// row by id alone — so the moment the accessor stops distinguishing them,
// every grant id becomes a working public URL. Grant an artifact to three
// people and you have silently minted three unguessable links serving it to
// anyone with no account; switch the public link off believing the artifact is
// now private, and three ids still serve it.
//
// Both halves are asserted in one test on purpose. Each is only interesting
// against the other: a refusal that also refused the link would be a broken
// share route, and a link that served would say nothing about the grant. A
// refactor that breaks either shows up here against the one it did not break.
func TestAShareLinkServesAndAGrantIsNotAURL(t *testing.T) {
	rd, st := newTestRenderer(t, "abc", "<html><head></head><body>shared</body></html>")
	ctx := context.Background()

	recipient, err := st.UpsertUser(ctx, "sub-recipient", "recipient@example.test")
	if err != nil {
		t.Fatal(err)
	}
	createdLink, err := st.CreateAnonymousLink(ctx, 1, "abc")
	if err != nil {
		t.Fatal(err)
	}
	createdGrant, err := st.CreateGrant(ctx, 1, "abc", recipient.ID)
	if err != nil {
		t.Fatal(err)
	}

	if w := serveShareRaw(t, rd, createdLink.ID); w.Code != 200 {
		t.Fatalf("the anonymous link must still serve the artifact, got %d: %s", w.Code, w.Body.String())
	}

	// And the grant is not a URL. 404 rather than 403, and the *same* 404 an
	// id that was never issued gets — otherwise the route answers whether a
	// given id names a live grant, which is a membership oracle over exactly
	// the ids a grant model must not leak.
	grant := serveShareRaw(t, rd, createdGrant.ID)
	unknown := serveShareRaw(t, rd, "no-such-share")
	if grant.Code != 404 {
		t.Fatalf("a grant id must not serve the artifact at /s/, got %d: %s", grant.Code, grant.Body.String())
	}
	if grant.Code != unknown.Code || grant.Body.String() != unknown.Body.String() {
		t.Fatalf("a grant id must answer exactly what an unissued id answers: %d %q vs %d %q",
			grant.Code, grant.Body.String(), unknown.Code, unknown.Body.String())
	}

	// Revoking the link revokes public reach, in full. The grant row outlives
	// it — that is the point of a grant — and must not keep the artifact
	// publicly served afterwards.
	if err := st.DeleteShare(ctx, 1, createdLink.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{createdLink.ID, createdGrant.ID} {
		if w := serveShareRaw(t, rd, id); w.Code != 404 {
			t.Fatalf("nothing may serve %q publicly once the link is revoked, got %d", id, w.Code)
		}
	}
	shares, err := st.ListArtifactShares(ctx, 1, "abc")
	if err != nil || len(shares) != 1 {
		t.Fatalf("revoking the public link must leave exactly the grant: %v %v", shares, err)
	}
	if sh := shares[0]; sh.ID != createdGrant.ID || sh.RecipientID == nil || *sh.RecipientID != recipient.ID {
		t.Fatalf("revoking the public link must leave the grant unchanged: %+v", sh)
	}
}
