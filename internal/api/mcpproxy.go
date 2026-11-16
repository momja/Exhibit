package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// NewMCPProxy forwards only the app host's MCP transport and resource discovery.
// The satellite still owns bearer verification; OAuth and all other routes stay
// on the core. Put this inside the host dispatcher so render always wins.
func NewMCPProxy(app http.Handler, appOrigin, publicURL, targetURL string) (http.Handler, error) {
	if publicURL != appOrigin+"/mcp" {
		return nil, fmt.Errorf("MCP_PROXY_URL requires MCP_URL to equal APP_ORIGIN + /mcp")
	}
	host, err := originHost(appOrigin)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(targetURL)
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.Port() == "" || target.Path != "" || target.RawQuery != "" || target.Fragment != "" || target.User != nil {
		return nil, fmt.Errorf("MCP_PROXY_URL must be http://127.0.0.1:<port> without a path")
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = r.In.Host
			// MCP is bearer-only. Never pass the app's browser session to a satellite.
			r.Out.Header.Del("Cookie")
		},
		FlushInterval: -time.Millisecond,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestHost(r) == host {
			switch r.URL.Path {
			case "/mcp", "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp":
				proxy.ServeHTTP(w, r)
				return
			}
		}
		app.ServeHTTP(w, r)
	}), nil
}
