package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The resource-server half of MCP authorization (Exh-354p).

func TestMCPUnauthorizedPointsAtResourceMetadata(t *testing.T) {
	r := newTestRouter(t)
	w := postInitialize(t, r, nil, nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, `Bearer resource_metadata="http://app.test/.well-known/oauth-protected-resource/mcp"`,
		w.Header().Get("WWW-Authenticate"))

	w = postInitialize(t, r, bearer("exgrant_revoked"), nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.NotEmpty(t, w.Header().Get("WWW-Authenticate"), "an unknown token is told where to get another")
}

func TestProtectedResourceMetadataNamesTheMCPEndpoint(t *testing.T) {
	r := newTestRouter(t)
	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, path)
		var meta map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &meta))
		assert.Equal(t, "http://app.test/mcp", meta["resource"], path)
		assert.Equal(t, []any{"header"}, meta["bearer_methods_supported"])
	}
}
