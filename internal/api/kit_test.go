package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/kit"
)

func TestListKitDescribesVersionsWithoutStylesheets(t *testing.T) {
	r := newTestRouter(t)

	w := do(t, r, "GET", "/api/kit", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got struct {
		Latest   string           `json:"latest"`
		Versions []kitVersionView `json:"versions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, kit.Latest(), got.Latest)
	require.NotEmpty(t, got.Versions)
	assert.Equal(t, got.Latest, got.Versions[0].Version, "newest first")
	assert.Equal(t, `<meta name="exhibit-kit" content="`+got.Latest+`">`, got.Versions[0].Marker)
	require.NotEmpty(t, got.Versions[0].Components)
	for _, c := range got.Versions[0].Components {
		assert.Emptyf(t, c.CSS, "the index carries no CSS (%s)", c.Name)
	}
}

func TestGetKitComponentReturnsItsStylesheet(t *testing.T) {
	r := newTestRouter(t)

	w := do(t, r, "GET", "/api/kit/v1/components/button", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct {
		Version   string        `json:"version"`
		Component kit.Component `json:"component"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "v1", got.Version)
	assert.Contains(t, got.Component.CSS, ".kit-btn")

	assert.Equal(t, http.StatusNotFound, do(t, r, "GET", "/api/kit/v999/components/button", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(t, r, "GET", "/api/kit/v1/components/nope", nil).Code)
}

// The kit is the one thing outside its artifact a session may read, and it may
// only read it.
func TestAgentCredentialMayReadTheKitOnly(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	grant, err := reg.Issue(1, id, "")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, doWithToken(t, r, "GET", "/api/kit", grant.Token(), nil).Code)
	assert.Equal(t, http.StatusOK, doWithToken(t, r, "GET", "/api/kit/v1/components/card", grant.Token(), nil).Code)

	for _, c := range []struct{ method, path string }{
		{"POST", "/api/kit"},
		{"PUT", "/api/kit/v1/components/card"},
		{"GET", "/api/kitchen"},
	} {
		assert.Falsef(t, kitPathReadable(c.method, c.path), "%s %s", c.method, c.path)
	}
}
