package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExtractSearchText pins the av-b6o9 contract: gallery search indexes
// what an artifact *shows*, not the code it's made of.
func TestExtractSearchText(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		contains []string
		excludes []string
	}{
		{
			name:   "visible text is kept, markup dropped",
			source: `<h1>Weather <b>Dashboard</b></h1><p>Current conditions</p>`,
			contains: []string{"Weather", "Dashboard", "Current conditions"},
		},
		{
			name:     "script content is dropped",
			source:   `<p>hello</p><script>const secretIdentifier = computeThing()</script>`,
			contains: []string{"hello"},
			excludes: []string{"secretIdentifier", "computeThing", "script"},
		},
		{
			name:     "style content is dropped",
			source:   `<style>.frobnicator { background: #1a1a2e }</style><p>styled</p>`,
			contains: []string{"styled"},
			excludes: []string{"frobnicator", "background", "1a1a2e"},
		},
		{
			name:     "comments are dropped",
			source:   `<p>real</p><!-- internalNoteToken -->`,
			contains: []string{"real"},
			excludes: []string{"internalNoteToken"},
		},
		{
			name:     "semantic attributes are kept",
			source:   `<img alt="Bar chart of sales" title="Sales chart"><input placeholder="Search cities" aria-label="City picker">`,
			contains: []string{"Bar chart of sales", "Sales chart", "Search cities", "City picker"},
		},
		{
			name:     "document title is kept",
			source:   `<html><head><title>Unit Converter</title><style>body{}</style></head><body><p>convert</p></body></html>`,
			contains: []string{"Unit Converter", "convert"},
		},
		{
			name:     "whitespace is collapsed",
			source:   "<p>  lots \n\t of   space </p>",
			contains: []string{"lots of space"},
		},
		{
			name:     "plain text passes through",
			source:   `just some prose without any tags`,
			contains: []string{"just some prose without any tags"},
		},
		{
			name:     "malformed HTML does not panic and keeps text",
			source:   `<div><p>unclosed <b>bold`,
			contains: []string{"unclosed", "bold"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSearchText(tt.source)
			for _, want := range tt.contains {
				assert.Contains(t, got, want)
			}
			for _, unwanted := range tt.excludes {
				assert.NotContains(t, got, unwanted)
			}
		})
	}
}
