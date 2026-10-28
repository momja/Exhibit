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
		{"/", "/", "My library"},
		{"/?q=chart&tag=Finance&visible=48", "/?q=chart&tag=Finance&visible=48", "My library"},
		{"/shared-by-me?q=chart", "/shared-by-me?q=chart", "Shared by me"},
		{"/artifacts/tool", "/artifacts/tool", "My tool"},
		{"https://elsewhere.test/", "/", "My library"},
		{"//elsewhere.test/", "/", "My library"},
		{"javascript:alert(1)", "/", "My library"},
		{"/artifacts/another", "/", "My library"},
		{"/auth/logout", "/", "My library"},
		{"/%2felsewhere.test", "/", "My library"},
		{"/artifacts/tool/../another", "/", "My library"},
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
	assert.Equal(t, editNavigation{URL: "/", Label: "My library"}, editorReturn(r, a))
}

func TestGalleryReturnRendersLoadedResults(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryArtifacts(t, ro, 60)
	page := getPage(t, ro, "/?q=Artifact&visible=48")
	ids, next := galleryCardsAndNext(page)
	require.Len(t, ids, 48)
	assert.Contains(t, next, "offset=48")
	assert.Contains(t, page, "return_to=%2F%3Fq%3DArtifact")
	fragment := getPage(t, ro, "/partials/gallery-grid?q=Artifact&visible=480")
	ids, _ = galleryCardsAndNext(fragment)
	assert.Len(t, ids, galleryPageSize, "ordinary fragment paging remains bounded")
}

func TestEditorNavigationMarkupAndVersionRefresh(t *testing.T) {
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"title": "My tool", "body": "<h1>Tool</h1>"})
	page := getPage(t, ro, editURL(id, "/?q=tool"))
	assert.Contains(t, page, `class="editor-return" href="/?q=tool"`)
	assert.Contains(t, page, `data-controller="unsaved-changes"`)
	assert.Contains(t, page, `type="module" src="`+assetURL("gallery/edit.js")+`"`)
	assert.NotContains(t, page, `onclick="save()"`)
	assert.Contains(t, page, "Save and leave")
	assert.Contains(t, page, "View artifact")
	assert.Contains(t, getPage(t, ro, "/partials/edit-versions?artifact="+id), "1 saved version")
	detail := getPage(t, ro, "/artifacts/"+id)
	assert.Contains(t, detail, `aria-label="My library" title="My library"><i class="ph ph-squares-four"`)
	assert.NotContains(t, detail, "Back to gallery")
	assert.Equal(t, 1, strings.Count(detail, `class="ph ph-squares-four"`))
	in := newGrantedInstance(t)
	assert.Equal(t, http.StatusNotFound, in.get(t, "/partials/edit-versions?artifact="+in.granted, in.cookieTwo).Code)
}
