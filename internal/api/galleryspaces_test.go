package api

import (
	"context"
	"errors"
	"net/http"
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

func TestGalleryWithoutSharesKeepsOriginalPage(t *testing.T) {
	in := newTwoOwnerInstance(t)
	before := in.get(t, "/", in.cookieTwo).Body.String()
	assert.NotContains(t, before, `gallery-layout`)
	assert.NotContains(t, before, `gallery-rail`)
	assert.NotContains(t, before, `Library spaces`)
	assert.Contains(t, before, "</header>\n<main>")
	// Other people's shares must not change this viewer's original page at all.
	third, err := in.ro.cfg.Store.UpsertUser(context.Background(), "third", "third@example.test")
	require.NoError(t, err)
	_, err = in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerOne, in.artifactOne, third.ID)
	require.NoError(t, err)
	after := in.get(t, "/", in.cookieTwo).Body.String()
	assert.Equal(t, normalizePerRender(before), normalizePerRender(after))
	// No private cards are needed to qualify: an incoming share alone is enough.
	_, err = in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerOne, in.artifactOne, in.ownerTwo)
	require.NoError(t, err)
	assert.Contains(t, in.get(t, "/", in.cookieTwo).Body.String(), `Library spaces`)
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(context.Background(), in.ownerOne, in.artifactOne, in.ownerTwo))
	assert.Equal(t, normalizePerRender(before), normalizePerRender(in.get(t, "/", in.cookieTwo).Body.String()))
}

func TestGalleryPublicLinkEnablesSpaces(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryArtifacts(t, ro, 1)
	assert.NotContains(t, getPage(t, ro, "/"), `Library spaces`)
	_, err := ro.cfg.Store.CreateAnonymousLink(context.Background(), defaultOwnerID, "gallery-000")
	require.NoError(t, err)
	assert.Contains(t, getPage(t, ro, "/"), `Library spaces`)
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
