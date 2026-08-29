package snapshot

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Where an asset ends up is a size decision, and both directions matter: an
// icon that becomes a second HTTP request is a worse page, and a large payload
// left in the body is the cost av-20fk/av-oz40 exist to remove.
func TestPlaceRoutesBySize(t *testing.T) {
	var sunk []string
	sink := func(a RuntimeAsset) (string, error) {
		sunk = append(sunk, a.SourceURL)
		return "https://render.test/a/x/assets/stored", nil
	}

	small := &Asset{URL: "https://x.test/icon.png", ContentType: "image/png", Body: []byte("tiny")}
	assert.True(t, strings.HasPrefix(place(sink, small), "data:image/png;base64,"),
		"a small asset stays inline; one saved request beats a few hundred bytes")
	assert.Empty(t, sunk)

	// The threshold is inclusive: at exactly the limit the asset stays inline,
	// so "at or below" in the constant's doc is a claim the code keeps.
	atLimit := &Asset{URL: "https://x.test/edge.png", ContentType: "image/png",
		Body: make([]byte, InlineDataURIMaxBytes)}
	assert.True(t, strings.HasPrefix(place(sink, atLimit), "data:image/png;base64,"))
	assert.Empty(t, sunk)

	big := &Asset{URL: "https://x.test/photo.png", ContentType: "image/png",
		Body: make([]byte, InlineDataURIMaxBytes+1)}
	assert.Equal(t, "https://render.test/a/x/assets/stored", place(sink, big))
	assert.Len(t, sunk, 1)
}

// A sink that reports success but hands back no URL is the same situation as
// one that fails: there is no reference to write, and the bytes are still in
// hand. Treating it as success would put an empty src in the document.
func TestPlaceFallsBackToInliningWhenTheSinkReturnsNoURL(t *testing.T) {
	silent := func(RuntimeAsset) (string, error) { return "", nil }
	big := &Asset{URL: "https://x.test/photo.png", ContentType: "image/png",
		Body: make([]byte, InlineDataURIMaxBytes+1)}
	assert.True(t, strings.HasPrefix(place(silent, big), "data:image/png;base64,"))
}

// With no sink the function is exactly what it was before out-of-line assets
// existed, which is what keeps every caller that passes nil unchanged.
func TestPlaceWithoutASinkAlwaysInlines(t *testing.T) {
	big := &Asset{URL: "https://x.test/photo.png", ContentType: "image/png",
		Body: make([]byte, InlineDataURIMaxBytes+1)}
	assert.True(t, strings.HasPrefix(place(nil, big), "data:image/png;base64,"))
}

// A sink that fails degrades to inlining rather than to a reference that
// resolves to nothing: the bytes are already in hand, and a heavier page beats
// a broken one. Same contract every other vendoring failure here has.
func TestPlaceFallsBackToInliningWhenTheSinkFails(t *testing.T) {
	failing := func(RuntimeAsset) (string, error) { return "", errors.New("no space") }
	big := &Asset{URL: "https://x.test/photo.png", ContentType: "image/png",
		Body: make([]byte, InlineDataURIMaxBytes+1)}
	assert.True(t, strings.HasPrefix(place(failing, big), "data:image/png;base64,"))
}
