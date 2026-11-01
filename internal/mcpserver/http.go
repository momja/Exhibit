package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const DefaultMaxRequestBodyBytes int64 = 32 << 20
const DefaultPublicURL = "http://localhost:8090/mcp"

// HTTPConfig separates the core API address from the service's public address.
// The service has no credential of its own; each session carries its caller's.
type HTTPConfig struct {
	BaseURL             string
	PublicURL           string
	Version             string
	MaxRequestBodyBytes int64
}

// NewHTTPHandler serves Streamable HTTP at PublicURL's path and GET /healthz.
// Authentication is checked against the core on every request, including session
// continuations. Tools then use the same bearer over the ordinary HTTP API.
func NewHTTPHandler(cfg HTTPConfig) (http.Handler, error) {
	if _, err := serviceURL(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("EXHIBIT_URL: %w", err)
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = DefaultPublicURL
	}
	public, err := serviceURL(cfg.PublicURL)
	if err != nil {
		return nil, fmt.Errorf("MCP_PUBLIC_URL: %w", err)
	}
	endpoint := public.Path
	if endpoint == "" {
		endpoint = "/mcp"
	}
	if endpoint == "/healthz" {
		return nil, fmt.Errorf("MCP_PUBLIC_URL: /healthz is reserved")
	}
	publicOrigin := canonicalOrigin(public)
	if cfg.MaxRequestBodyBytes == 0 {
		cfg.MaxRequestBodyBytes = DefaultMaxRequestBodyBytes
	}
	if cfg.MaxRequestBodyBytes < 0 {
		return nil, fmt.Errorf("MAX_REQUEST_BODY_BYTES must be positive")
	}

	// Never follow redirects with a caller's credential, and never keep a cookie
	// jar. Only the configured core receives credentials or answers verification.
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	verifierClient := &http.Client{Timeout: 10 * time.Second, CheckRedirect: noRedirect}
	toolClient := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: noRedirect}
	stream := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return New(&HTTPAPI{BaseURL: cfg.BaseURL, Token: requestBearer(r), Client: toolClient}, cfg.Version)
	}, &mcp.StreamableHTTPOptions{
		SessionTimeout:      30 * time.Minute,
		MaxRequestBodyBytes: cfg.MaxRequestBodyBytes,
		// A local reverse proxy carries the public Host. Origin plus bearer-only
		// auth supplies the browser boundary without rejecting that deployment.
		DisableLocalhostProtection: true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok\n"))
			return
		}
		if r.URL.Path != endpoint {
			http.NotFound(w, r)
			return
		}
		if origins := r.Header.Values("Origin"); len(origins) > 0 && (len(origins) != 1 || origins[0] != publicOrigin) {
			http.Error(w, "cross-origin requests are not accepted", http.StatusForbidden)
			return
		}
		token := requestBearer(r)
		if token == "" {
			http.Error(w, "a bearer token is required", http.StatusUnauthorized)
			return
		}
		credential, status := verifyLibraryCredential(r.Context(), &HTTPAPI{BaseURL: cfg.BaseURL, Token: token, Client: verifierClient})
		if status != 0 {
			message := "invalid or non-library agent credential"
			if status == http.StatusServiceUnavailable {
				message = "Exhibit credential verification is unavailable"
			}
			http.Error(w, message, status)
			return
		}
		if r.ContentLength > cfg.MaxRequestBodyBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		// The SDK owns session binding. Verification above is per-request, not a
		// cache; separating it lets core outages return 503 instead of the SDK's
		// generic verifier-error 500. No request can reach the stream without it.
		auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
			return &auth.TokenInfo{UserID: "grant:" + credential}, nil
		}, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(stream).ServeHTTP(w, r)
	}), nil
}

func requestBearer(r *http.Request) string {
	if len(r.Header.Values("Authorization")) != 1 {
		return ""
	}
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return ""
	}
	return fields[1]
}

func verifyLibraryCredential(ctx context.Context, api *HTTPAPI) (string, int) {
	response, err := api.Do(ctx, http.MethodGet, "/api/agent/credential", nil)
	if err != nil {
		return "", http.StatusServiceUnavailable
	}
	if response.Status == http.StatusUnauthorized || response.Status == http.StatusForbidden {
		return "", http.StatusUnauthorized
	}
	if response.Status != http.StatusOK {
		return "", http.StatusServiceUnavailable
	}
	var credential struct {
		Scope   string `json:"scope"`
		GrantID string `json:"grant_id"`
	}
	if err := json.Unmarshal(response.Body, &credential); err != nil {
		return "", http.StatusServiceUnavailable
	}
	if credential.Scope != "library" {
		return "", http.StatusUnauthorized
	}
	if credential.GrantID == "" {
		return "", http.StatusServiceUnavailable
	}
	return credential.GrantID, 0
}

func serviceURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return nil, fmt.Errorf("must be an absolute http(s) URL without credentials, query or fragment")
	}
	return u, nil
}

// Browser origins lowercase the hostname and omit the scheme's default port.
func canonicalOrigin(u *url.URL) string {
	host := strings.ToLower(u.Host)
	if u.Scheme == "https" {
		host = strings.TrimSuffix(host, ":443")
	}
	if u.Scheme == "http" {
		host = strings.TrimSuffix(host, ":80")
	}
	return u.Scheme + "://" + host
}
