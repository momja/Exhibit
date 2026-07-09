package snapshot

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAsset is one resource served by the fake asset origin.
type testAsset struct {
	contentType string
	body        []byte
}

// assetOrigin stands in for the imported page's source site. Paths present in
// the map are served with their content type (omitted when empty); everything
// else 404s so failure paths are easy to exercise.
func assetOrigin(t *testing.T, assets map[string]testAsset) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := assets[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if a.contentType != "" {
			w.Header().Set("Content-Type", a.contentType)
		}
		_, _ = w.Write(a.body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// inline runs the transform with the dial guard disabled (via testFetcher) so
// it can reach the loopback httptest origin.
func inline(t *testing.T, base, body string, limits Limits) (string, []*FetchError) {
	t.Helper()
	f := testFetcher(t, base, limits)
	out, errs, err := InlineHTMLAssets(context.Background(), f, body)
	require.NoError(t, err)
	return out, errs
}

func TestInlineHTMLAssets(t *testing.T) {
	pic := []byte("\x89PNG-pic-bytes")
	oneX := []byte("one-x-bytes")
	twoX := []byte("two-x-bytes")
	js := []byte("console.log('hi');")
	css := []byte("body{color:red}")
	favicon := []byte("favicon-bytes")
	apple := []byte("apple-icon-bytes")
	srcBytes := []byte("video-source-bytes")

	assets := map[string]testAsset{
		"/img/pic.png":     {"image/png", pic},
		"/1x.png":          {"image/png", oneX},
		"/2x.png":          {"image/png", twoX},
		"/app.js":          {"application/javascript", js},
		"/style.css":       {"text/css; charset=utf-8", css},
		"/favicon.ico":     {"image/x-icon", favicon},
		"/icons/apple.png": {"image/png", apple},
		"/media/clip.webm": {"video/webm", srcBytes},
	}
	srv := assetOrigin(t, assets)
	base := srv.URL + "/index.html"

	dataURI := func(ct string, b []byte) string { return "data:" + ct + ";base64," + b64(b) }

	t.Run("img src and srcset", func(t *testing.T) {
		out, errs := inline(t, base, `<html><body>`+
			`<img src="img/pic.png" srcset="1x.png 1x, 2x.png 2x" alt="pic">`+
			`</body></html>`, DefaultLimits())
		require.Empty(t, errs)
		assert.Contains(t, out, `src="`+dataURI("image/png", pic)+`"`)
		wantSrcset := dataURI("image/png", oneX) + " 1x, " + dataURI("image/png", twoX) + " 2x"
		assert.Contains(t, out, `srcset="`+wantSrcset+`"`)
		assert.Contains(t, out, `alt="pic"`) // unrelated attributes preserved
	})

	t.Run("script src becomes inline script", func(t *testing.T) {
		out, errs := inline(t, base, `<html><body><script src="app.js"></script></body></html>`, DefaultLimits())
		require.Empty(t, errs)
		assert.Contains(t, out, `<script>console.log('hi');</script>`)
		assert.NotContains(t, out, "app.js")
		assert.NotContains(t, out, "src=")
	})

	t.Run("stylesheet link becomes inline style", func(t *testing.T) {
		out, errs := inline(t, base, `<html><head><link rel="stylesheet" href="style.css"></head><body></body></html>`, DefaultLimits())
		require.Empty(t, errs)
		assert.Contains(t, out, `<style>body{color:red}</style>`)
		assert.NotContains(t, out, "<link")
		assert.NotContains(t, out, "style.css")
	})

	t.Run("favicon and apple-touch-icon become data URIs", func(t *testing.T) {
		out, errs := inline(t, base, `<html><head>`+
			`<link rel="icon" href="favicon.ico">`+
			`<link rel="apple-touch-icon" href="icons/apple.png">`+
			`</head><body></body></html>`, DefaultLimits())
		require.Empty(t, errs)
		assert.Contains(t, out, `<link rel="icon" href="`+dataURI("image/x-icon", favicon)+`"`)
		assert.Contains(t, out, `<link rel="apple-touch-icon" href="`+dataURI("image/png", apple)+`"`)
	})

	t.Run("shortcut icon rel matches", func(t *testing.T) {
		out, errs := inline(t, base, `<html><head><link rel="shortcut icon" href="favicon.ico"></head><body></body></html>`, DefaultLimits())
		require.Empty(t, errs)
		assert.Contains(t, out, dataURI("image/x-icon", favicon))
	})

	t.Run("source element src and srcset", func(t *testing.T) {
		out, errs := inline(t, base, `<html><body>`+
			`<video><source src="media/clip.webm"></video>`+
			`<picture><source srcset="1x.png 1x, 2x.png 2x"><img src="img/pic.png"></picture>`+
			`</body></html>`, DefaultLimits())
		require.Empty(t, errs)
		assert.Contains(t, out, `src="`+dataURI("video/webm", srcBytes)+`"`)
		wantSrcset := dataURI("image/png", oneX) + " 1x, " + dataURI("image/png", twoX) + " 2x"
		assert.Contains(t, out, `srcset="`+wantSrcset+`"`)
	})

	t.Run("anchors and forms are untouched", func(t *testing.T) {
		out, errs := inline(t, base, `<html><body>`+
			`<a href="other-page.html">nav</a>`+
			`<form action="/submit"><input name="q"></form>`+
			`</body></html>`, DefaultLimits())
		require.Empty(t, errs, "navigation targets must not be fetched")
		assert.Contains(t, out, `href="other-page.html"`)
		assert.Contains(t, out, `action="/submit"`)
		assert.NotContains(t, out, "data:")
	})

	t.Run("inline style block and style attribute survive", func(t *testing.T) {
		// InlineCSS is the identity stub on this branch, so CSS passes through
		// unchanged — this exercises the seam wiring without asserting CSS rewrites.
		out, errs := inline(t, base, `<html><head><style>.a{color:blue}</style></head>`+
			`<body><div style="color:green">x</div></body></html>`, DefaultLimits())
		require.Empty(t, errs)
		assert.Contains(t, out, `<style>.a{color:blue}</style>`)
		assert.Contains(t, out, `style="color:green"`)
	})
}

func TestInlineHTMLAssetsResidualFailures(t *testing.T) {
	pic := []byte("\x89PNG-pic-bytes")
	assets := map[string]testAsset{
		"/img/pic.png": {"image/png", pic},
		"/big.png":     {"image/png", []byte("0123456789")}, // 10 bytes
	}
	srv := assetOrigin(t, assets)
	base := srv.URL + "/index.html"

	t.Run("failed fetch left as residual reference", func(t *testing.T) {
		out, errs := inline(t, base, `<html><body><img src="missing.png"><img src="img/pic.png"></body></html>`, DefaultLimits())
		// The good asset still inlines...
		assert.Contains(t, out, "data:image/png;base64,"+b64(pic))
		// ...while the 404 keeps its original reference.
		assert.Contains(t, out, `src="missing.png"`)
		require.Len(t, errs, 1)
		assert.Equal(t, ErrHTTPStatus, errs[0].Kind)
		assert.Contains(t, errs[0].URL, "/missing.png")
	})

	t.Run("over-limit asset left as residual reference", func(t *testing.T) {
		tiny := DefaultLimits()
		tiny.MaxAssetBytes = 4 // /big.png is 10 bytes
		tiny.Timeout = 5 * time.Second
		out, errs := inline(t, base, `<html><body><img src="big.png"></body></html>`, tiny)
		assert.Contains(t, out, `src="big.png"`)
		assert.NotContains(t, out, "data:")
		require.Len(t, errs, 1)
		assert.Equal(t, ErrTooLarge, errs[0].Kind)
	})

	t.Run("non-network schemes are skipped without error", func(t *testing.T) {
		in := `<html><body>` +
			`<img src="data:image/gif;base64,R0lGOD">` +
			`<a href="mailto:x@y.z">mail</a>` +
			`<img src="#anchor">` +
			`</body></html>`
		out, errs := inline(t, base, in, DefaultLimits())
		require.Empty(t, errs)
		assert.Contains(t, out, `src="data:image/gif;base64,R0lGOD"`)
		assert.Contains(t, out, `href="mailto:x@y.z"`)
		assert.Contains(t, out, `src="#anchor"`)
	})
}

func TestInlineHTMLAssetsParseError(t *testing.T) {
	// html.Parse is extremely lenient and effectively never errors on string
	// input, so a normal document round-trips cleanly and returns no error.
	f := testFetcher(t, "https://example.com/page.html", DefaultLimits())
	out, errs, err := InlineHTMLAssets(context.Background(), f, "<html><body>plain</body></html>")
	require.NoError(t, err)
	require.Empty(t, errs)
	assert.Contains(t, out, "plain")
}

func TestGuessContentType(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://x.example/a/logo.png", "image/png"},
		{"https://x.example/style.css", "text/css; charset=utf-8"},
		{"https://x.example/data.unknownext", "application/octet-stream"},
		{"https://x.example/noextension", "application/octet-stream"},
	}
	for _, tt := range tests {
		got := guessContentType(tt.url)
		// Normalize the mime DB's optional charset/space differences: we only
		// require the leading media type to match.
		assert.Truef(t, strings.HasPrefix(got, strings.Split(tt.want, ";")[0]),
			"guessContentType(%q) = %q, want prefix %q", tt.url, got, tt.want)
	}
}

func TestParseSrcset(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []srcsetCandidate
	}{
		{"descriptors", "a.png 1x, b.png 2x", []srcsetCandidate{{"a.png", "1x"}, {"b.png", "2x"}}},
		{"width descriptors", "s.png 480w, l.png 1024w", []srcsetCandidate{{"s.png", "480w"}, {"l.png", "1024w"}}},
		{"no descriptor", "only.png", []srcsetCandidate{{"only.png", ""}}},
		{"trailing comma descriptorless", "a.png, b.png 2x", []srcsetCandidate{{"a.png", ""}, {"b.png", "2x"}}},
		{"extra whitespace", "  a.png   1x ,  b.png 2x ", []srcsetCandidate{{"a.png", "1x"}, {"b.png", "2x"}}},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseSrcset(tt.input))
		})
	}
}
