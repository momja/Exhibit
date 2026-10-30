package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/kit"
)

func TestListKitDescribesKitsAndContractsWithoutStylesheets(t *testing.T) {
	r := newTestRouter(t)

	w := do(t, r, "GET", "/api/kit", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got struct {
		Default   string         `json:"default"`
		Kits      []kitView      `json:"kits"`
		Contracts []kit.Contract `json:"contracts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, kit.Default(), got.Default)
	require.Len(t, got.Kits, len(kit.IDs()))
	for _, k := range got.Kits {
		assert.Equal(t, `<meta name="exhibit-kit" content="`+k.ID+`">`, k.Marker)
		assert.Equal(t, "v1", k.Contract)
	}
	require.Len(t, got.Contracts, 1, "listed once, however many kits implement it")
	require.NotEmpty(t, got.Contracts[0].Components)
	for _, c := range got.Contracts[0].Components {
		assert.Emptyf(t, c.CSS, "the index carries no CSS (%s)", c.Name)
	}
}

func TestGetKitComponentReturnsThatKitsStylesheet(t *testing.T) {
	r := newTestRouter(t)

	read := func(path string) kit.Component {
		w := do(t, r, "GET", path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got struct {
			Kit       string        `json:"kit"`
			Component kit.Component `json:"component"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got.Component
	}
	exhibit := read("/api/kit/exhibit/v1/components/button")
	paper := read("/api/kit/paper/v1/components/button")
	assert.Contains(t, exhibit.CSS, ".kit-btn")
	assert.Equal(t, exhibit.Classes, paper.Classes, "one contract")
	assert.NotEqual(t, exhibit.CSS, paper.CSS, "two looks")

	assert.Equal(t, http.StatusNotFound, do(t, r, "GET", "/api/kit/exhibit/v999/components/button", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(t, r, "GET", "/api/kit/nope/v1/components/button", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(t, r, "GET", "/api/kit/exhibit/v1/components/nope", nil).Code)
}

// The kit is the one thing outside its artifact a session may read, and it may
// only read it.
func TestAgentCredentialMayReadTheKitOnly(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	grant, err := reg.Issue(1, id, "")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, doWithToken(t, r, "GET", "/api/kit", grant.Token(), nil).Code)
	assert.Equal(t, http.StatusOK, doWithToken(t, r, "GET", "/api/kit/exhibit/v1/components/card", grant.Token(), nil).Code)

	for _, c := range []struct{ method, path string }{
		{"POST", "/api/kit"},
		{"PUT", "/api/kit/exhibit/v1/components/card"},
		{"GET", "/api/kitchen"},
	} {
		assert.Falsef(t, kitPathReadable(c.method, c.path), "%s %s", c.method, c.path)
	}
}
