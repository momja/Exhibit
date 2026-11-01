package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGallerySpacesCountsAndActivePage(t *testing.T) {
	in := newGrantedInstance(t)
	// Each side sees only its own incoming/outgoing counts, regardless of search.
	for _, viewer := range []struct {
		cookie *http.Cookie
		with   string
		by     string
	}{{in.cookieOne, "0", "1"}, {in.cookieTwo, "1", "0"}} {
		for _, path := range []string{"/", "/shared", "/shared-by-me"} {
			body := in.get(t, path+"?q=absent", viewer.cookie).Body.String()
			assert.Equal(t, 1, strings.Count(body, `aria-label="Library spaces"`))
			assert.Contains(t, body, `href="`+path+`" aria-current="page"`)
			assert.Equal(t, 1, strings.Count(body, `aria-current="page"`))
			counts := regexp.MustCompile(`(?s)href="(/shared(?:-by-me)?)"[^>]*>.*?class="space-count">([0-9]+)</span>`).FindAllStringSubmatch(body, -1)
			require.Len(t, counts, 2)
			assert.Equal(t, []string{"/shared", viewer.with}, counts[0][1:])
			assert.Equal(t, []string{"/shared-by-me", viewer.by}, counts[1][1:])
			assert.Contains(t, body, `href="/shared"`)
			assert.Contains(t, body, `href="/shared-by-me"`)
		}
	}
	// Fragments own grid markup only, so paging/search cannot duplicate the nav.
	assert.NotContains(t, in.get(t, "/partials/gallery-grid/shared", in.cookieTwo).Body.String(), `Library spaces`)
}

func TestGallerySpacesStayAvailableWithoutShares(t *testing.T) {
	in := newTwoOwnerInstance(t)
	before := in.get(t, "/", in.cookieTwo).Body.String()
	assert.Contains(t, before, `aria-label="Library spaces"`)
	assert.Contains(t, before, `<span class="space-label">Library</span>`)
	assert.Contains(t, before, `<span class="space-label">Shared with me</span>`)
	assert.Contains(t, before, `<span class="space-label">Shared by me</span>`)
	assert.Equal(t, 2, strings.Count(before, `class="space-count">0</span>`))
	// Other people's shares must not affect this viewer's navigation or counts.
	third, err := in.ro.cfg.Store.UpsertUser(context.Background(), "third", "third@example.test")
	require.NoError(t, err)
	_, err = in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerOne, in.artifactOne, third.ID)
	require.NoError(t, err)
	assert.Equal(t, normalizePerRender(before), normalizePerRender(in.get(t, "/", in.cookieTwo).Body.String()))
	// Receiving and losing the last share changes counts, never destinations.
	_, err = in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerOne, in.artifactOne, in.ownerTwo)
	require.NoError(t, err)
	incoming := in.get(t, "/shared", in.cookieTwo).Body.String()
	assert.Contains(t, incoming, `href="/shared" aria-current="page"`)
	assert.Contains(t, incoming, `class="space-count">1</span>`)
	assert.Contains(t, incoming, `data-gallery-read-only="true"`)
	assert.NotContains(t, incoming, `data-duplicate-dialog`)
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(context.Background(), in.ownerOne, in.artifactOne, in.ownerTwo))
	assert.Equal(t, normalizePerRender(before), normalizePerRender(in.get(t, "/", in.cookieTwo).Body.String()))
}

func TestGalleryPublicLinkKeepsSpacesAvailable(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryArtifacts(t, ro, 1)
	var linkID string
	for _, shared := range []bool{false, true, false} {
		if shared {
			link, err := ro.cfg.Store.CreateAnonymousLink(context.Background(), defaultOwnerID, "gallery-000")
			require.NoError(t, err)
			linkID = link.ID
		} else if linkID != "" {
			require.NoError(t, ro.cfg.Store.DeleteShare(context.Background(), defaultOwnerID, linkID))
		}
		body := getPage(t, ro, "/shared-by-me")
		assert.Contains(t, body, `href="/shared-by-me" aria-current="page"`)
		if shared {
			assert.Contains(t, body, `class="space-count">1</span>`)
		} else {
			assert.Equal(t, 2, strings.Count(body, `class="space-count">0</span>`))
			assert.Contains(t, body, "Share an artifact from its share panel.")
		}
	}
}

func TestGallerySpacesRequireSession(t *testing.T) {
	in := newTwoOwnerInstance(t)
	// Public mode does not make personal gallery routes anonymous.
	in.ro.cfg.Public = PublicMode{Enabled: true, OwnerID: in.ownerOne}
	for _, path := range []string{"/", "/shared", "/shared-by-me"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "/auth/login")
		assert.NotContains(t, w.Body.String(), "Library spaces")
	}
}

type unavailableSpaceCountsStore struct{ store.Store }

func (s unavailableSpaceCountsStore) SpaceCounts(context.Context, store.ViewerID) (int, int, error) {
	return 0, 0, errors.New("private counts error")
}

func TestGallerySpaceCountsFailure(t *testing.T) {
	in := newGrantedInstance(t)
	in.ro.cfg.Store = unavailableSpaceCountsStore{in.ro.cfg.Store}
	for _, path := range []string{"/", "/shared", "/shared-by-me"} {
		page := in.get(t, path, in.cookieTwo)
		assert.Equal(t, http.StatusInternalServerError, page.Code)
		assert.NotContains(t, page.Body.String(), `Library spaces`)
	}
}
