package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This deliberately crosses both account and instance boundaries, and retains
// a widget's historical dependencies after a source refetch. Export and import
// unit fixtures alone cannot demonstrate that their formats agree.
func TestArtifactBundleRoundTripAcrossInstances(t *testing.T) {
	source := newTestRouter(t)
	destination := newTestRouter(t)
	destination.cfg.RenderOrigin = "https://destination-render.example"
	destination.cfg.ArtifactImportAdminToken = "private-import-credential"
	ctx := t.Context()
	_, err := destination.cfg.Store.UpsertUser(ctx, "other-account", "other@example.test")
	require.NoError(t, err)
	account, err := destination.cfg.Store.UpsertUser(ctx, "target-account", "target@example.test")
	require.NoError(t, err)
	require.NotEqual(t, int64(defaultOwnerID), account.ID)
	id := createTestArtifact(t, source, "Portable tool")
	prefix := source.cfg.RenderOrigin + "/a/" + id + "/assets/"
	const oldImageID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const newImageID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const styleID = "cccccccccccccccccccccccccccccccc"
	oldImage, newImage := "\x89PNG\x00old\xff", "\x89PNG\x00new\xfe"
	css := `body{background:url("` + prefix + oldImageID + `?size=small#preview")}`
	asset := func(assetID, sourceURL, contentType, content string) store.ArtifactAsset {
		blobID := store.AssetBlobID(defaultOwnerID, []byte(content))
		require.NoError(t, source.putBlob(ctx, blobID, strings.NewReader(content)))
		return store.ArtifactAsset{ID: assetID, SourceURL: sourceURL, ContentType: contentType, BlobID: blobID, SizeBytes: int64(len(content))}
	}
	imageURL := "https://upstream.example/image.png"
	_, err = source.cfg.Store.ReplaceArtifactAssets(ctx, defaultOwnerID, id, "old-generation", []store.ArtifactAsset{
		asset(oldImageID, imageURL, "image/png", oldImage),
		asset(styleID, "https://upstream.example/style.css", "text/css", css),
	})
	require.NoError(t, err)
	widget := `<html><head><link rel="stylesheet" href="` + prefix + styleID + `"></head><body>Widget<img src="` + prefix + oldImageID + `"></body></html>`
	require.Equal(t, http.StatusOK, putWidgetReq(t, source, id, widget).Code)
	body := `<html><head></head><body>Source<img src="` + prefix + newImageID + `"><script>fetch('` + imageURL + `')</script></body></html>`
	bodyBlob := uuid.NewString()
	require.NoError(t, source.putBlob(ctx, bodyBlob, strings.NewReader(body)))
	_, err = source.cfg.Store.ReplaceArtifactSnapshot(ctx, defaultOwnerID, id, store.ArtifactSnapshot{
		SourceBlobID: bodyBlob, GenerationID: "new-generation",
		Assets: []store.ArtifactAsset{asset(newImageID, imageURL, "image/png", newImage)},
	})
	require.NoError(t, err)
	state := map[string]string{"": "", "__proto__": "safe", "todos": "[1, 2]", "unicode": "héllo\n世界"}
	for key, value := range state {
		require.NoError(t, source.cfg.Store.SetState(ctx, store.OwnerID(defaultOwnerID), id, store.ViewerID(defaultOwnerID), key, value))
	}
	require.NoError(t, source.cfg.Store.SetState(ctx, store.OwnerID(defaultOwnerID), id, 99, "private-viewer-data", "must stay behind"))
	require.NoError(t, source.cfg.Store.UpdateArtifact(ctx, defaultOwnerID, id, map[string]any{
		"network_allowlist": []string{"https://upstream.example"}, "downloads_approved": true,
		"clipboard_approved": true, "links_approved": true, "camera_approved": true,
		"microphone_approved": true, "geolocation_approved": true, "share_state_mode": store.ShareStateShared,
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+id+"/export", nil)
	request.Header.Set("Authorization", authHeader())
	exported := httptest.NewRecorder()
	source.ServeHTTP(exported, request)
	require.Equal(t, http.StatusOK, exported.Code, exported.Body.String())
	require.Equal(t, "application/zip", exported.Header().Get("Content-Type"))
	// The originating instance can disappear before import starts.
	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, source, id).Code)

	request = httptest.NewRequest(http.MethodPost, "/api/admin/users/"+strconv.FormatInt(account.ID, 10)+"/artifacts/import", bytes.NewReader(exported.Body.Bytes()))
	request.Header.Set("Authorization", "Bearer "+destination.cfg.ArtifactImportAdminToken)
	request.Header.Set("Content-Type", "application/zip")
	response := httptest.NewRecorder()
	destination.ServeHTTP(response, request)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var imported store.Artifact
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &imported))
	require.NotEmpty(t, imported.ID)
	assert.NotEqual(t, id, imported.ID)
	assert.Equal(t, account.ID, imported.OwnerID)
	assert.Equal(t, "Portable tool", imported.Title)
	assert.Empty(t, imported.NetworkAllowlist)
	assert.False(t, imported.DownloadsApproved || imported.ClipboardApproved || imported.LinksApproved || imported.CameraApproved || imported.MicrophoneApproved || imported.GeolocationApproved)
	assert.Equal(t, store.ShareStateOwn, imported.ShareStateMode)
	assert.Zero(t, imported.ShareGrantCount)
	assert.False(t, imported.SharePublicLink)
	assets, err := destination.cfg.Store.ListArtifactAssets(ctx, account.ID, imported.ID)
	require.NoError(t, err)
	require.Len(t, assets, 3)
	newPrefix := destination.cfg.RenderOrigin + "/a/" + imported.ID + "/assets/"
	var replacements []string
	for _, saved := range assets {
		assert.NotContains(t, []string{oldImageID, newImageID, styleID}, saved.ID)
		content, err := destination.readBlobString(ctx, saved.BlobID)
		require.NoError(t, err)
		switch {
		case content == oldImage:
			replacements = append(replacements, prefix+oldImageID, newPrefix+saved.ID)
			assert.NotEqual(t, imageURL, saved.SourceURL, "historical bytes cannot shadow the current runtime URL")
		case content == newImage:
			replacements = append(replacements, prefix+newImageID, newPrefix+saved.ID)
			assert.Equal(t, imageURL, saved.SourceURL)
		case saved.ContentType == "text/css":
			replacements = append(replacements, prefix+styleID, newPrefix+saved.ID)
			assert.NotContains(t, content, source.cfg.RenderOrigin)
		default:
			t.Fatalf("unexpected imported asset: %+v", saved)
		}
	}
	rewrite := strings.NewReplacer(replacements...)
	importedBody, err := destination.readBlobString(ctx, imported.SourceBlobID)
	require.NoError(t, err)
	assert.Equal(t, rewrite.Replace(body), importedBody)
	importedWidget, err := destination.readBlobString(ctx, imported.WidgetBlobID)
	require.NoError(t, err)
	assert.Equal(t, rewrite.Replace(widget), importedWidget)
	for _, saved := range assets {
		if saved.ContentType == "text/css" {
			content, err := destination.readBlobString(ctx, saved.BlobID)
			require.NoError(t, err)
			assert.Equal(t, rewrite.Replace(css), content)
		}
	}
	gotState, err := destination.cfg.Store.GetState(ctx, store.OwnerID(account.ID), imported.ID, store.ViewerID(account.ID))
	require.NoError(t, err)
	assert.Equal(t, state, gotState)
	versions, err := destination.cfg.Store.ListVersions(ctx, account.ID, imported.ID)
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, imported.SourceBlobID, versions[0].BodyBlobID)
	foreign, err := destination.cfg.Store.GetArtifact(ctx, defaultOwnerID, imported.ID)
	require.NoError(t, err)
	assert.Nil(t, foreign)
}
