package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// putState writes one key through the same route the storage bridge uses.
func putState(t *testing.T, r *Router, artifactID, key, value string) {
	t.Helper()
	w := doJSON(t, r, "PUT", "/api/artifacts/"+artifactID+"/state",
		map[string]string{"key": key, "value": value})
	require.Equal(t, http.StatusNoContent, w.Code)
}

func getState(t *testing.T, r *Router, artifactID string) map[string]string {
	t.Helper()
	w := doJSON(t, r, "GET", "/api/artifacts/"+artifactID+"/state", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var state map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&state))
	return state
}

// DELETE of one key removes the row: the key reads back absent, not as the
// empty string the shim's removeItem currently leaves (av-ms3r). This route is
// the contract av-ms3r and av-st7c consume.
func TestDeleteStateKey(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Stateful")

	putState(t, r, id, "keep", `{"a":1}`)
	putState(t, r, id, "drop", `"gone"`)

	w := doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/state/drop", nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	state := getState(t, r, id)
	_, present := state["drop"]
	assert.False(t, present, "deleted key must be absent, not blank")
	assert.Equal(t, `{"a":1}`, state["keep"])
}

// Deleting a key that was never stored succeeds: the caller asked for the key
// to be gone, and it is. The shim's removeItem fires unconditionally, so this
// has to be a no-op rather than a 404.
func TestDeleteStateKeyIsIdempotent(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Stateful")

	w := doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/state/never-existed", nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

// State keys are arbitrary artifact-chosen text, so the route takes one
// percent-encoded path segment — including slashes, spaces and percent signs,
// which chi hands to the handler still encoded.
func TestDeleteStateKeyWithReservedCharacters(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Stateful")

	for _, key := range []string{"app/settings", "a key with spaces", "100% done", "ünïcode"} {
		putState(t, r, id, key, "1")
		putState(t, r, id, "survivor", "2")

		w := doJSON(t, r, "DELETE",
			"/api/artifacts/"+id+"/state/"+url.PathEscape(key), nil)
		require.Equal(t, http.StatusNoContent, w.Code, key)

		state := getState(t, r, id)
		_, present := state[key]
		assert.False(t, present, "key %q should be gone", key)
		assert.Equal(t, "2", state["survivor"], "unrelated key survives deleting %q", key)
	}
}

// Erase-all drops every state row for the artifact and nothing else: the body,
// the network allowlist, and the capability approvals all survive (AC 8).
func TestClearStateLeavesArtifactIntact(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{
		"title":             "Stateful",
		"body":              `<html><body><script src="https://cdn.example.com/x.js"></script></body></html>`,
		"network_allowlist": []string{"https://cdn.example.com"},
	})
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"downloads_approved": true, "clipboard_approved": true,
	})
	require.Equal(t, http.StatusOK, w.Code)

	putState(t, r, id, "k1", "v1")
	putState(t, r, id, "k2", "v2")

	w = doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/state", nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	assert.Empty(t, getState(t, r, id))

	w = doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, []any{"https://cdn.example.com"}, got["network_allowlist"])
	assert.Equal(t, true, got["downloads_approved"])
	assert.Equal(t, true, got["clipboard_approved"])

	// The body is still served by the render surface's source of truth.
	w = doJSON(t, r, "GET", "/artifacts/"+id+"/edit", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "cdn.example.com/x.js")
}

// Erasing one artifact's state leaves every other artifact's alone.
func TestClearStateIsScopedToOneArtifact(t *testing.T) {
	r := newTestRouter(t)
	mine := createTestArtifact(t, r, "Mine")
	theirs := createTestArtifact(t, r, "Theirs")

	putState(t, r, mine, "k", "mine")
	putState(t, r, theirs, "k", "theirs")

	w := doJSON(t, r, "DELETE", "/api/artifacts/"+mine+"/state", nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	assert.Empty(t, getState(t, r, mine))
	assert.Equal(t, "theirs", getState(t, r, theirs)["k"])
}

// Both DELETEs 404 on an unknown artifact rather than silently succeeding —
// removing rows that don't exist would otherwise look like a write.
func TestDeleteStateUnknownArtifact(t *testing.T) {
	r := newTestRouter(t)

	for _, path := range []string{
		"/api/artifacts/nope/state",
		"/api/artifacts/nope/state/key",
	} {
		w := doJSON(t, r, "DELETE", path, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, path)
	}
}

// Both DELETEs sit inside the authenticated API group, like every other
// mutating route — there is no second write path (architecture.md §4.1).
func TestDeleteStateRequiresAuth(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Stateful")
	putState(t, r, id, "k", "v")

	for _, path := range []string{
		"/api/artifacts/" + id + "/state",
		"/api/artifacts/" + id + "/state/k",
	} {
		req := httptest.NewRequest("DELETE", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, path)
	}

	assert.Equal(t, "v", getState(t, r, id)["k"])
}

// AC 9: the edit page renders the state panel's shell — the collapsible
// section, its three actions, and the add-key controls. The rows themselves
// are fetched on first open (state.js), so the shell is what the server owes.
func TestEditPageRendersStatePanel(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Stateful")

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `<details class="details-panel" id="state-panel">`)
	assert.Contains(t, page, `<div id="state-rows"></div>`)
	assert.Contains(t, page, `id="state-save"`)
	assert.Contains(t, page, `id="state-cancel"`)
	// Erase-all is warning-styled, not a peer of the other two (AC 8).
	assert.Contains(t, page, `class="btn btn-sm btn-danger" id="state-erase"`)
	// Add-key asks for key + type up front (AC 6).
	assert.Contains(t, page, `id="state-add-key"`)
	assert.Contains(t, page, `id="state-add-type"`)
	assert.Contains(t, page, `<script src="/assets/gallery/state.js"></script>`)

	// The erase confirmation names the artifact, so the page bootstrap has to
	// carry the persisted title (not the editable title field).
	assert.Contains(t, page, `const TITLE = "Stateful";`)
}

// The panel's script is embedded and served from the app origin, and holds the
// two properties the panel's design rests on: no raw-text editing of values,
// and no artifact-controlled text interpolated into markup.
func TestStateInspectorAssetServed(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/assets/gallery/state.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "javascript")

	js := w.Body.String()
	// Values are edited through inferred controls: no control anywhere in the
	// panel is a free-text box a JSON blob could be retyped into (AC 2). The
	// read-only fallback is a <pre>, which nothing can type into.
	assert.NotContains(t, js, `'textarea'`)
	assert.NotContains(t, js, "contentEditable")
	assert.Contains(t, js, `createElement('pre')`)
	// Keys and values reach the DOM as text, never as markup (av-tux9).
	assert.NotContains(t, js, "innerHTML")
	assert.Contains(t, js, "textContent")
}
