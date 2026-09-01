package origin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormalizeOriginAccepts covers the values that are origins already, or
// that differ from one only in spelling the canonical form fixes.
func TestNormalizeOriginAccepts(t *testing.T) {
	cases := map[string]string{
		"plain":                  "https://example.com",
		"trailing slash":         "https://example.com/",
		"uppercase host":         "https://EXAMPLE.com",
		"uppercase scheme":       "HTTPS://example.com",
		"trailing dot host":      "https://unpkg.com.",
		"dot and slash":          "https://unpkg.com./",
		"default port":           "https://example.com:443",
		"explicit port":          "https://example.com:8443",
		"leading-zero port":      "https://example.com:08080",
		"leading-zero default":   "https://example.com:0443",
		"http loopback":          "http://localhost:3000",
		"http loopback ip":       "http://127.0.0.1:8080",
		"https ipv6":             "https://[::1]",
		"subdomain":              "https://cdn.jsdelivr.net",
		"host with underscore":   "https://my_cdn.example.com",
		"http localhost no port": "http://localhost",
	}
	want := map[string]string{
		"plain":                  "https://example.com",
		"trailing slash":         "https://example.com",
		"uppercase host":         "https://example.com",
		"uppercase scheme":       "https://example.com",
		"trailing dot host":      "https://unpkg.com",
		"dot and slash":          "https://unpkg.com",
		"default port":           "https://example.com",
		"explicit port":          "https://example.com:8443",
		"leading-zero port":      "https://example.com:8080",
		"leading-zero default":   "https://example.com",
		"http loopback":          "http://localhost:3000",
		"http loopback ip":       "http://127.0.0.1:8080",
		"https ipv6":             "https://[::1]",
		"subdomain":              "https://cdn.jsdelivr.net",
		"host with underscore":   "https://my_cdn.example.com",
		"http localhost no port": "http://localhost",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := NormalizeOrigin(in)
			require.NoError(t, err)
			assert.Equal(t, want[name], got)
		})
	}
}

// TestNormalizeOriginRecoverable covers inputs that carry more than an origin.
// They are errors (the write path rejects them) but still yield the origin they
// name, which is what the legacy-row repair salvages.
func TestNormalizeOriginRecoverable(t *testing.T) {
	cases := map[string]struct{ in, origin string }{
		"path": {
			"https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/esm/worker.js",
			"https://unpkg.com",
		},
		"path and trailing dot": {
			"https://unpkg.com./dist/worker.js",
			"https://unpkg.com",
		},
		"query":    {"https://example.com/?a=1", "https://example.com"},
		"fragment": {"https://example.com/#frag", "https://example.com"},
		"userinfo": {"https://user:pw@example.com", "https://example.com"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := NormalizeOrigin(tc.in)
			require.Error(t, err, "a non-origin value must not pass the write path")
			assert.Equal(t, tc.origin, got, "the derived origin is still returned for the legacy repair")
		})
	}
}

// TestNormalizeOriginRejects covers values the write path refuses outright:
// the error is fatal and nothing is salvaged, not even a derived origin.
func TestNormalizeOriginRejects(t *testing.T) {
	for name, in := range map[string]string{
		"empty":             "",
		"whitespace only":   "   ",
		"surrounding space": " https://example.com ",
		"internal space":    "https://exa mple.com",
		"zero port":         "https://example.com:0",
		"port above max":    "https://example.com:65536",
		"five-digit port":   "https://example.com:99999",
		"scheme-less host":  "example.com",
		"scheme-less path":  "/lib/thing.js",
		"protocol relative": "//cdn.example.com",
		"wildcard host":     "https://*.example.com",
		"bare wildcard":     "*",
		"keyword self":      "'self'",
		"keyword none":      "'none'",
		"keyword inline":    "'unsafe-inline'",
		"data scheme":       "data:",
		"blob scheme":       "blob:",
		"data uri":          "data:text/html,hi",
		"blob uri":          "blob:https://example.com/abc",
		"ftp scheme":        "ftp://example.com",
		"javascript":        "javascript:alert(1)",
		"mailto":            "mailto:a@example.com",
		"http remote":       "http://example.com",
		"no host":           "https://",
		"csp injection":     "https://example.com; frame-ancestors *",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := NormalizeOrigin(in)
			require.Error(t, err)
			assert.Empty(t, got, "nothing may be salvaged from a value with no origin")
		})
	}
}

func TestNormalizeOriginsDeduplicatesPreservingOrder(t *testing.T) {
	got, err := NormalizeOrigins([]string{
		"https://b.example.com",
		"https://A.example.com",
		"https://a.example.com.",
		"https://a.example.com:443/",
		"https://b.example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://b.example.com", "https://a.example.com"}, got)
}

func TestNormalizeOriginsNamesTheOffendingEntry(t *testing.T) {
	_, err := NormalizeOrigins([]string{"https://ok.example.com", "https://unpkg.com/dist/x.js"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https://unpkg.com/dist/x.js")
}

func TestNormalizeOriginsEmpty(t *testing.T) {
	got, err := NormalizeOrigins(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
