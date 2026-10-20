package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/momja/Exhibit/internal/blob"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func duplicateRequest(title string) map[string]any {
	return map[string]any{"title": title, "request_id": uuid.NewString()}
}

func duplicateResult(t *testing.T, ro *Router, source string, request map[string]any) *store.Artifact {
	t.Helper()
	w := doJSON(t, ro, http.MethodPost, "/api/artifacts/"+source+"/duplicate", request)
	require.Contains(t, []int{http.StatusCreated, http.StatusOK}, w.Code, w.Body.String())
	var result store.Artifact
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.NotEmpty(t, result.ID)
	return &result
}

func TestDuplicateArtifactCopiesContentAndRemapsIndependentAssets(t *testing.T) {
	ro, _ := newTestRouterWithBlobDir(t)
	ctx := context.Background()
	id := createArtifact(t, ro, map[string]any{"title": "Original", "body": "old source", "url": "https://example.com/tool"})
	prefix := ro.cfg.RenderOrigin + "/a/" + id + "/assets/"
	css := `body{background:url("` + prefix + `image")}`
	image := "\x89PNG\x00\xffbinary"
	wasm := "\x00asm\x01\x00\x00\x00"
	assets := []store.ArtifactAsset{
		{ID: "image", SourceURL: "https://example.com/image.png", ContentType: "image/png"},
		{ID: "style", SourceURL: "https://example.com/style.css", ContentType: "text/css"},
		{ID: "wasm", SourceURL: "https://example.com/code.wasm", ContentType: "application/wasm"},
	}
	for i, body := range []string{image, css, wasm} {
		assets[i].BlobID = store.AssetBlobID(1, []byte(body))
		assets[i].SizeBytes = int64(len(body))
		require.NoError(t, ro.putBlob(ctx, assets[i].BlobID, strings.NewReader(body)))
	}
	_, err := ro.cfg.Store.ReplaceArtifactAssets(ctx, 1, id, "source-generation", assets)
	require.NoError(t, err)
	body := `<link rel="stylesheet" href="` + prefix + `style"><img src="` + prefix + `image"><p>Current source</p><script>fetch('https://example.com/code.wasm')</script>`
	patchArtifact(t, ro, id, map[string]any{"body": body, "camera_approved": true, "network_allowlist": []string{"https://example.com"}})
	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, `<img src="`+prefix+`image">`).Code)
	used := storageUsed(t, ro)
	// Fresh asset IDs are 32 characters; these fixture IDs have 5. Source,
	// widget and CSS each grow to hold the remapped URLs. Binary bytes dedupe.
	additional := int64(len(body) + 54 + len(`<img src="`+prefix+`image">`) + 27 + len(css) + 27)
	ro.cfg.Entitlements = enforcing(used + additional)
	a := duplicateResult(t, ro, id, duplicateRequest("A new title"))
	assert.Equal(t, used+additional, storageUsed(t, ro))
	assert.Equal(t, "A new title", a.Title)
	assert.Equal(t, "https://example.com/tool", a.SourceURL)
	assert.True(t, a.CameraApproved)
	assert.Equal(t, []string{"https://example.com"}, a.NetworkAllowlist)
	assert.Equal(t, 1, a.VersionSeq)
	copied, err := ro.cfg.Store.ListArtifactAssets(ctx, 1, a.ID)
	require.NoError(t, err)
	require.Len(t, copied, 3)
	ids := map[string]string{}
	for _, asset := range copied {
		ids[asset.SourceURL] = asset.ID
		assert.Equal(t, a.ID, asset.ArtifactID)
		assert.NotEqual(t, "source-generation", asset.GenerationID)
		assert.NotContains(t, []string{"image", "style", "wasm"}, asset.ID)
	}
	newPrefix := ro.cfg.RenderOrigin + "/a/" + a.ID + "/assets/"
	copiedBody := getArtifactBody(t, ro, a.ID)
	assert.NotContains(t, copiedBody, prefix)
	assert.Contains(t, copiedBody, newPrefix+ids["https://example.com/style.css"])
	assert.Contains(t, copiedBody, newPrefix+ids["https://example.com/image.png"])
	widget, err := ro.readBlobString(ctx, a.WidgetBlobID)
	require.NoError(t, err)
	assert.Equal(t, `<img src="`+newPrefix+ids["https://example.com/image.png"]+`">`, widget)
	for _, asset := range copied {
		content, err := ro.readBlobString(ctx, asset.BlobID)
		require.NoError(t, err)
		switch asset.ContentType {
		case "text/css":
			assert.Equal(t, `body{background:url("`+newPrefix+ids["https://example.com/image.png"]+`")}`, content)
		case "image/png":
			assert.Equal(t, image, content)
		case "application/wasm":
			assert.Equal(t, wasm, content)
		}
	}
	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, ro, id).Code)
	assert.Equal(t, copiedBody, getArtifactBody(t, ro, a.ID))
	for _, asset := range copied {
		_, err := ro.readBlobString(ctx, asset.BlobID)
		require.NoError(t, err, "deleting original must keep all clone bytes")
	}
	patchArtifact(t, ro, a.ID, map[string]any{"body": "Changed copy"})
	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, ro, a.ID).Code)
}

func TestDuplicateArtifactOptionsValidationAndOwnerBoundaries(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()
	id := createArtifact(t, ro, map[string]any{"title": "Original", "body": "Source"})
	require.NoError(t, ro.cfg.Store.SetState(ctx, 1, id, 1, "mine", "owner"))
	require.NoError(t, ro.cfg.Store.SetState(ctx, 1, id, 2, "private", "viewer"))
	require.NoError(t, ro.cfg.Store.UpdateArtifact(ctx, 1, id, map[string]any{"share_state_mode": "shared"}))
	require.NoError(t, ro.cfg.Store.CreateShare(ctx, 1, &store.Share{ID: "original-share", ArtifactID: id}))
	for _, copySettings := range []bool{false, true} {
		for _, copyData := range []bool{false, true} {
			req := duplicateRequest(" Copy ")
			req["copy_data"], req["copy_share_settings"] = copyData, copySettings
			a := duplicateResult(t, ro, id, req)
			assert.Equal(t, "Copy", a.Title)
			state, err := ro.cfg.Store.GetState(ctx, 1, a.ID, 1)
			require.NoError(t, err)
			if copyData {
				assert.Equal(t, map[string]string{"mine": "owner"}, state)
			} else {
				assert.Empty(t, state)
			}
			state, err = ro.cfg.Store.GetState(ctx, 1, a.ID, 2)
			require.NoError(t, err)
			assert.Empty(t, state)
			shares, err := ro.cfg.Store.ListArtifactShares(ctx, 1, a.ID)
			require.NoError(t, err)
			if copySettings {
				require.Len(t, shares, 1)
				assert.NotEqual(t, "original-share", shares[0].ID)
				assert.Equal(t, "shared", a.ShareStateMode)
			} else {
				assert.Empty(t, shares)
				assert.Equal(t, "own", a.ShareStateMode)
			}
		}
	}
	for _, req := range []map[string]any{{"title": "Copy"}, {"title": "Copy", "request_id": "bad"}, {"title": " ", "request_id": uuid.NewString()}, {"title": "Copy", "request_id": uuid.NewString(), "copy_data": "yes"}} {
		w := doJSON(t, ro, "POST", "/api/artifacts/"+id+"/duplicate", req)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	foreignID := seedForeignArtifact(t, ro)
	for _, sourceID := range []string{foreignID, "missing"} {
		w := doJSON(t, ro, "POST", "/api/artifacts/"+sourceID+"/duplicate", duplicateRequest("Copy"))
		assert.Equal(t, http.StatusNotFound, w.Code)
	}
	regRouter, reg := newScopedTestRouter(t)
	scopedID := createOwnArtifact(t, regRouter)
	grant, err := reg.Issue(1, scopedID, "")
	require.NoError(t, err)
	w := doWithToken(t, regRouter, "POST", "/api/artifacts/"+scopedID+"/duplicate", grant.Token(), duplicateRequest("Copy"))
	assert.Equal(t, http.StatusForbidden, w.Code, "agent credentials cannot clone sharing policy or expand their scope")
}

func TestDuplicateArtifactQuotaAndIdempotentRetries(t *testing.T) {
	ro, dir := newTestRouterWithBlobDir(t)
	id := createArtifact(t, ro, map[string]any{"body": "source", "title": "Original"})
	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, "widget").Code)
	used := storageUsed(t, ro)
	before := quotaBlobContents(t, dir)
	ro.cfg.Entitlements = enforcing(used + 11)
	req := duplicateRequest("Copy")
	w := doJSON(t, ro, "POST", "/api/artifacts/"+id+"/duplicate", req)
	quotaRefusal(t, w, used+11, used, 12, 12)
	assert.Equal(t, before, quotaBlobContents(t, dir), "quota refusal must stage nothing")
	ro.cfg.Entitlements = enforcing(used + 12)
	a := duplicateResult(t, ro, id, req)
	assert.Equal(t, used+12, storageUsed(t, ro))
	before = quotaBlobContents(t, dir)
	// A replay is successful at the now-full quota and after original deletion.
	replay := duplicateResult(t, ro, id, req)
	assert.Equal(t, a.ID, replay.ID)
	assert.Equal(t, before, quotaBlobContents(t, dir))
	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, ro, id).Code)
	replay = duplicateResult(t, ro, id, req)
	assert.Equal(t, a.ID, replay.ID)
}

type failingDuplicateStore struct {
	store.Store
	afterCommit bool
}

func (s failingDuplicateStore) DuplicateArtifact(ctx context.Context, ownerID int64, sourceID string, d store.ArtifactDuplicate) (bool, error) {
	if s.afterCommit {
		if _, err := s.Store.DuplicateArtifact(ctx, ownerID, sourceID, d); err != nil {
			return false, err
		}
	}
	return false, errors.New("simulated duplicate failure")
}

func TestDuplicateArtifactFailurePublishesNothingAndReclaimsStaging(t *testing.T) {
	for _, failure := range []string{"transaction", "blob", "missing body", "lost acknowledgement"} {
		t.Run(failure, func(t *testing.T) {
			ro, dir := newTestRouterWithBlobDir(t)
			id := createArtifact(t, ro, map[string]any{"body": "source", "title": "Original"})
			require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, "widget").Code)
			realStore, realBlob := ro.cfg.Store, ro.cfg.Blob
			switch failure {
			case "transaction":
				ro.cfg.Store = failingDuplicateStore{Store: realStore}
			case "lost acknowledgement":
				ro.cfg.Store = failingDuplicateStore{Store: realStore, afterCommit: true}
			case "blob":
				ro.cfg.Blob = failingDuplicateBlob{Store: realBlob}
			case "missing body":
				a, err := ro.cfg.Store.GetArtifact(context.Background(), 1, id)
				require.NoError(t, err)
				require.NoError(t, ro.cfg.Blob.Delete(context.Background(), a.SourceBlobID))
			}
			before := quotaBlobContents(t, dir)
			req := duplicateRequest("Copy")
			w := doJSON(t, ro, "POST", "/api/artifacts/"+id+"/duplicate", req)
			ro.cfg.Store, ro.cfg.Blob = realStore, realBlob
			arts, err := ro.cfg.Store.ListArtifacts(context.Background(), store.ListOptions{OwnerID: 1})
			require.NoError(t, err)
			if failure == "lost acknowledgement" {
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				require.Len(t, arts, 2)
				duplicateResult(t, ro, id, req)
			} else {
				require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
				assert.Len(t, arts, 1)
				assert.Equal(t, before, quotaBlobContents(t, dir), "failed clone cannot leak staged blobs or delete originals")
			}
		})
	}
}

func TestDuplicateGalleryModalAndRefreshPartial(t *testing.T) {
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"title": "Original", "body": "source"})
	page := getPage(t, ro, "/")
	assert.Contains(t, page, `data-action-menu-event-param="exhibit:duplicate"`)
	assert.Contains(t, page, `&#34;artifactID&#34;:&#34;`+id+`&#34;`)
	assert.Contains(t, page, `<dialog class="duplicate-dialog"`)
	for _, name := range []string{"title", "copy_data", "copy_share_settings"} {
		assert.Contains(t, page, `name="`+name+`"`)
	}
	assert.Contains(t, page, `name="copy_data" type="checkbox">`)
	assert.Contains(t, page, `name="copy_share_settings" type="checkbox">`)
	a := duplicateResult(t, ro, id, duplicateRequest("Renamed copy"))
	fragment := getPage(t, ro, "/partials/gallery-grid")
	assert.Contains(t, fragment, `data-href="/artifacts/`+a.ID+`"`)
	assert.Contains(t, fragment, "Renamed copy")
	assert.NotContains(t, fragment, "<html")
}

type failingDuplicateBlob struct{ blob.Store }

func (b failingDuplicateBlob) Put(context.Context, string, io.Reader) error {
	return errors.New("simulated blob failure")
}

func TestDuplicateArtifactReusesExistingAssetBlobsWithoutRewriting(t *testing.T) {
	ro, dir := newTestRouterWithBlobDir(t)
	ctx := context.Background()
	id := createArtifact(t, ro, map[string]any{"body": "source", "title": "Original"})
	payload := "\x00asm\x01\x00\x00\x00"
	blobID := store.AssetBlobID(1, []byte(payload))
	require.NoError(t, ro.putBlob(ctx, blobID, strings.NewReader(payload)))
	_, err := ro.cfg.Store.ReplaceArtifactAssets(ctx, 1, id, "original-generation", []store.ArtifactAsset{{ID: "binary", BlobID: blobID, ContentType: "application/wasm", SizeBytes: int64(len(payload)), SourceURL: "https://example.com/tool.wasm"}})
	require.NoError(t, err)
	used := storageUsed(t, ro)
	ro.cfg.Entitlements = enforcing(used + 6)
	// Any attempt to rewrite the existing binary would fail. The copy only adds
	// references, so its independent source blob is the whole additional charge.
	ro.cfg.Blob = failingAssetBlob{Store: ro.cfg.Blob}
	a := duplicateResult(t, ro, id, duplicateRequest("Copy"))
	assert.Equal(t, used+6, storageUsed(t, ro))
	before := quotaBlobContents(t, dir)
	assert.Equal(t, payload, before[blobID])
	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, ro, id).Code)
	stored, err := ro.readBlobString(ctx, blobID)
	require.NoError(t, err)
	assert.Equal(t, payload, stored)
	assert.Equal(t, "source", getArtifactBody(t, ro, a.ID))
}
