package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedGalleryRecipientCardsAndRevocation(t *testing.T) {
	in := newGrantedInstance(t)
	for _, path := range []string{"/shared", "/partials/gallery-grid/shared"} {
		page := in.get(t, path, in.cookieTwo)
		require.Equal(t, http.StatusOK, page.Code)
		ids, _ := galleryCardsAndNext(page.Body.String())
		assert.Equal(t, []string{in.granted}, ids)
		assert.Contains(t, page.Body.String(), `class="card-by"`)
		assert.Contains(t, page.Body.String(), `one@example.test`)
		for _, forbidden := range []string{`data-controller="action-menu"`, `data-capability-trigger`, `ph-link-simple`, `exhibit:duplicate`, `/edit"`} {
			assert.NotContains(t, page.Body.String(), forbidden)
		}
		creds := renderCredentialsIn(page.Body.String())
		require.Len(t, creds, 1)
		claims, err := in.ro.tokens.Verify(creds[0].token, in.granted)
		require.NoError(t, err)
		assert.Equal(t, in.ownerOne, claims.OwnerID)
		assert.Equal(t, in.ownerTwo, claims.ViewerID)
		doc := renderGet(t, in.ro, creds[0].url)
		require.Equal(t, http.StatusOK, doc.Code)
		assert.Contains(t, doc.Body.String(), recipientState)
		assert.NotContains(t, doc.Body.String(), ownerOneState)
	}
	reloadPath := "/artifacts/" + in.granted + "/widget/viewer/open"
	reload := in.get(t, reloadPath, in.cookieTwo)
	require.Equal(t, http.StatusFound, reload.Code)
	assert.Equal(t, "no-store", reload.Header().Get("Cache-Control"))
	doc := renderGet(t, in.ro, reload.Header().Get("Location"))
	require.Equal(t, http.StatusOK, doc.Code)
	assert.Contains(t, doc.Body.String(), recipientState)
	// The old owner-scoped refresh route still refuses recipients.
	assert.Equal(t, http.StatusNotFound, in.get(t, "/artifacts/"+in.granted+"/widget/open", in.cookieTwo).Code)
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(context.Background(), in.ownerOne, in.granted, in.ownerTwo))
	assert.NotContains(t, in.get(t, "/shared", in.cookieTwo).Body.String(), in.granted)
	assert.Equal(t, http.StatusNotFound, in.get(t, reloadPath, in.cookieTwo).Code)
}

func seedSharedGallery(t *testing.T, in grantedInstance, count int) {
	t.Helper()
	ctx := context.Background()
	for i := range count {
		a := &store.Artifact{ID: fmt.Sprintf("shared-%03d", i), OwnerID: in.ownerOne, Title: fmt.Sprintf("Shared chart %03d", i), SourceBlobID: "fixture", Tier: store.Tier1, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Now()}
		require.NoError(t, in.ro.cfg.Store.PutArtifact(ctx, a))
		_, err := in.ro.cfg.Store.CreateGrant(ctx, in.ownerOne, a.ID, in.ownerTwo)
		require.NoError(t, err)
	}
}

func TestSharedGalleryPagingSearchAndOwnerFilter(t *testing.T) {
	in := newGrantedInstance(t)
	seedSharedGallery(t, in, 49)
	ctx := context.Background()
	third, err := in.ro.cfg.Store.UpsertUser(ctx, "third", "third@example.test")
	require.NoError(t, err)
	other := seedOwnedArtifact(t, in.ro, third.ID, "third-chart", "Shared chart third", "body", "state")
	_, err = in.ro.cfg.Store.CreateGrant(ctx, third.ID, other, in.ownerTwo)
	require.NoError(t, err)
	filter := fmt.Sprintf("owner=%d", in.ownerOne)
	path := "/shared?" + filter + "&q=chart"
	var all []string
	for n := 0; path != ""; n++ {
		require.Less(t, n, 4)
		page := in.get(t, path, in.cookieTwo)
		require.Equal(t, http.StatusOK, page.Code)
		ids, next := galleryCardsAndNext(page.Body.String())
		all = append(all, ids...)
		if next != "" {
			assert.Contains(t, next, filter)
			assert.Contains(t, next, "q=chart")
		}
		path = next
	}
	require.Len(t, all, 49)
	for i, id := range all {
		assert.Equal(t, fmt.Sprintf("shared-%03d", 48-i), id)
	}
	page := in.get(t, "/partials/gallery-grid/shared?"+filter+"&q=chart", in.cookieTwo)
	assert.Equal(t, "/shared?"+filter+"&q=chart", page.Header().Get("HX-Replace-Url"))
	full := in.get(t, "/shared?"+filter+"&q=chart", in.cookieTwo).Body.String()
	assert.Contains(t, normalizePerRender(full), normalizePerRender(page.Body.String()))
	assert.Contains(t, full, fmt.Sprintf(`name="owner" value="%d"`, in.ownerOne))
	both := in.get(t, fmt.Sprintf("/shared?owner=%d&owner=%d&q=third", in.ownerOne, third.ID), in.cookieTwo)
	ids, _ := galleryCardsAndNext(both.Body.String())
	assert.Equal(t, []string{other}, ids)
	canonical := in.get(t, fmt.Sprintf("/partials/gallery-grid/shared?owner=0%d&owner=%d", in.ownerOne, in.ownerOne), in.cookieTwo)
	require.Equal(t, http.StatusOK, canonical.Code)
	assert.Equal(t, "/shared?"+filter, canonical.Header().Get("HX-Replace-Url"))
	for _, raw := range []string{"", "-1", "nonsense", "999999999999999999999", fmt.Sprint(in.ownerTwo), "9999"} {
		for _, prefix := range []string{"/shared", "/partials/gallery-grid/shared"} {
			assert.Equal(t, http.StatusBadRequest, in.get(t, prefix+"?owner="+url.QueryEscape(raw), in.cookieTwo).Code)
		}
	}
	for _, offset := range []string{"-1", "oops"} {
		assert.Equal(t, http.StatusBadRequest, in.get(t, "/partials/gallery-grid/shared?offset="+offset, in.cookieTwo).Code)
	}
	assert.Contains(t, in.get(t, "/shared?q=absent", in.cookieTwo).Body.String(), "No artifacts match your search.")
}

func TestSharedByGallerySelection(t *testing.T) {
	in := newGrantedInstance(t)
	seedSharedGallery(t, in, 49)
	// Grant-only, link-only, private, and another owner's outgoing share.
	linkOnly := seedOwnedArtifact(t, in.ro, in.ownerOne, "link-only", "Shared chart link", "body", "state")
	_, err := in.ro.cfg.Store.CreateAnonymousLink(context.Background(), in.ownerOne, linkOnly)
	require.NoError(t, err)
	private := seedOwnedArtifact(t, in.ro, in.ownerOne, "private", "Private chart", "body", "state")
	_, err = in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerTwo, in.artifactTwo, in.ownerOne)
	require.NoError(t, err)
	path := "/shared-by-me?q=chart"
	var all []string
	for n := 0; path != ""; n++ {
		require.Less(t, n, 4)
		page := in.get(t, path, in.cookieOne)
		require.Equal(t, http.StatusOK, page.Code)
		ids, next := galleryCardsAndNext(page.Body.String())
		all = append(all, ids...)
		if next != "" {
			assert.True(t, strings.HasPrefix(next, "/partials/gallery-grid/shared-by-me?"))
			assert.Contains(t, next, "q=chart")
		}
		assert.Contains(t, page.Body.String(), `data-controller="action-menu"`)
		assert.NotContains(t, page.Body.String(), `class="card-by"`)
		path = next
	}
	assert.Len(t, all, 50)
	assert.Contains(t, all, linkOnly)
	assert.NotContains(t, all, private)
	assert.NotContains(t, all, in.artifactTwo)
	// Own cards execute the same partial; only Edit's return destination differs.
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(withPrincipal(req.Context(), Principal{OwnerID: in.ownerOne, Kind: PrincipalSession}))
	owned, err := in.ro.loadGalleryGrid(req, libraryGallery, 0)
	require.NoError(t, err)
	shared, err := in.ro.loadGalleryGrid(req, sharedByGallery, 0)
	require.NoError(t, err)
	first, err := renderPage("galleryCard", shared.Cards[0])
	require.NoError(t, err)
	assert.Contains(t, first, "return_to=%2Fshared-by-me")
	for _, card := range owned.Cards {
		if card.ArtifactID == shared.Cards[0].ArtifactID {
			same, err := renderPage("galleryCard", card)
			require.NoError(t, err)
			assert.Equal(t, normalizePerRender(same), strings.ReplaceAll(normalizePerRender(first), "return_to=%2Fshared-by-me", "return_to=%2F"))
			return
		}
	}
	t.Fatal("shared card absent from library")
}

func TestSharedGalleryRoutesResolveSingleUserOwner(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()
	one, err := ro.cfg.Store.UpsertUser(ctx, "single-owner", "single@example.test")
	require.NoError(t, err)
	require.Equal(t, defaultOwnerID, one.ID)
	two, err := ro.cfg.Store.UpsertUser(ctx, "other-owner", "other@example.test")
	require.NoError(t, err)
	owned := seedOwnedArtifact(t, ro, one.ID, "own-share", "Owned share", "body", "state")
	incoming := seedOwnedArtifact(t, ro, two.ID, "incoming-share", "Incoming share", "body", "state")
	_, err = ro.cfg.Store.CreateGrant(ctx, one.ID, owned, two.ID)
	require.NoError(t, err)
	_, err = ro.cfg.Store.CreateGrant(ctx, two.ID, incoming, one.ID)
	require.NoError(t, err)
	for _, space := range []struct{ path, id string }{{"shared", incoming}, {"shared-by-me", owned}} {
		for _, prefix := range []string{"/", "/partials/gallery-grid/"} {
			body := getPage(t, ro, prefix+space.path)
			ids, _ := galleryCardsAndNext(body)
			assert.Equal(t, []string{space.id}, ids)
		}
	}
}

func TestSharedGalleryEmptyStates(t *testing.T) {
	ro := newTestRouter(t)
	for _, space := range []struct{ path, message string }{{"shared", "Nothing has been shared with you yet."}, {"shared-by-me", "Share an artifact from its share panel."}} {
		full := getPage(t, ro, "/"+space.path)
		fragment := getPage(t, ro, "/partials/gallery-grid/"+space.path)
		assert.Contains(t, full, space.message)
		assert.Contains(t, fragment, space.message)
		assert.NotContains(t, fragment, `href="/new"`)
	}
}

type unavailableSharedGalleryStore struct {
	store.Store
	ownersFail bool
}

func (s unavailableSharedGalleryStore) ListArtifactsSharedWith(context.Context, store.ViewerID, store.SharedListOptions) ([]*store.SharedArtifact, error) {
	return nil, errors.New("private database error")
}
func (s unavailableSharedGalleryStore) SharedWithOwners(ctx context.Context, viewer store.ViewerID) ([]store.SharedOwner, error) {
	if s.ownersFail {
		return nil, errors.New("private owner error")
	}
	return s.Store.SharedWithOwners(ctx, viewer)
}
func TestSharedGalleryRetryPreservesSelection(t *testing.T) {
	for _, ownersFail := range []bool{false, true} {
		in := newGrantedInstance(t)
		in.ro.cfg.Store = unavailableSharedGalleryStore{in.ro.cfg.Store, ownersFail}
		path := fmt.Sprintf("/partials/gallery-grid/shared?offset=24&owner=%d&q=chart", in.ownerOne)
		page := in.get(t, path, in.cookieTwo)
		require.Equal(t, http.StatusInternalServerError, page.Code)
		_, retry := galleryCardsAndNext(page.Body.String())
		assert.Equal(t, path, retry)
		assert.Empty(t, page.Header().Get("HX-Replace-Url"))
		assert.NotContains(t, page.Body.String(), "private")
	}
}
