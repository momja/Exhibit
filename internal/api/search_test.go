package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listArtifactsQuery GETs /api/artifacts?q=... through the router and
// returns the decoded artifact list.
func listArtifactsQuery(t *testing.T, r *Router, q string) []map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/artifacts?q="+q, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got []map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	return got
}

// TestSearchMatchesSourceEndToEnd covers av-b6o9 through the actual HTTP
// ingest path (not just the store layer): a term that only appears in the
// artifact's body, never in its title, must still surface in gallery search.
func TestSearchMatchesSourceEndToEnd(t *testing.T) {
	r := newTestRouter(t)

	createArtifact(t, r, map[string]any{
		"title":             "Untitled Tool",
		"body":              "<script>const uniqueSearchToken42 = 1;</script>",
		"network_allowlist": []string{},
	})
	createArtifact(t, r, map[string]any{
		"title":             "Other Tool",
		"body":              "<script>nothing interesting</script>",
		"network_allowlist": []string{},
	})

	found := listArtifactsQuery(t, r, "uniqueSearchToken42")
	require.Len(t, found, 1)
	assert.Equal(t, "Untitled Tool", found[0]["title"])
}

// TestArtifactJSONNeverLeaksSourceText guards the write-only contract on
// store.Artifact.SourceText (json:"-"): the field exists purely to seed the
// FTS index and must never appear in an API response, which would otherwise
// duplicate the full body into every list/detail payload.
func TestArtifactJSONNeverLeaksSourceText(t *testing.T) {
	r := newTestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Leak Check",
		"body":              "<script>secretSourceMarker</script>",
		"network_allowlist": []string{},
	})

	req := httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "secretSourceMarker")
}
