package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuplicateArtifactPreservesReferencedHistoricalAssetsAfterRefetch(t *testing.T) {
	ro, dir := newTestRouterWithBlobDir(t)
	ctx := t.Context()
	id := createArtifact(t, ro, map[string]any{"title": "Original", "body": "old source"})
	prefix := ro.cfg.RenderOrigin + "/a/" + id + "/assets/"
	const (
		oldImage     = "11111111111111111111111111111111"
		oldStyle     = "22222222222222222222222222222222"
		oldSVG       = "33333333333333333333333333333333"
		unused       = "44444444444444444444444444444444"
		currentImage = "55555555555555555555555555555555"
		currentStyle = "66666666666666666666666666666666"
	)
	imageURL, styleURL := "https://example.com/image.png", "https://example.com/style.css"
	oldCSS := `@import url("` + prefix + oldStyle + `");body{background:url("` + prefix + oldImage + `?size=small#preview")}`
	svg := `<svg><image href="` + prefix + oldImage + `"/></svg>`
	currentCSS := `body{mask-image:url("` + prefix + oldSVG + `")}`
	content := map[string]string{
		oldImage: "\x89PNG\x00old", oldStyle: oldCSS, oldSVG: svg, unused: "unused historical payload",
		currentImage: "\x89PNG\x00current", currentStyle: currentCSS,
	}
	makeAsset := func(id, sourceURL, contentType string) store.ArtifactAsset {
		body := content[id]
		blobID := store.AssetBlobID(1, []byte(body))
		require.NoError(t, ro.putBlob(ctx, blobID, strings.NewReader(body)))
		return store.ArtifactAsset{ID: id, SourceURL: sourceURL, ContentType: contentType, BlobID: blobID, SizeBytes: int64(len(body))}
	}
	_, err := ro.cfg.Store.ReplaceArtifactAssets(ctx, 1, id, "old-generation", []store.ArtifactAsset{
		makeAsset(oldImage, imageURL, "image/png"), makeAsset(oldStyle, styleURL, "text/css"),
		makeAsset(oldSVG, "https://example.com/mask.svg", "image/svg+xml"),
		makeAsset(unused, "https://example.com/unused.bin", "application/octet-stream"),
	})
	require.NoError(t, err)
	widget := `<link rel="stylesheet" href="` + prefix + oldStyle + `"><img src="` + prefix + oldImage + `">`
	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, widget).Code)
	body := `<link rel="stylesheet" href="` + prefix + currentStyle + `"><img src="` + prefix + oldImage + `"><script>fetch('` + imageURL + `')</script>`
	bodyBlob := uuid.NewString()
	require.NoError(t, ro.putBlob(ctx, bodyBlob, strings.NewReader(body)))
	_, err = ro.cfg.Store.ReplaceArtifactSnapshot(ctx, 1, id, store.ArtifactSnapshot{
		SourceBlobID: bodyBlob, GenerationID: "current-generation",
		Assets: []store.ArtifactAsset{makeAsset(currentImage, imageURL, "image/png"), makeAsset(currentStyle, styleURL, "text/css")},
	})
	require.NoError(t, err)
	source, err := ro.cfg.Store.GetArtifact(ctx, 1, id)
	require.NoError(t, err)
	storedWidget, err := ro.readBlobString(ctx, source.WidgetBlobID)
	require.NoError(t, err)
	require.Equal(t, widget, storedWidget, "refetch preserves a widget pointing into the old generation")

	used := storageUsed(t, ro)
	// IDs keep their length; binaries dedupe. Only source, widget and the three
	// textual assets need new bytes. Historical dependencies count toward quota.
	additional := int64(len(body) + len(widget) + len(oldCSS) + len(svg) + len(currentCSS))
	before := quotaBlobContents(t, dir)
	ro.cfg.Entitlements = enforcing(used + additional - 1)
	w := doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/duplicate", duplicateRequest("Copy"))
	writeBytes := additional + int64(len(content[oldImage])+len(content[currentImage]))
	quotaRefusal(t, w, used+additional-1, used, writeBytes, additional)
	assert.Equal(t, before, quotaBlobContents(t, dir))
	ro.cfg.Entitlements = enforcing(used + additional)
	a := duplicateResult(t, ro, id, duplicateRequest("Copy"))
	assert.Equal(t, used+additional, storageUsed(t, ro))
	copied, err := ro.cfg.Store.ListArtifactAssets(ctx, 1, a.ID)
	require.NoError(t, err)
	require.Len(t, copied, 5, "both current assets and three reachable historical assets, without unused history")
	newPrefix := ro.cfg.RenderOrigin + "/a/" + a.ID + "/assets/"
	ids := map[string]string{}
	for _, asset := range copied {
		assert.NotContains(t, content, asset.ID)
		assert.NotEqual(t, "old-generation", asset.GenerationID)
		assert.NotEqual(t, "current-generation", asset.GenerationID)
		switch {
		case asset.SourceURL == imageURL:
			ids[currentImage] = asset.ID
		case asset.SourceURL == styleURL:
			ids[currentStyle] = asset.ID
		case asset.ContentType == "text/css":
			ids[oldStyle] = asset.ID
		case asset.ContentType == "image/svg+xml":
			ids[oldSVG] = asset.ID
		case asset.ContentType == "image/png":
			ids[oldImage] = asset.ID
		default:
			t.Fatalf("unexpected copied asset: %+v", asset)
		}
		if asset.SourceURL != imageURL && asset.SourceURL != styleURL {
			assert.Equal(t, newPrefix+asset.ID, asset.SourceURL, "historical payloads must not override the current runtime manifest")
		}
	}
	require.Len(t, ids, 5)
	var replacements []string
	for oldID, newID := range ids {
		replacements = append(replacements, prefix+oldID, newPrefix+newID)
	}
	rewrite := strings.NewReplacer(replacements...)
	assert.Equal(t, rewrite.Replace(body), getArtifactBody(t, ro, a.ID))
	copiedWidget, err := ro.readBlobString(ctx, a.WidgetBlobID)
	require.NoError(t, err)
	assert.Equal(t, rewrite.Replace(widget), copiedWidget)
	versions, err := ro.cfg.Store.ListVersions(ctx, 1, a.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 1, "dependencies are copied without importing version history")

	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, ro, id).Code)
	for oldID, newID := range ids {
		asset, err := ro.cfg.Store.GetArtifactAssetUnscoped(ctx, a.ID, newID)
		require.NoError(t, err)
		require.NotNil(t, asset, "the copied render URL still resolves after deleting its source")
		stored, err := ro.readBlobString(ctx, asset.BlobID)
		require.NoError(t, err)
		assert.Equal(t, rewrite.Replace(content[oldID]), stored)
	}
	_, err = ro.readBlobString(ctx, store.AssetBlobID(1, []byte(content[unused])))
	assert.Error(t, err, "unreferenced historical assets were not retained by the clone")
}
