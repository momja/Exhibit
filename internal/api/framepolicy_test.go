package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayFramePolicyPinsDocumentPaths(t *testing.T) {
	assert.Equal(t, "frame-src https://render.example/a/app-1 https://render.example/w/app-1 https://render.example/a/app-1/versions/", relayFramePolicy("https://render.example", "app-1"))
	assert.Equal(t, "frame-src https://render.example/a/ https://render.example/w/", relayFramePolicy("https://render.example:443/", ""))
	assert.Equal(t, "frame-src http://render.test:8091/a/app-1 http://render.test:8091/w/app-1 http://render.test:8091/a/app-1/versions/", relayFramePolicy("http://render.test:8091", "app-1"))
	assert.Contains(t, relayFramePolicy("https://render.example", "app-1", "https://app.example"), " https://app.example/artifacts/app-1/open")
	assert.NotContains(t, relayFramePolicy("https://render.example", "", "https://app.example"), "https://app.example")
	policy := relayFramePolicy("https://render.example", "id; frame-src https://attacker.example")
	assert.NotContains(t, policy, ";")
	assert.NotContains(t, policy, " https://attacker.example")
	for _, invalid := range []string{"https://name:secret@render.example", "https://render.example/path", "https://render.example?key=secret", "javascript:alert(1)", "https://render.example;frame-src%20*"} {
		assert.Equal(t, "frame-src 'none'", relayFramePolicy(invalid, "app-1"), invalid)
	}
}

func TestRelayPageFramePoliciesAllowOnlyTheirArtifactDocuments(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	id := createArtifact(t, ro, map[string]any{"title": "App", "body": "<h1>App</h1>"})
	for _, test := range []struct {
		path, expected string
	}{
		{"/artifacts/" + id, relayFramePolicy(ro.cfg.RenderOrigin, id, ro.cfg.AppOrigin)},
		{"/agent?artifact=" + id, relayFramePolicy(ro.cfg.RenderOrigin, id)},
		{"/agent", relayFramePolicy(ro.cfg.RenderOrigin, "")},
	} {
		response := doJSON(t, ro, http.MethodGet, test.path, nil)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, test.expected, response.Header().Get("Content-Security-Policy"), test.path)
	}
}
