package scanner

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScan(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected []string
	}{
		{
			name:     "empty document",
			html:     "<html><body></body></html>",
			expected: []string{},
		},
		{
			name:     "script src",
			html:     `<html><head><script src="https://cdn.example.com/lib.js"></script></head></html>`,
			expected: []string{"https://cdn.example.com"},
		},
		{
			name:     "fetch in JS",
			html:     `<html><body><script>fetch("https://api.example.com/data")</script></body></html>`,
			expected: []string{"https://api.example.com"},
		},
		{
			name:     "multiple origins deduplicated",
			html:     `<html><head><script src="https://cdn.example.com/a.js"></script><script src="https://cdn.example.com/b.js"></script></head></html>`,
			expected: []string{"https://cdn.example.com"},
		},
		{
			name:     "no external origins",
			html:     `<html><body><script>const x = 1;</script></body></html>`,
			expected: []string{},
		},
		{
			name:     "ESM import",
			html:     `<html><body><script type="module">import x from "https://esm.sh/react"</script></body></html>`,
			expected: []string{"https://esm.sh"},
		},
		{
			name:     "base tag is not a contact and does not resolve relatives",
			html:     `<html><head><base href="https://source.example.com/"></head><body><img src="a.png"></body></html>`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origins := Scan(tt.html)
			sort.Strings(origins)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, origins)
		})
	}
}

// TestScanDocResolvesThroughDocBase: relatives resolve against the document's
// own <base>, exactly where the browser sends them.
func TestScanDocResolvesThroughDocBase(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		base     string
		expected []string
	}{
		{
			name:     "relative src resolves against base",
			html:     `<html><body><script src="js/app.js"></script><img src="/assets/logo.png"></body></html>`,
			base:     "https://source.example.com/tools/page.html",
			expected: []string{"https://source.example.com"},
		},
		{
			name:     "relative stylesheet link resolves against base",
			html:     `<html><head><link rel="stylesheet" href="css/main.css"></head></html>`,
			base:     "https://source.example.com/page",
			expected: []string{"https://source.example.com"},
		},
		{
			name:     "relative fetch and import literals resolve against base",
			html:     `<html><body><script type="module">import x from "./lib/util.js"; fetch("/api/data")</script></body></html>`,
			base:     "https://cdn.source.com/app/",
			expected: []string{"https://cdn.source.com"},
		},
		{
			name:     "dot-dot relative resolves to base origin",
			html:     `<html><body><img src="../other/pic.png"></body></html>`,
			base:     "https://a.example.com/deep/dir/index.html",
			expected: []string{"https://a.example.com"},
		},
		{
			name:     "base preserves http scheme when resolving relatives",
			html:     `<html><body><script src="app.js"></script></body></html>`,
			base:     "http://plain.example.com/x",
			expected: []string{"http://plain.example.com"},
		},
		{
			name:     "absolute refs unaffected by base",
			html:     `<html><head><script src="https://cdn.other.com/lib.js"></script></head><body><script>fetch("https://api.other.com/data")</script></body></html>`,
			base:     "https://source.example.com/page",
			expected: []string{"https://cdn.other.com", "https://api.other.com"},
		},
		{
			name:     "mix of absolute and relative with base",
			html:     `<html><head><script src="https://cdn.other.com/lib.js"></script><script src="local/app.js"></script></head></html>`,
			base:     "https://source.example.com/page",
			expected: []string{"https://cdn.other.com", "https://source.example.com"},
		},
		{
			name:     "empty base drops relatives (equals Scan)",
			html:     `<html><body><script src="js/app.js"></script><script src="https://cdn.other.com/lib.js"></script></body></html>`,
			base:     "",
			expected: []string{"https://cdn.other.com"},
		},
		{
			name:     "relative base drops relatives (equals Scan)",
			html:     `<html><body><script src="js/app.js"></script><script src="https://cdn.other.com/lib.js"></script></body></html>`,
			base:     "not-an-absolute-url",
			expected: []string{"https://cdn.other.com"},
		},
		{
			name:     "non-http base scheme drops relatives (equals Scan)",
			html:     `<html><body><script src="js/app.js"></script></body></html>`,
			base:     "ftp://files.example.com/x",
			expected: []string{},
		},
		{
			name:     "protocol-relative base takes https",
			html:     `<html><body><img src="a.png"></body></html>`,
			base:     "//cdn.example/x/",
			expected: []string{"https://cdn.example"},
		},
		{
			name:     "base href is trimmed like the browser trims it",
			html:     `<html><body><img src="a.png"></body></html>`,
			base:     " https://ws.example/ ",
			expected: []string{"https://ws.example"},
		},
		{
			name:     "data uri still dropped even with a base",
			html:     `<html><body><img src="data:image/png;base64,iVBORw0KGgo="></body></html>`,
			base:     "https://source.example.com/page",
			expected: []string{},
		},
		{
			name:     "anchor href still ignored with a base",
			html:     `<html><body><a href="page2.html">next</a></body></html>`,
			base:     "https://source.example.com/page",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origins := ScanDoc(`<base href="` + tt.base + `">` + tt.html)
			sort.Strings(origins)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, origins)
		})
	}
}

// TestScanDocWithoutUsableBaseMatchesScan proves the no-base path is identical
// to Scan: a document with no base, or one whose governing base is not an
// absolute http(s) URL, must scan exactly as if the tag were not there.
func TestScanDocWithoutUsableBaseMatchesScan(t *testing.T) {
	samples := []string{
		`<html><body></body></html>`,
		`<html><head><script src="https://cdn.example.com/lib.js"></script></head></html>`,
		`<html><body><script src="js/app.js"></script>fetch("/api/x")</body></html>`,
		`<html><body><script type="module">import x from "./util.js"</script></body></html>`,
		`<html><body><img src="//proto.example.com/x.png"></body></html>`,
	}
	prefixes := []string{
		``,
		`<base href="">`,
		`<base href="not-an-absolute-url">`,
		`<base href="ftp://files.example.com/x">`,
		`<base target="_blank">`,
	}
	for _, s := range samples {
		want := Scan(s)
		sort.Strings(want)

		for _, prefix := range prefixes {
			got := ScanDoc(prefix + s)
			sort.Strings(got)
			assert.Equal(t, want, got, "ScanDoc with prefix %q should equal Scan", prefix)
		}
	}
}

// TestScanDoc covers the av-wu9d contract: the document's own <base> governs
// relatives but is never itself reported as a contact.
func TestScanDoc(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected []string
	}{
		{
			name:     "base tag itself is never reported",
			html:     `<html><head><base href="https://source.example.com/blog/post"></head><body><h1>hi</h1></body></html>`,
			expected: []string{},
		},
		{
			name:     "inherited relative resolves through preserved base",
			html:     `<html><head><base href="https://source.example.com/blog/post"></head><body><img src="/images/logo.png"></body></html>`,
			expected: []string{"https://source.example.com"},
		},
		{
			name:     "relative fetch literal resolves through preserved base",
			html:     `<html><head><base href="https://source.example.com/blog/post"></head><body><script>fetch('/api/data')</script></body></html>`,
			expected: []string{"https://source.example.com"},
		},
		{
			name:     "authored local without base stays local",
			html:     `<html><body><h1>hi</h1><img src="/new.png"></body></html>`,
			expected: []string{},
		},
		{
			name:     "absolute refs unaffected by doc base",
			html:     `<html><head><base href="https://source.example.com/x"></head><body><script src="https://cdn.other.com/lib.js"></script></body></html>`,
			expected: []string{"https://cdn.other.com"},
		},
		{
			name:     "non-http doc base is ignored",
			html:     `<html><head><base href="ftp://files.example.com/x"></head><body><img src="/a.png"></body></html>`,
			expected: []string{},
		},
		{
			name:     "relative doc base is ignored",
			html:     `<html><head><base href="/rooted/base"></head><body><img src="/a.png"></body></html>`,
			expected: []string{},
		},
		{
			name:     "first base with an href governs, even an empty one",
			html:     `<html><head><base href=""><base href="https://late.example/"></head><body><img src="a.png"></body></html>`,
			expected: []string{},
		},
		{
			name:     "a relative first base shadows a later absolute one",
			html:     `<html><head><base href="/local/"><base href="https://late.example/"></head><body><img src="a.png"></body></html>`,
			expected: []string{},
		},
		{
			name:     "base without href is skipped",
			html:     `<html><head><base target="_blank"><base href="https://second.example/"></head><body><img src="a.png"></body></html>`,
			expected: []string{"https://second.example"},
		},
		{
			name:     "base in svg foreign content does not govern",
			html:     `<html><body><svg><base href="https://svg.example/"></base></svg><img src="a.png"></body></html>`,
			expected: []string{},
		},
		{
			name:     "base inside an inert template does not govern",
			html:     `<html><head><template><base href="https://tmpl.example/"></template></head><body><img src="a.png"></body></html>`,
			expected: []string{},
		},
		{
			name:     "anchor href still ignored with doc base",
			html:     `<html><head><base href="https://source.example.com/x"></head><body><a href="page2.html">next</a></body></html>`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origins := ScanDoc(tt.html)
			sort.Strings(origins)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, origins)
		})
	}
}
