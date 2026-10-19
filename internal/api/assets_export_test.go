package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getAssetsExport(r *Router, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/artifacts/"+id+"/assets/export", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func seedExportAssets(t *testing.T, r *Router, id string) ([]store.ArtifactAsset, [][]byte) {
	t.Helper()
	// Same source filename, different URLs and bytes. Include nulls and invalid
	// UTF-8: the ZIP must preserve binary bytes rather than round-tripping text.
	bodies := [][]byte{{0, 97, 115, 109, 255}, {0, 254, 13, 10, 128}}
	assets := []store.ArtifactAsset{
		{ID: "asset-one", BlobID: "export-one", SourceURL: "https://example.test/a/app.wasm?token=one", ContentType: "application/wasm", SizeBytes: 5},
		{ID: "asset-two", BlobID: "export-two", SourceURL: "https://example.test/b/app.wasm?token=two", ContentType: "application/octet-stream", SizeBytes: 5},
	}
	for i, a := range assets {
		require.NoError(t, r.cfg.Blob.Put(t.Context(), a.BlobID, bytes.NewReader(bodies[i])))
	}
	_, err := r.cfg.Store.ReplaceArtifactAssets(t.Context(), defaultOwnerID, id, "export-generation", assets)
	require.NoError(t, err)
	return assets, bodies
}

func readAssetsZIP(t *testing.T, w *httptest.ResponseRecorder) (assetsManifest, map[string][]byte) {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/zip", w.Header().Get("Content-Type"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	archive, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)
	files := make(map[string][]byte)
	for _, file := range archive.File {
		require.True(t, fs.ValidPath(file.Name), file.Name)
		body, err := file.Open()
		require.NoError(t, err)
		files[file.Name], err = io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, body.Close())
	}
	var manifest assetsManifest
	require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
	return manifest, files
}

func TestExportAssetsPreservesBytesAndMetadata(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Binary assets")
	assets, bodies := seedExportAssets(t, r, id)
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	w := getAssetsExport(r, id)
	manifest, files := readAssetsZIP(t, w)
	assert.Contains(t, w.Header().Get("Content-Disposition"), id+"-assets.zip")
	assert.Equal(t, id, manifest.ArtifactID)
	require.Len(t, manifest.Assets, 2)
	assert.Len(t, files, 3)
	for i, asset := range manifest.Assets {
		assert.Equal(t, assets[i].SourceURL, asset.SourceURL)
		assert.Equal(t, assets[i].ContentType, asset.ContentType)
		assert.Equal(t, assets[i].SizeBytes, asset.SizeBytes)
		assert.Equal(t, "export-generation", asset.Generation)
		assert.Equal(t, bodies[i], files[asset.Path])
		assert.Equal(t, "assets/"+assets[i].ID+"/app.wasm", asset.Path)
	}
	remaining, err := os.ReadDir(temp)
	require.NoError(t, err)
	assert.Empty(t, remaining, "the completed archive is removed from temporary storage")
	assert.NotContains(t, string(files["manifest.json"]), "export-one", "internal blob ids stay internal")
}

func TestExportAssetsWithNoAssets(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "No assets")
	manifest, files := readAssetsZIP(t, getAssetsExport(r, id))
	assert.Equal(t, id, manifest.ArtifactID)
	assert.NotNil(t, manifest.Assets)
	assert.Empty(t, manifest.Assets)
	assert.Len(t, files, 1)
}

func TestExportAssetsMissingPayloadFailsWithoutPartialDownload(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Missing asset")
	assets, _ := seedExportAssets(t, r, id)
	require.NoError(t, r.cfg.Blob.Delete(t.Context(), assets[1].BlobID))
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	w := getAssetsExport(r, id)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Empty(t, w.Header().Get("Content-Disposition"))
	assert.NotContains(t, w.Header().Get("Content-Type"), "zip")
	remaining, err := os.ReadDir(temp)
	require.NoError(t, err)
	assert.Empty(t, remaining, "a failed archive is removed too")
}

func TestExportAssetsRefusesUnknownAndForeignArtifacts(t *testing.T) {
	r := newTestRouter(t)
	foreign := seedForeignArtifact(t, r)
	for _, id := range []string{"does-not-exist", foreign} {
		assert.Equal(t, http.StatusNotFound, getAssetsExport(r, id).Code)
	}
}

func TestAssetArchivePathsStayLocal(t *testing.T) {
	for _, source := range []string{
		"https://example.test/../../etc/passwd", "https://example.test/%2fetc%2fpasswd",
		"https://example.test/..%5c..%5cevil", "https://example.test/", "", ":invalid",
		"https://example.test/" + strings.Repeat("x", 300),
	} {
		name := assetArchivePath(store.ArtifactAsset{ID: "opaque-id", SourceURL: source})
		assert.True(t, fs.ValidPath(name), name)
		assert.Equal(t, "assets/opaque-id", path.Dir(name))
		assert.LessOrEqual(t, len(path.Base(name)), 80)
	}
}
