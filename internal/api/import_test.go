package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postImport(r *Router, id, kind string, body []byte, replace bool) *httptest.ResponseRecorder {
	path := "/api/artifacts/" + id + "/" + kind + "/import"
	if replace {
		path += "?replace=true"
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestImportStateReplacesStringsAndPreservesOtherViewer(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "State import")
	payload := []byte(`{"":"","__proto__":"safe","../a?b#c":"héllo\n世界","todos":"[1, 2]"}`)
	w := postImport(r, id, "state", payload, false)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	var want map[string]string
	require.NoError(t, json.Unmarshal(payload, &want))
	got, err := r.cfg.Store.GetStateAsViewer(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	require.NoError(t, r.cfg.Store.SetState(t.Context(), 1, id, 9, "theirs", "untouched"))
	w = postImport(r, id, "state", []byte(`{"replacement":"true"}`), false)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"confirmation_required":true`)
	got, err = r.cfg.Store.GetStateAsViewer(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	w = postImport(r, id, "state", []byte(`{"replacement":"true"}`), true)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	got, err = r.cfg.Store.GetStateAsViewer(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"replacement": "true"}, got)
	other, err := r.cfg.Store.GetState(t.Context(), 1, id, 9)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"theirs": "untouched"}, other)
	w = postImport(r, id, "state", []byte(`{}`), false)
	require.Equal(t, http.StatusConflict, w.Code)
	w = postImport(r, id, "state", []byte(`{}`), true)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = postImport(r, id, "state", []byte(`{}`), false)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestImportStateRejectsInvalidFilesBeforeConfirmation(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Invalid state")
	require.NoError(t, r.cfg.Store.SetStateAsViewer(t.Context(), 1, id, "saved", "keep"))
	for _, body := range []string{`null`, `[]`, `true`, `{"x":1}`, `{"x":null}`, `{"x":false}`, `{"x":[]}`, `{"x":{}}`, `{"x":"a"} {}`, `{"x":`} {
		t.Run(body, func(t *testing.T) {
			w := postImport(r, id, "state", []byte(body), true)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			got, err := r.cfg.Store.GetStateAsViewer(t.Context(), 1, id)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"saved": "keep"}, got)
		})
	}
}

func TestImportAssetsRoundTripAndEmptyReplacement(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Asset import")
	original, _ := seedExportAssets(t, r, id)
	exported := getAssetsExport(r, id).Body.Bytes()
	w := postImport(r, id, "assets", exported, false)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	w = postImport(r, id, "assets", exported, true)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	_, want := readAssetsZIP(t, getAssetsExport(r, id))
	delete(want, "manifest.json")
	// The ids remain usable by markup; bytes and metadata survive a second export.
	got, err := r.cfg.Store.ListArtifactAssets(t.Context(), 1, id)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, original[0].ID, got[0].ID)
	empty := assetsManifest{ArtifactID: id, Assets: []exportedAsset{}}
	emptyZIP := makeImportZIP(t, empty, map[string][]byte{})
	w = postImport(r, id, "assets", emptyZIP, false)
	require.Equal(t, http.StatusConflict, w.Code)
	w = postImport(r, id, "assets", emptyZIP, true)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	got, err = r.cfg.Store.ListArtifactAssets(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Empty(t, got)
	w = postImport(r, id, "assets", exported, false)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	_, actual := readAssetsZIP(t, getAssetsExport(r, id))
	delete(actual, "manifest.json")
	assert.Equal(t, want, actual)
}

func makeImportZIP(t *testing.T, manifest assetsManifest, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	entry, err := archive.Create("manifest.json")
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(entry).Encode(manifest))
	for name, body := range files {
		entry, err := archive.Create(name)
		require.NoError(t, err)
		_, err = entry.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	return buf.Bytes()
}

func TestImportAssetsInvalidArchiveLeavesExistingData(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Invalid assets")
	original, _ := seedExportAssets(t, r, id)
	manifest, files := readAssetsZIP(t, getAssetsExport(r, id))
	delete(files, "manifest.json")
	cases := map[string]func(*assetsManifest, map[string][]byte){
		"missing payload":     func(m *assetsManifest, f map[string][]byte) { delete(f, m.Assets[1].Path) },
		"wrong size":          func(m *assetsManifest, f map[string][]byte) { m.Assets[1].SizeBytes++ },
		"wrong artifact":      func(m *assetsManifest, f map[string][]byte) { m.ArtifactID = "another" },
		"duplicate URL":       func(m *assetsManifest, f map[string][]byte) { m.Assets[1].SourceURL = m.Assets[0].SourceURL },
		"duplicate ID":        func(m *assetsManifest, f map[string][]byte) { m.Assets[1].ID = m.Assets[0].ID },
		"traversal":           func(m *assetsManifest, f map[string][]byte) { f["../outside"] = []byte("bad") },
		"extra payload":       func(m *assetsManifest, f map[string][]byte) { f["extra"] = []byte("bad") },
		"unsafe content type": func(m *assetsManifest, f map[string][]byte) { m.Assets[0].ContentType = "text/html\r\nX: bad" },
		"immutable ID":        func(m *assetsManifest, f map[string][]byte) { f[m.Assets[1].Path] = []byte("other") },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := manifest
			m.Assets = append([]exportedAsset{}, manifest.Assets...)
			f := map[string][]byte{}
			for k, v := range files {
				f[k] = v
			}
			change(&m, f)
			w := postImport(r, id, "assets", makeImportZIP(t, m, f), true)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			got, err := r.cfg.Store.ListArtifactAssets(t.Context(), 1, id)
			require.NoError(t, err)
			require.Len(t, got, 2)
			assert.Equal(t, original[0].BlobID, got[0].BlobID)
			_, after := readAssetsZIP(t, getAssetsExport(r, id))
			for k, v := range files {
				assert.Equal(t, v, after[k])
			}
		})
	}
	for _, body := range [][]byte{[]byte("invalid zip"), makeImportZIP(t, assetsManifest{}, nil)} {
		assert.Equal(t, http.StatusBadRequest, postImport(r, id, "assets", body, true).Code)
	}
}

func TestImportsRefuseUnknownAndForeignArtifacts(t *testing.T) {
	r := newTestRouter(t)
	foreign := seedForeignArtifact(t, r)
	for _, id := range []string{foreign, "does-not-exist"} {
		for _, kind := range []string{"state", "assets"} {
			assert.Equal(t, http.StatusNotFound, postImport(r, id, kind, []byte(`{}`), true).Code)
		}
	}
}

func TestImportControlsAndAssetPartial(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Import controls")
	page := httptest.NewRecorder()
	r.ServeHTTP(page, httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil))
	require.Equal(t, http.StatusOK, page.Code)
	for _, s := range []string{`id="state-import"`, `id="assets-import"`, `id="state-import-file"`, `id="assets-import-file"`, `type="module" src="/assets/gallery/storage-import.js"`, `type="module" src="/assets/gallery/assets.js"`} {
		assert.Contains(t, page.Body.String(), s)
	}
	seedExportAssets(t, r, id)
	partial := httptest.NewRecorder()
	r.ServeHTTP(partial, httptest.NewRequest("GET", "/partials/asset-panel?artifact="+id, nil))
	require.Equal(t, http.StatusOK, partial.Code, partial.Body.String())
	assert.Contains(t, partial.Body.String(), `data-delete-asset="asset-one"`)
	assert.Contains(t, partial.Body.String(), "2 assets")
}

func TestImportAssetsQuotaAndBlobFailurePreserveData(t *testing.T) {
	for _, operation := range []string{"quota", "blob"} {
		t.Run(operation, func(t *testing.T) {
			r, dir := newTestRouterWithBlobDir(t)
			id := createTestArtifact(t, r, "Failed import")
			seedExportAssets(t, r, id)
			manifest, files := readAssetsZIP(t, getAssetsExport(r, id))
			delete(files, "manifest.json")
			// New ids and bytes form a valid replacement, rather than altering the
			// bytes behind an immutable existing asset URL.
			for i := range manifest.Assets {
				a := &manifest.Assets[i]
				oldPath := a.Path
				a.ID = "new-" + a.ID
				a.Path = "assets/" + a.ID + "/payload"
				files[a.Path] = []byte("other")
				delete(files, oldPath)
			}
			before := quotaBlobContents(t, dir)
			if operation == "quota" {
				r.cfg.Entitlements = enforcing(storageUsed(t, r))
			} else {
				r.cfg.Blob = failingAssetBlob{r.cfg.Blob}
			}
			w := postImport(r, id, "assets", makeImportZIP(t, manifest, files), true)
			if operation == "quota" {
				assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
			} else {
				assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
			}
			after, err := r.cfg.Store.ListArtifactAssets(t.Context(), 1, id)
			require.NoError(t, err)
			require.Len(t, after, 2)
			assert.Equal(t, "asset-one", after[0].ID)
			assert.Equal(t, before, quotaBlobContents(t, dir), "no staged payloads survive a failed import")
		})
	}
}

func TestImportZIPRejectsCRCFailureAndExpandedOverrun(t *testing.T) {
	manifest := assetsManifest{ArtifactID: "artifact", Assets: []exportedAsset{{assetView: assetView{ID: "asset", SourceURL: "https://example.test/asset", ContentType: "application/wasm", SizeBytes: 5}, Path: "assets/asset/payload"}}}
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	entry, err := archive.Create("manifest.json")
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(entry).Encode(manifest))
	entry, err = archive.CreateRaw(&zip.FileHeader{Name: "assets/asset/payload", Method: zip.Store, CompressedSize64: 5, UncompressedSize64: 5, CRC32: 0})
	require.NoError(t, err)
	_, err = entry.Write([]byte("bytes"))
	require.NoError(t, err)
	require.NoError(t, archive.Close())
	_, err = readAssetsImport(buf.Bytes(), "artifact")
	require.ErrorContains(t, err, "checksum")
	_, err = readImportEntry(&zip.File{FileHeader: zip.FileHeader{Name: "oversized", UncompressedSize64: 49 << 20}}, 48<<20)
	require.ErrorContains(t, err, "size limit")
}
