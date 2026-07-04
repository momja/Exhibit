package render

import (
	"strings"
	"testing"
)

// connectSrc extracts the connect-src directive value from a CSP string.
func connectSrc(t *testing.T, csp string) string {
	t.Helper()
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		if v, ok := strings.CutPrefix(d, "connect-src "); ok {
			return v
		}
	}
	t.Fatalf("no connect-src directive in CSP: %q", csp)
	return ""
}

// The storage shim fetches appOrigin/api/artifacts/:id/state. If the app origin
// is missing from connect-src, the browser blocks the shim's hydrate and
// write-through and state silently never persists. Guard both allowlist paths.
func TestBuildCSPConnectSrcAlwaysIncludesAppOrigin(t *testing.T) {
	const appOrigin = "https://app.example.com"

	t.Run("empty allowlist", func(t *testing.T) {
		cs := connectSrc(t, buildCSP(nil, appOrigin))
		if !strings.Contains(cs, appOrigin) {
			t.Fatalf("connect-src %q missing app origin %q", cs, appOrigin)
		}
		if strings.Contains(cs, "'none'") {
			t.Fatalf("connect-src is 'none' — shim state proxy would be blocked: %q", cs)
		}
	})

	t.Run("populated allowlist", func(t *testing.T) {
		cs := connectSrc(t, buildCSP([]string{"https://api.github.com"}, appOrigin))
		if !strings.Contains(cs, appOrigin) {
			t.Fatalf("connect-src %q missing app origin %q", cs, appOrigin)
		}
		if !strings.Contains(cs, "https://api.github.com") {
			t.Fatalf("connect-src %q dropped the allowlisted origin", cs)
		}
	})
}
