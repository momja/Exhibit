package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditorReturnDestinations(t *testing.T) {
	a := &store.Artifact{ID: "tool", Title: "My tool"}
	for _, tc := range []struct{ target, path, label string }{
		{"/", "/", "Starred"},
		{"/search?q=chart&tag=Finance&visible=48", "/search?q=chart&tag=Finance&visible=48", "Search"},
		{"/search?q=chart", "/search?q=chart", "Search"},
		{"/shared-by-me?q=chart", "/shared-by-me?q=chart", "Shared"},
		{"/artifacts/tool", "/artifacts/tool", "My tool"},
		{"https://elsewhere.test/", "/search", "Search"},
		{"//elsewhere.test/", "/search", "Search"},
		{"javascript:alert(1)", "/search", "Search"},
		{"/artifacts/another", "/search", "Search"},
		{"/auth/logout", "/search", "Search"},
		{"/%2felsewhere.test", "/search", "Search"},
		{"/artifacts/tool/../another", "/search", "Search"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/artifacts/tool/edit?"+url.Values{"return_to": {tc.target}}.Encode(), nil)
			assert.Equal(t, editNavigation{URL: tc.path, Label: tc.label}, editorReturn(r, a))
		})
	}
	r := httptest.NewRequest(http.MethodGet, "http://app.test/artifacts/tool/edit", nil)
	r.Header.Set("Referer", "http://app.test/artifacts/tool")
	assert.Equal(t, editNavigation{URL: "/artifacts/tool", Label: "My tool"}, editorReturn(r, a))
	r.Header.Set("Referer", "https://elsewhere.test/artifacts/tool")
	assert.Equal(t, editNavigation{URL: "/search", Label: "Search"}, editorReturn(r, a))
}

func TestGalleryReturnRendersLoadedResults(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryArtifacts(t, ro, 60)
	page := getPage(t, ro, "/search?q=Artifact&visible=48")
	ids, next := galleryCardsAndNext(page)
	require.Len(t, ids, 48)
	assert.Contains(t, next, "offset=48")
	assert.Contains(t, page, "return_to=%2Fsearch%3Fq%3DArtifact")
	fragment := getPage(t, ro, "/partials/gallery-grid/search?q=Artifact&visible=480")
	ids, _ = galleryCardsAndNext(fragment)
	assert.Len(t, ids, galleryPageSize, "ordinary fragment paging remains bounded")
}

func TestEditorNavigationMarkupAndVersionRefresh(t *testing.T) {
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"title": "My tool", "body": "<h1>Tool</h1>"})
	page := getPage(t, ro, editURL(id, "/search?q=tool"))
	assert.Contains(t, page, `class="app-header-back" href="/search?q=tool"`)
	assert.Contains(t, page, `data-controller="unsaved-changes"`)
	assert.Contains(t, page, `type="module" src="`+assetURL("gallery/edit.js")+`"`)
	assert.NotContains(t, page, `onclick="save()"`)
	assert.Contains(t, page, "Save and leave")
	assert.Contains(t, page, "View artifact")
	assert.Contains(t, getPage(t, ro, "/partials/edit-versions?artifact="+id), "1 saved version")
	detail := getPage(t, ro, "/artifacts/"+id)
	assert.Contains(t, detail, `aria-label="Starred" title="Starred"><i class="ph ph-star"`)
	assert.NotContains(t, detail, "Back to gallery")
	assert.Equal(t, 1, strings.Count(detail, `class="ph ph-star"`))
	in := newGrantedInstance(t)
	assert.Equal(t, http.StatusNotFound, in.get(t, "/partials/edit-versions?artifact="+in.granted, in.cookieTwo).Code)
}
