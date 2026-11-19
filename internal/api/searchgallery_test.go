package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchGalleryAccessibleCards(t *testing.T) {
	in := newGrantedInstance(t)
	ctx := t.Context()
	seedSharedGallery(t, in, 27)
	owned := seedOwnedArtifact(t, in.ro, in.ownerTwo, "owned-chart", "Owned chart", "dashboard", "own state")
	private := seedOwnedArtifact(t, in.ro, in.ownerOne, "private-chart", "Private chart", "dashboard", "private state")
	public := seedOwnedArtifact(t, in.ro, in.ownerOne, "public-chart", "Public chart", "dashboard", "public state")
	_, err := in.ro.cfg.Store.CreateAnonymousLink(ctx, in.ownerOne, public)
	require.NoError(t, err)
	var ids []string
	for path := "/search?q=chart"; path != ""; {
		page := in.get(t, path, in.cookieTwo)
		require.Equal(t, http.StatusOK, page.Code)
		cards, next := galleryCardsAndNext(page.Body.String())
		ids = append(ids, cards...)
		if next != "" {
			assert.True(t, strings.HasPrefix(next, "/partials/gallery-grid/search?"))
			assert.Contains(t, next, "q=chart")
		}
		path = next
	}
	assert.Len(t, ids, 28)
	assert.Contains(t, ids, owned)
	assert.NotContains(t, ids, private)
	assert.NotContains(t, ids, public)
	full := in.get(t, "/search?q=chart", in.cookieTwo).Body.String()
	assert.Contains(t, full, `action="/search" method="get"`)
	assert.Contains(t, full, `class="gallery-page-search"`)
	assert.Contains(t, full, `return_to=%2Fsearch%3Fq%3Dchart`)
	assert.Equal(t, 1, strings.Count(full, `role="search"`))
	assert.NotContains(t, full, `gallery-filter-trigger`)
	// A shared card keeps the recipient's widget/state identity and has no
	// owner actions or capability controls, even beside editable owned cards.
	shared := in.get(t, "/search?q="+url.QueryEscape(ownerOneTitle), in.cookieTwo).Body.String()
	for _, forbidden := range []string{`data-capability-trigger`, `/edit?`} {
		assert.NotContains(t, shared, forbidden)
	}
	assert.Contains(t, shared, `class="card-owner-name">one@example.test`)
	assert.Contains(t, shared, `/widget/viewer/open`)
	creds := renderCredentialsIn(shared)
	require.Len(t, creds, 1)
	claims, err := in.ro.tokens.Verify(creds[0].token, in.granted)
	require.NoError(t, err)
	assert.Equal(t, in.ownerOne, claims.OwnerID)
	assert.Equal(t, in.ownerTwo, claims.ViewerID)
	doc := renderGet(t, in.ro, creds[0].url)
	require.Equal(t, http.StatusOK, doc.Code)
	assert.Contains(t, doc.Body.String(), recipientState)
	assert.NotContains(t, doc.Body.String(), ownerOneState)
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(ctx, in.ownerOne, in.granted, in.ownerTwo))
	assert.NotContains(t, in.get(t, "/search?q="+url.QueryEscape(ownerOneTitle), in.cookieTwo).Body.String(), in.granted)
	gridReq := in.get(t, "/partials/gallery-grid/search?q=chart", in.cookieTwo)
	assert.Equal(t, "/search?q=chart", gridReq.Header().Get("HX-Replace-Url"))
	assert.Contains(t, normalizePerRender(full), normalizePerRender(gridReq.Body.String()))
	// Clearing returns a prompt, rather than leaking a different library or
	// retaining results from the last nonempty query.
	for _, path := range []string{"/search", "/partials/gallery-grid/search?q=%20%20"} {
		page := in.get(t, path, in.cookieTwo)
		cards, next := galleryCardsAndNext(page.Body.String())
		assert.Len(t, cards, galleryPageSize)
		assert.NotEmpty(t, next)
		assert.NotContains(t, page.Body.String(), private)
		assert.NotContains(t, page.Body.String(), public)
	}
	assert.Contains(t, in.get(t, "/search?q=notfound", in.cookieTwo).Body.String(), "No artifacts match your search.")
	for _, offset := range []string{"-1", "bad"} {
		assert.Equal(t, http.StatusBadRequest, in.get(t, "/partials/gallery-grid/search?offset="+offset, in.cookieTwo).Code)
	}
}

type unavailableSearchStore struct{ store.Store }

func (s unavailableSearchStore) ListViewerGallery(context.Context, store.ViewerID, store.ViewerGalleryOptions) ([]*store.SharedArtifact, error) {
	return nil, errors.New("private search database failure")
}

func TestSearchGalleryRetry(t *testing.T) {
	in := newGrantedInstance(t)
	in.ro.cfg.Store = unavailableSearchStore{in.ro.cfg.Store}
	path := "/partials/gallery-grid/search?offset=24&q=chart"
	page := in.get(t, path, in.cookieTwo)
	require.Equal(t, http.StatusInternalServerError, page.Code)
	_, retry := galleryCardsAndNext(page.Body.String())
	assert.Equal(t, path, retry)
	assert.Empty(t, page.Header().Get("HX-Replace-Url"))
	assert.NotContains(t, page.Body.String(), "private search database failure")
}
