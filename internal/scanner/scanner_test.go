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
