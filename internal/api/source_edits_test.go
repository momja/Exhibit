package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceEditsPreserveUntouchedBytes(t *testing.T) {
	source := "\ufeff<h1>🌸</h1>\r\n<p>old</p>\r\n"
	updated, err := applySourceEdits(source, []sourceEdit{
		{OldText: "old", NewText: "new 🌼"},
		{OldText: "🌸", NewText: "🌻"},
	})
	require.NoError(t, err)
	assert.Equal(t, "\ufeff<h1>🌻</h1>\r\n<p>new 🌼</p>\r\n", updated)
}

func TestSourcePatchUsesTheReadRevisionAndNormalSavePath(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "Patch", "body": "<h1>old</h1><p>keep 🌼</p>"})
	_, revision := sourceSnapshot(t, r, id)
	before := versionCount(t, r, id)
	patch := map[string]any{"expected_source_revision": revision,
		"edits": []map[string]string{{"oldText": "old", "newText": `<img src="https://cdn.example.com/new.png">`}}}
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, patch)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, getArtifactBody(t, r, id), "<p>keep 🌼</p>")
	assert.Contains(t, w.Body.String(), "https://cdn.example.com")
	_, savedRevision := sourceSnapshot(t, r, id)
	assert.NotEqual(t, revision, savedRevision)
	assert.Equal(t, before+1, versionCount(t, r, id))
	// Retrying a patch based on the old read does not silently edit newer source.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, patch)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, before+1, versionCount(t, r, id))
}

func TestWidgetPatchReturnsConfirmationWithoutEchoingTheSource(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Tile")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>old</b><p>untouched</p>").Code)
	revision := widgetSnapshot(t, r, id).WidgetRevision
	w := doJSON(t, r, "PUT", "/api/artifacts/"+id+"/widget", map[string]any{
		"expected_widget_revision": revision,
		"edits":                    []map[string]string{{"oldText": "old", "newText": "new"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var confirmation map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &confirmation))
	assert.NotContains(t, confirmation, "body")
	assert.NotEmpty(t, confirmation["widget_revision"])
	w = doJSON(t, r, "GET", "/api/artifacts/"+id+"/widget", nil)
	assert.Contains(t, w.Body.String(), "new")
	assert.Contains(t, w.Body.String(), "untouched")
}

func TestMalformedOrUnconditionalPatchSavesNothing(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "Patch", "body": "<h1>old</h1>"})
	_, revision := sourceSnapshot(t, r, id)
	before := versionCount(t, r, id)
	for _, patch := range []map[string]any{
		{"edits": []map[string]string{{"oldText": "old", "newText": "new"}}},
		{"expected_source_revision": revision, "edits": []map[string]string{{"oldText": "missing", "newText": "new"}}},
		{"expected_source_revision": revision, "body": "<p>mixed</p>", "edits": []map[string]string{{"oldText": "old", "newText": "new"}}},
	} {
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, patch)
		assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusPreconditionRequired, w.Body.String())
		assert.Equal(t, "<h1>old</h1>", getArtifactBody(t, r, id))
		assert.Equal(t, before, versionCount(t, r, id))
	}
}

func TestSourceEditsRejectAmbiguousAndIntersectingReplacements(t *testing.T) {
	for name, edits := range map[string][]sourceEdit{
		"empty":       {},
		"empty match": {{OldText: "", NewText: "x"}},
		"missing":     {{OldText: "z", NewText: "x"}},
		"ambiguous":   {{OldText: "aa", NewText: "x"}},
		"overlap":     {{OldText: "aab", NewText: "x"}, {OldText: "bc", NewText: "y"}},
		"adjacent":    {{OldText: "aaa", NewText: "x"}, {OldText: "bc", NewText: "y"}},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := applySourceEdits("aaabc", edits)
			require.Error(t, err)
			assert.Empty(t, result)
		})
	}
}

func TestDecodeSourceEditsRequiresExplicitStringReplacements(t *testing.T) {
	for _, value := range []any{nil, "not an array", []any{}, []any{nil},
		[]any{map[string]any{"oldText": "old"}},
		[]any{map[string]any{"oldText": "old", "newText": nil}},
		[]any{map[string]any{"oldText": "old", "newText": 2}},
	} {
		_, err := decodeSourceEdits(value)
		require.Error(t, err, "%#v", value)
	}
	edits, err := decodeSourceEdits([]any{map[string]any{"oldText": "old", "newText": ""}})
	require.NoError(t, err)
	assert.Equal(t, []sourceEdit{{OldText: "old", NewText: ""}}, edits)
}
