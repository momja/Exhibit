package render

import (
	"context"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/artifacticon"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

func TestArtifactFavicon(t *testing.T) {
	rd, st := newTestRenderer(t, "icon", `<html><head data-test="head"><link rel="shortcut ICON" href="old.ico"></head><body>hello</body></html>`)
	ctx := context.Background()
	tags := []*store.Tag{{ID: "blue", OwnerID: 1, Name: "Blue", Color: "#00f"}}
	require.NoError(t, st.CreateTag(ctx, tags[0]))
	require.NoError(t, st.AddArtifactTag(ctx, 1, "icon", "blue"))
	out := serve(t, rd, "icon")
	require.Contains(t, out, `href="`+artifacticon.DataURI(tags)+`"`)
	require.NotContains(t, out, "old.ico")
	require.NoError(t, st.RemoveArtifactTag(ctx, 1, "icon", "blue"))
	require.Contains(t, serve(t, rd, "icon"), `href="`+artifacticon.DataURI(nil)+`"`)
}

func TestInjectFaviconPreservesArtifact(t *testing.T) {
	for _, body := range []string{
		`<!doctype html><HTML><HEAD lang="en"><title>Tool</title></HEAD><body>hi</body></HTML>`,
		`<p>fragment</p>`,
		`<head><script>const example = '<link rel="icon" href="keep">';</script><link rel="icon" href="remove"><link rel="stylesheet" href="keep.css"></head>`,
	} {
		out := injectFavicon(body, "test-icon")
		require.Equal(t, 1, strings.Count(out, `href="test-icon"`))
		require.NotContains(t, out, `href="remove"`)
		if strings.Contains(body, "<script>") {
			require.Contains(t, out, `<script>const example = '<link rel="icon" href="keep">';</script>`)
			require.Contains(t, out, `<link rel="stylesheet" href="keep.css">`)
		}
	}
}
