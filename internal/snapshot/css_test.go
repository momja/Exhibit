package snapshot

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cssServer serves a fixed set of paths and records which were requested, so a
// test can assert an asset was fetched against the correct per-sheet base.
type cssServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func newCSSServer(t *testing.T, routes map[string]func(w http.ResponseWriter)) *cssServer {
	t.Helper()
	cs := &cssServer{hits: map[string]int{}}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		cs.hits[r.URL.Path]++
		cs.mu.Unlock()
		if h, ok := routes[r.URL.Path]; ok {
			h(w)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(cs.Close)
	return cs
}

func (cs *cssServer) hitCount(path string) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.hits[path]
}

func servePNG(body []byte) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}
}

func serveCSS(body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte(body))
	}
}

func pngDataURI(body []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(body)
}

// TestInlineCSSURLForms inlines url() in every quoting style and proves the
// reference resolves against the CSS's own base (…/css/style.css), not the
// origin root.
func TestInlineCSSURLForms(t *testing.T) {
	img := []byte("\x89PNG-quoted-and-unquoted")
	srv := newCSSServer(t, map[string]func(http.ResponseWriter){
		"/css/img/bg.png": servePNG(img),
	})

	css := strings.Join([]string{
		`a { background: url(img/bg.png); }`,
		`b { background: url("img/bg.png"); }`,
		`c { background: url('img/bg.png'); }`,
		`d { background: url(  img/bg.png  ); }`, // surrounding whitespace tolerated
	}, "\n")

	f := testFetcher(t, srv.URL+"/css/style.css", DefaultLimits())
	out, errs := InlineCSS(context.Background(), f, srv.URL+"/css/style.css", css)

	require.Empty(t, errs)
	want := `url("` + pngDataURI(img) + `")`
	assert.Equal(t, 4, strings.Count(out, want), "every url() form should inline to the same data URI:\n%s", out)
	assert.NotContains(t, out, "img/bg.png", "no relative reference should survive")
	assert.Equal(t, 1, srv.hitCount("/css/img/bg.png"), "identical URLs fetch once")
}

// TestInlineCSSSkipsNonNetworkRefs leaves data:/blob:/about:/#fragment targets
// untouched with no error and never hits the network.
func TestInlineCSSSkipsNonNetworkRefs(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()

	css := strings.Join([]string{
		`a { background: url(data:image/png;base64,AAAA); }`,
		`b { background: url("#gradient"); }`,
		`c { background: url(about:blank); }`,
		`d { background: url(blob:https://x/y); }`,
		`e { mask: url(#clip); }`,
		`@import "javascript:alert(1)";`,
	}, "\n")

	f := testFetcher(t, srv.URL+"/style.css", DefaultLimits())
	out, errs := InlineCSS(context.Background(), f, srv.URL+"/style.css", css)

	assert.Equal(t, css, out, "non-network refs must pass through unchanged")
	assert.Empty(t, errs)
	assert.False(t, hit, "no network fetch for non-network schemes")
}

// TestInlineCSSNestedImportRebase is the crux of the ticket: a nested @import
// chain must resolve each sheet's relative assets against THAT sheet's URL. The
// inner sheet lives at /a/sub/b.css, so its `img.png` must resolve to
// /a/sub/img.png — not /a/img.png against the outer sheet's base.
func TestInlineCSSNestedImportRebase(t *testing.T) {
	outerBG := []byte("outer-bg-bytes")
	innerIMG := []byte("inner-img-bytes")

	srv := newCSSServer(t, map[string]func(http.ResponseWriter){
		"/a/sub/b.css":   serveCSS(`.x { background: url(img.png); }`),
		"/a/bg.png":      servePNG(outerBG),
		"/a/sub/img.png": servePNG(innerIMG),
	})

	// The outer sheet body is supplied directly (as exhibit-lwb.3 would after
	// fetching it); its base is /a/main.css.
	outer := "@import url(sub/b.css);\nbody { background: url(bg.png); }"

	f := testFetcher(t, srv.URL+"/a/main.css", DefaultLimits())
	out, errs := InlineCSS(context.Background(), f, srv.URL+"/a/main.css", outer)

	require.Empty(t, errs)
	assert.NotContains(t, out, "@import", "the @import must be folded away")
	assert.Contains(t, out, `url("`+pngDataURI(outerBG)+`")`, "outer bg.png inlined against /a/")
	assert.Contains(t, out, `url("`+pngDataURI(innerIMG)+`")`, "inner img.png inlined against /a/sub/")
	assert.NotContains(t, out, "url(img.png)", "inner relative ref must be rewritten")

	assert.Equal(t, 1, srv.hitCount("/a/sub/b.css"), "imported sheet fetched")
	assert.Equal(t, 1, srv.hitCount("/a/sub/img.png"), "inner asset fetched against per-sheet base")
	assert.Equal(t, 0, srv.hitCount("/a/img.png"), "must NOT resolve inner ref against the outer base")
}

// TestInlineCSSImportStringAndMedia covers the @import "…" string form and
// preservation of a media query by wrapping the folded rules in @media.
func TestInlineCSSImportStringAndMedia(t *testing.T) {
	img := []byte("print-bytes")
	srv := newCSSServer(t, map[string]func(http.ResponseWriter){
		"/print.css": serveCSS(`.p { background: url(p.png); }`),
		"/p.png":     servePNG(img),
	})

	css := `@import "print.css" print and (min-width: 400px);`
	f := testFetcher(t, srv.URL+"/main.css", DefaultLimits())
	out, errs := InlineCSS(context.Background(), f, srv.URL+"/main.css", css)

	require.Empty(t, errs)
	assert.Contains(t, out, "@media print and (min-width: 400px) {", "media query preserved as @media wrapper")
	assert.Contains(t, out, `url("`+pngDataURI(img)+`")`)
	assert.NotContains(t, out, "@import")
}

// TestInlineCSSFailuresLeftVerbatim: an over-limit asset and a 404 asset are
// each left verbatim in the output and reported as a *FetchError carrying the
// resolved origin, while a sibling good asset still inlines. One failure never
// aborts the transform.
func TestInlineCSSFailuresLeftVerbatim(t *testing.T) {
	good := []byte("ok")
	big := strings.Repeat("x", 4096)
	srv := newCSSServer(t, map[string]func(http.ResponseWriter){
		"/good.png": servePNG(good),
		"/big.png": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(big))
		},
		// /missing.png is unregistered → 404.
	})

	css := "a{background:url(good.png)} b{background:url(big.png)} c{background:url(missing.png)}"

	limits := DefaultLimits()
	limits.MaxAssetBytes = 1024
	f := testFetcher(t, srv.URL+"/style.css", limits)
	out, errs := InlineCSS(context.Background(), f, srv.URL+"/style.css", css)

	assert.Contains(t, out, `url("`+pngDataURI(good)+`")`, "good asset inlined")
	assert.Contains(t, out, "url(big.png)", "over-limit asset left verbatim")
	assert.Contains(t, out, "url(missing.png)", "failed asset left verbatim")

	require.Len(t, errs, 2)
	byKind := map[ErrorKind]*FetchError{}
	for _, e := range errs {
		byKind[e.Kind] = e
	}
	require.Contains(t, byKind, ErrTooLarge)
	require.Contains(t, byKind, ErrHTTPStatus)
	assert.Equal(t, "big.png", byKind[ErrTooLarge].Ref, "error reports the CSS reference")
	assert.Equal(t, srv.URL+"/big.png", byKind[ErrTooLarge].URL, "resolved origin preserved for the scanner")
	assert.Equal(t, srv.URL+"/missing.png", byKind[ErrHTTPStatus].URL)
}

// TestInlineCSSSelfImportTerminates: a sheet that @imports itself must not
// loop. The cycle back-edge is left verbatim and the rest of the sheet still
// processes.
func TestInlineCSSSelfImportTerminates(t *testing.T) {
	img := []byte("self-bytes")
	srv := newCSSServer(t, map[string]func(http.ResponseWriter){
		"/self.css": serveCSS("@import url(self.css); body { background: url(x.png); }"),
		"/x.png":    servePNG(img),
	})

	// The body passed in is self.css's own content, based at self.css.
	body := "@import url(self.css);\nbody { background: url(x.png); }"
	f := testFetcher(t, srv.URL+"/self.css", DefaultLimits())
	out, errs := InlineCSS(context.Background(), f, srv.URL+"/self.css", body)

	// Terminates. The self-import is a no-op back-edge (never fetched), the
	// real asset still inlines.
	assert.Contains(t, out, "@import url(self.css)", "self-import left verbatim as a cycle back-edge")
	assert.Contains(t, out, `url("`+pngDataURI(img)+`")`)
	assert.Empty(t, errs, "a cycle is not a fetch failure")
	assert.Equal(t, 0, srv.hitCount("/self.css"), "cycle detected before any refetch")
}

// TestInlineCSSMutualImportCycleTerminates: a → b → a. The back-edge from b to
// a is dropped (verbatim) while both sheets otherwise fold.
func TestInlineCSSMutualImportCycleTerminates(t *testing.T) {
	srv := newCSSServer(t, map[string]func(http.ResponseWriter){
		"/b.css": serveCSS("@import url(a.css); .b { color: blue; }"),
	})

	a := "@import url(b.css);\n.a { color: red; }"
	f := testFetcher(t, srv.URL+"/a.css", DefaultLimits())
	out, errs := InlineCSS(context.Background(), f, srv.URL+"/a.css", a)

	assert.Contains(t, out, ".b { color: blue; }", "b.css folded into a")
	assert.Contains(t, out, ".a { color: red; }")
	assert.Contains(t, out, "@import url(a.css)", "the a→b→a back-edge stays verbatim")
	assert.Empty(t, errs)
	assert.Equal(t, 1, srv.hitCount("/b.css"), "b fetched exactly once")
}

// TestInlineCSSImportDepthCap: a linear, acyclic @import chain deeper than
// maxImportDepth stops folding and reports the over-cap import, without looping.
func TestInlineCSSImportDepthCap(t *testing.T) {
	routes := map[string]func(http.ResponseWriter){}
	// Sheets 1..N each import the next: /s1.css → /s2.css → …
	const chain = maxImportDepth + 3
	for i := 1; i <= chain; i++ {
		next := fmt.Sprintf("@import url(s%d.css); .s%d{}", i+1, i)
		routes[fmt.Sprintf("/s%d.css", i)] = serveCSS(next)
	}
	srv := newCSSServer(t, routes)

	root := "@import url(s1.css);"
	f := testFetcher(t, srv.URL+"/root.css", DefaultLimits())
	out, errs := InlineCSS(context.Background(), f, srv.URL+"/root.css", root)

	require.Len(t, errs, 1)
	assert.Equal(t, ErrBudget, errs[0].Kind, "the over-cap import is reported as budget-exhausted")
	assert.Contains(t, out, ".s1{}", "shallow sheets still fold")
	// Folding covers root(0)+s1..s8 (depths 1..8); s9 onward is not fetched.
	assert.Equal(t, 0, srv.hitCount(fmt.Sprintf("/s%d.css", maxImportDepth+1)),
		"sheets past the nesting cap are never fetched")
}

// TestInlineCSSIdentityWhenNoRefs returns the input unchanged (and nil errors)
// when there is nothing to inline.
func TestInlineCSSIdentityWhenNoRefs(t *testing.T) {
	css := "body { color: red; margin: 0; }"
	f := testFetcher(t, "https://example.com/style.css", DefaultLimits())
	out, errs := InlineCSS(context.Background(), f, "https://example.com/style.css", css)
	assert.Equal(t, css, out)
	assert.Nil(t, errs)
}
