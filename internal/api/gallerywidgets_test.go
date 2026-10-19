package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGalleryWidgetReloadMintsAFreshCredential(t *testing.T) {
	ro := newTestRouter(t)
	id := createTestArtifact(t, ro, "Run tracker")
	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, "<p>42 km</p>").Code)
	expired := ro.tokens.MintFor(id, defaultOwnerID, -time.Minute)
	assert.Equal(t, http.StatusNotFound, renderGet(t, ro, "/w/"+id+"?t="+expired).Code)

	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/artifacts/"+id+"/widget/open", nil))
	require.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.True(t, strings.HasPrefix(location, "http://render.test/w/"+id+"?"+rendertoken.Param+"="), location)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, http.StatusOK, renderGet(t, ro, strings.TrimPrefix(location, "http://render.test")).Code)

	gallery := getPage(t, ro, "/")
	assert.Contains(t, gallery, `data-gallery-widget data-widget-reload="/artifacts/`+id+`/widget/open"`)
	assert.NotContains(t, getPage(t, ro, "/partials/card-widget?artifact="+id), "data-gallery-widget",
		"the edit and agent previews keep their existing lifecycle")
}

func TestGalleryWidgetReloadRefusesUnknownArtifacts(t *testing.T) {
	ro := newTestRouter(t)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/artifacts/missing/widget/open", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, w.Header().Get("Location"))
}
