package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGalleryIndexRendersTagPills(t *testing.T) {
	r := newTestRouter(t)

	dark := createTestTag(t, r, "charts", "#FFFFFF")  // light bg -> dark text
	light := createTestTag(t, r, "urgent", "#111111") // dark bg -> light text
	id := createTestArtifact(t, r, "Tagged")

	for _, tag := range []struct{ id string }{{dark.ID}, {light.ID}} {
		w := doJSON(t, r, "POST", "/api/tags/"+tag.id+"/artifacts/"+id, nil)
		require.Equal(t, http.StatusNoContent, w.Code)
	}

	untaggedID := createTestArtifact(t, r, "Untagged")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	// Tagged card: a pill list keyed to the artifact, with per-tag hooks and
	// auto-contrasted text for both a light and a dark background.
	assert.Contains(t, page, `<ul class="tag-pills" data-artifact-id="`+id+`">`)
	assert.Contains(t, page, `data-tag-id="`+dark.ID+`" style="background:#ffffff;color:#111111"`)
	assert.Contains(t, page, `<span class="tag-pill-label">charts</span>`)
	assert.Contains(t, page, `data-tag-id="`+light.ID+`" style="background:#111111;color:#ffffff"`)
	assert.Contains(t, page, `<span class="tag-pill-label">urgent</span>`)

	// Untagged card: no empty pill row at all.
	assert.NotContains(t, page, `data-artifact-id="`+untaggedID+`"`)
}

func TestPillTextColorContrast(t *testing.T) {
	assert.Equal(t, pillTextDark, pillTextColor("#FFFFFF"))
	assert.Equal(t, pillTextLight, pillTextColor("#000000"))
	assert.Equal(t, pillTextLight, pillTextColor("#111111"))
	assert.Equal(t, pillTextDark, pillTextColor("#FAE317")) // brandYellow
}

func TestNormalizeHexColorFallback(t *testing.T) {
	assert.Equal(t, "#ff0000", normalizeHexColor("#f00"))
	assert.Equal(t, "#abcdef", normalizeHexColor("#ABCDEF"))
	assert.Equal(t, "#6b7280", normalizeHexColor("not-a-color"))
	assert.Equal(t, "#6b7280", normalizeHexColor(""))
}
