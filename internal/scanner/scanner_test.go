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

func TestScanWithBase(t *testing.T) {
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
			name:     "invalid base drops relatives (equals Scan)",
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
			origins := ScanWithBase(tt.html, tt.base)
			sort.Strings(origins)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, origins)
		})
	}
}

// TestScanWithBaseMatchesScanWithoutBase proves the no-base path is byte-for-byte
// identical to Scan: an empty or invalid base must not change the result at all.
func TestScanWithBaseMatchesScanWithoutBase(t *testing.T) {
	samples := []string{
		`<html><body></body></html>`,
		`<html><head><script src="https://cdn.example.com/lib.js"></script></head></html>`,
		`<html><body><script src="js/app.js"></script>fetch("/api/x")</body></html>`,
		`<html><body><script type="module">import x from "./util.js"</script></body></html>`,
		`<html><body><img src="//proto.example.com/x.png"></body></html>`,
	}
	for _, s := range samples {
		want := Scan(s)
		sort.Strings(want)

		for _, base := range []string{"", "not-an-absolute-url", "ftp://files.example.com/x"} {
			got := ScanWithBase(s, base)
			sort.Strings(got)
			assert.Equal(t, want, got, "ScanWithBase(_, %q) should equal Scan", base)
		}
	}
}
