package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPProxyRoutesOnlyAppHostTransportAndDiscovery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer caller", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		require.Empty(t, r.Header.Get("X-Forwarded-Host"))
		require.Equal(t, "exhibit.dizzard.net", r.Host)
		fmt.Fprint(w, "MCP "+r.URL.RequestURI())
	}))
	defer upstream.Close()
	marker := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, name) })
	}
	app, err := NewMCPProxy(marker("CORE"), "https://exhibit.dizzard.net", "https://exhibit.dizzard.net/mcp", upstream.URL)
	require.NoError(t, err)
	h, err := NewHostDispatcher(app, marker("RENDER"), "https://exhibit.dizzard.net", "https://render.test")
	require.NoError(t, err)
	for _, tc := range []struct{ host, path, want string }{
		{"exhibit.dizzard.net", "/mcp?session=x", "MCP /mcp?session=x"},
		{"exhibit.dizzard.net", "/.well-known/oauth-protected-resource", "MCP /.well-known/oauth-protected-resource"},
		{"exhibit.dizzard.net", "/.well-known/oauth-protected-resource/mcp", "MCP /.well-known/oauth-protected-resource/mcp"},
		{"exhibit.dizzard.net", "/oauth/token", "CORE"},
		{"exhibit.dizzard.net", "/.well-known/oauth-authorization-server", "CORE"},
		{"exhibit.dizzard.net", "/mcp/extra", "CORE"},
		{"old.fly.dev", "/mcp", "CORE"},
		{"render.test", "/mcp", "RENDER"},
		{"render.test", "/.well-known/oauth-protected-resource/mcp", "RENDER"},
	} {
		t.Run(tc.host+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tc.path, nil)
			r.Host = tc.host
			r.Header.Set("Authorization", "Bearer caller")
			r.Header.Set("Cookie", "session=private")
			r.Header.Set("X-Forwarded-Host", "forged.test")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			require.Equal(t, tc.want, w.Body.String())
		})
	}
}

func TestMCPProxyFailsClosedForInvalidConfiguration(t *testing.T) {
	for _, target := range []string{"https://remote.test", "http://localhost:8090", "http://127.0.0.1:8090/mcp", "http://127.0.0.1:8090?x=y", "http://u:p@127.0.0.1:8090", "http://127.0.0.1"} {
		_, err := NewMCPProxy(http.NotFoundHandler(), "https://app.test", "https://app.test/mcp", target)
		require.Error(t, err, target)
	}
	_, err := NewMCPProxy(http.NotFoundHandler(), "https://app.test", "https://old.fly.dev/mcp", "http://127.0.0.1:8090")
	require.Error(t, err)
}

func TestMCPProxyUnavailableSatellite(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	upstream.Close()
	h, err := NewMCPProxy(http.NotFoundHandler(), "https://app.test", "https://app.test/mcp", upstream.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, dispatchTo(h, http.MethodPost, "app.test", "/mcp").Code)
}
