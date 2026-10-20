package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactDownloadControls(t *testing.T) {
	router := newTestRouter(t)
	id := createTestArtifact(t, router, `Tool "<& title`)
	gallery := getPage(t, router, "/")
	detail := getPage(t, router, "/artifacts/"+id)
	for _, page := range []string{gallery, detail} {
		assert.Equal(t, 1, strings.Count(page, `<dialog class="artifact-download-dialog"`))
		assert.Contains(t, page, `data-controller="artifact-download"`)
		assert.Contains(t, page, `aria-labelledby="artifact-download-title" aria-describedby="artifact-download-description"`)
		assert.Contains(t, page, `name="format" value="zip" checked autofocus`)
		assert.Contains(t, page, `name="format" value="html"`)
		assert.Contains(t, page, `Everything (.zip)`)
		assert.Contains(t, page, `Recommended`)
		assert.Contains(t, page, `Artifact HTML only (.html)`)
		assert.Contains(t, page, `<script type="module" src="/assets/gallery/controllers.js"></script>`)
		assert.NotContains(t, page, `/export" download`)
	}
	fragment := getPage(t, router, "/partials/gallery-grid")
	assert.Contains(t, fragment, `data-action-menu-event-param="exhibit:download"`)
	assert.NotContains(t, fragment, `<dialog`, "card swaps use the stable page dialog")
	assert.NotContains(t, fragment, `<script`)

	// These are different exports and must retain their dedicated controls.
	edit := getPage(t, router, "/artifacts/"+id+"/edit")
	assert.Contains(t, edit, `id="state-export"`)
	assert.Contains(t, edit, `id="assets-export"`)

	a := &store.Artifact{ID: id, OwnerID: defaultOwnerID, Title: "Tool", Tier: store.Tier1, CreatedAt: time.Now()}
	readOnlyDetail, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), pageCredentials{ReadOnly: true}, nil)
	require.NoError(t, err)
	assert.NotContains(t, readOnlyDetail, `artifact-download`)

	// Supply the resolved read-only principal directly: public API reads do
	// not imply anonymous access through the page session gate.
	for _, path := range []string{"/", "/partials/gallery-grid"} {
		req := httptest.NewRequest("GET", path, nil)
		req = req.WithContext(withPrincipal(req.Context(), Principal{OwnerID: defaultOwnerID, Kind: PrincipalPublic, ReadOnly: true}))
		response := httptest.NewRecorder()
		if path == "/" {
			router.galleryIndex(response, req)
		} else {
			router.galleryGridPartial(response, req)
		}
		require.Equal(t, 200, response.Code)
		page := response.Body.String()
		assert.NotContains(t, page, `exhibit:download`)
		assert.NotContains(t, page, `artifact-download`)
	}
}
