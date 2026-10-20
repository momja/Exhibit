package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bundleImportTestToken = "dedicated-private-import-token"

func artifactBundleFixture() (artifactBundleManifest, map[string][]byte) {
	m := artifactBundleManifest{Format: artifactBundleFormat, Version: artifactBundleVersion, ArtifactID: "old-artifact",
		RenderOrigin: "https://old-render.example", EntryPage: "main", StatePath: "state.json",
		Artifact: artifactBundleMetadata{Title: "Editable imported tool", SourceURL: "https://example.com/tool", Tier: store.Tier2,
			NetworkAllowlist: []string{"https://allowed.example"}, DownloadsApproved: true, ClipboardApproved: true, LinksApproved: true,
			CameraApproved: true, MicrophoneApproved: true, GeolocationApproved: true, ShareStateMode: store.ShareStateShared},
		Pages:  []artifactBundlePage{{ID: "main", Path: "pages/main.html", SourcePath: "source/main.html"}},
		Widget: &artifactBundlePage{ID: "widget", Path: "pages/widget.html", SourcePath: "source/widget.html"},
		Assets: []exportedAsset{},
	}
	prefix := "/a/old-artifact/assets/"
	f := map[string][]byte{
		"pages/main.html":    []byte("<html>Portable materialization: never restore this</html>"),
		"source/main.html":   []byte(`<html><h1>editable source</h1><img src="` + m.RenderOrigin + prefix + `image"><script>fetch('` + prefix + `image-long')</script></html>`),
		"pages/widget.html":  []byte("<html>Portable widget: never restore this</html>"),
		"source/widget.html": []byte(`<link rel="stylesheet" href="` + m.RenderOrigin + prefix + `css">`),
		"state.json":         []byte(`{"":"","__proto__":"literal","todos":"[1,  2]","unicode":"世界\n","url":"https://old-render.example/a/old-artifact/assets/image"}`),
	}
	for _, asset := range []struct{ id, source, contentType, body string }{
		{"image", "https://cdn.example/image.png", "image/png", "\x89PNG\x00binary"},
		{"image-long", "https://cdn.example/image.png", "image/png", "\x89PNG\x00binary"},
		{"css", m.RenderOrigin + prefix + "css", "text/css", `/*theme*/body{background:url('` + m.RenderOrigin + prefix + `image')}/* ` + prefix + `image-long */`},
	} {
		path := "assets/" + asset.id + "/payload"
		f[path] = []byte(asset.body)
		m.Assets = append(m.Assets, exportedAsset{assetView: assetView{ID: asset.id, SourceURL: asset.source,
			ContentType: asset.contentType, SizeBytes: int64(len(asset.body)), Generation: "old-generation"}, Path: path})
	}
	return m, f
}

func makeArtifactBundleZIP(t *testing.T, manifest any, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	write := func(name string, body []byte) {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		require.NoError(t, err)
		_, err = entry.Write(body)
		require.NoError(t, err)
	}
	body, err := json.Marshal(manifest)
	require.NoError(t, err)
	write("manifest.json", body)
	for name, body := range files {
		write(name, body)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func postArtifactBundle(ro *Router, ownerID int64, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+strconv.FormatInt(ownerID, 10)+"/artifacts/import", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+bundleImportTestToken)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, r)
	return w
}

func enableBundleImport(t *testing.T, ro *Router) *store.User {
	t.Helper()
	ro.cfg.ArtifactImportAdminToken = bundleImportTestToken
	u, err := ro.cfg.Store.UpsertUser(t.Context(), "import-owner", "owner@example.test")
	require.NoError(t, err)
	return u
}

func TestArtifactBundleImportRestoresSourceWithFreshIdentitiesAndExactOwnerState(t *testing.T) {
	ro := newTestRouter(t)
	u := enableBundleImport(t, ro)
	ro.cfg.RenderOrigin = "https://new-render.example"
	other, err := ro.cfg.Store.UpsertUser(t.Context(), "other-owner", "other@example.test")
	require.NoError(t, err)
	m, files := artifactBundleFixture()
	body := makeArtifactBundleZIP(t, m, files)
	seen := map[string]bool{}
	for attempt := 0; attempt < 2; attempt++ {
		w := postArtifactBundle(ro, other.ID, body)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var artifact store.Artifact
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &artifact))
		for _, id := range []string{artifact.ID, artifact.SourceBlobID, artifact.WidgetBlobID} {
			assert.False(t, seen[id], "imports and content each get independent identities")
			seen[id] = true
		}
		assert.NotEqual(t, m.ArtifactID, artifact.ID)
		assert.Equal(t, other.ID, artifact.OwnerID)
		assert.Equal(t, m.Artifact.Title, artifact.Title)
		assert.Equal(t, m.Artifact.SourceURL, artifact.SourceURL)
		assert.Equal(t, m.Artifact.Tier, artifact.Tier)
		assert.Equal(t, 1, artifact.VersionSeq)
		assert.False(t, artifact.DownloadsApproved || artifact.ClipboardApproved || artifact.LinksApproved || artifact.CameraApproved || artifact.MicrophoneApproved || artifact.GeolocationApproved)
		assert.Empty(t, artifact.NetworkAllowlist)
		assert.Equal(t, store.ShareStateOwn, artifact.ShareStateMode)
		shares, err := ro.cfg.Store.ListArtifactShares(t.Context(), other.ID, artifact.ID)
		require.NoError(t, err)
		assert.Empty(t, shares)
		var state map[string]string
		require.NoError(t, json.Unmarshal(files["state.json"], &state))
		gotState, err := ro.cfg.Store.GetState(t.Context(), store.OwnerID(other.ID), artifact.ID, store.ViewerID(other.ID))
		require.NoError(t, err)
		assert.Equal(t, state, gotState)
		gotState, err = ro.cfg.Store.GetState(t.Context(), store.OwnerID(other.ID), artifact.ID, store.ViewerID(u.ID))
		require.NoError(t, err)
		assert.Empty(t, gotState)
		source, err := ro.readBlobString(t.Context(), artifact.SourceBlobID)
		require.NoError(t, err)
		widget, err := ro.readBlobString(t.Context(), artifact.WidgetBlobID)
		require.NoError(t, err)
		assert.Contains(t, source, "editable source")
		assert.NotContains(t, source+widget, "Portable")
		assert.NotContains(t, source+widget, m.RenderOrigin)
		assert.NotContains(t, source+widget, "/a/old-artifact/")
		assets, err := ro.cfg.Store.ListArtifactAssets(t.Context(), other.ID, artifact.ID)
		require.NoError(t, err)
		require.Len(t, assets, 3, "distinct collectors for the same URL remain distinct")
		var imageIDs []string
		for _, asset := range assets {
			assert.NotContains(t, []string{"image", "image-long", "css"}, asset.ID)
			assert.NotEqual(t, "old-generation", asset.GenerationID)
			assert.False(t, seen[asset.ID])
			seen[asset.ID] = true
			payload, err := ro.readBlobString(t.Context(), asset.BlobID)
			require.NoError(t, err)
			assert.Equal(t, int64(len(payload)), asset.SizeBytes)
			assert.Equal(t, store.AssetBlobID(other.ID, []byte(payload)), asset.BlobID)
			assert.NotContains(t, asset.SourceURL, m.RenderOrigin)
			if asset.ContentType == "text/css" {
				assert.Equal(t, ro.cfg.RenderOrigin+"/a/"+artifact.ID+"/assets/"+asset.ID, asset.SourceURL)
				assert.Contains(t, widget, asset.SourceURL)
				comment := `/* /a/old-artifact/assets/image-long */`
				assert.True(t, strings.HasSuffix(payload, comment), "inactive comment text remains editable as supplied")
				assert.NotContains(t, strings.TrimSuffix(payload, comment), "/a/old-artifact/")
				assert.NotContains(t, payload, m.RenderOrigin)
			} else {
				assert.Equal(t, "\x89PNG\x00binary", payload)
				imageIDs = append(imageIDs, asset.ID)
				assert.Contains(t, source, "/a/"+artifact.ID+"/assets/"+asset.ID)
			}
		}
		require.Len(t, imageIDs, 2)
		assert.NotEqual(t, imageIDs[0], imageIDs[1])
	}
}

type bundleImportLookupTrap struct {
	store.Store
	lookups int
}

func (s *bundleImportLookupTrap) GetUser(context.Context, int64) (*store.User, error) {
	s.lookups++
	return nil, errors.New("must not look up target")
}

func TestArtifactBundleImportCredentialRefusalsAre404BeforeLookupOrBody(t *testing.T) {
	for _, configured := range []string{"", bundleImportTestToken} {
		ro := newTestRouter(t)
		ro.cfg.ArtifactImportAdminToken = configured
		trap := &bundleImportLookupTrap{Store: ro.cfg.Store}
		ro.cfg.Store = trap
		for _, target := range []string{"1", "9999", "invalid"} {
			for _, authorization := range []string{"", "Bearer secret", "Bearer wrong", "Bearer dedicated-private-import-tokem", "Basic " + bundleImportTestToken} {
				for _, contentLength := range []int64{-1, DefaultMaxRequestBodyBytes + 1} {
					body := &countingBody{remaining: DefaultMaxRequestBodyBytes + 10}
					r := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+target+"/artifacts/import", body)
					r.ContentLength = contentLength
					r.Header.Set("Authorization", authorization)
					w := httptest.NewRecorder()
					ro.ServeHTTP(w, r)
					require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
					assert.Equal(t, "404 page not found\n", w.Body.String())
					assert.Zero(t, body.read)
				}
			}
		}
		assert.Zero(t, trap.lookups)
	}
}

func TestArtifactBundleImportSessionsAloneRefuseAndTokenWorksWithoutSession(t *testing.T) {
	in := newAdminInstance(t)
	in.ro.cfg.ArtifactImportAdminToken = bundleImportTestToken
	path := userPath(in.member.ID) + "/artifacts/import"
	for _, cookie := range []*http.Cookie{in.adminCookie, in.memberCookie} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader("invalid"))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	}
	m, files := artifactBundleFixture()
	w := postArtifactBundle(in.ro, in.member.ID, makeArtifactBundleZIP(t, m, files))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.NoError(t, in.st.SetUserDisabled(t.Context(), in.member.ID, true))
	for _, id := range []int64{in.member.ID, 99999, -1} {
		w = postArtifactBundle(in.ro, id, []byte("invalid"))
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestArtifactBundleImportRejectsMalformedBundlesBeforeStaging(t *testing.T) {
	cases := map[string]func(*artifactBundleManifest, map[string][]byte){
		"version":       func(m *artifactBundleManifest, _ map[string][]byte) { m.Version++ },
		"format":        func(m *artifactBundleManifest, _ map[string][]byte) { m.Format = "other" },
		"multipage":     func(m *artifactBundleManifest, _ map[string][]byte) { m.Pages = append(m.Pages, m.Pages[0]) },
		"entry page":    func(m *artifactBundleManifest, _ map[string][]byte) { m.EntryPage = "missing" },
		"tier":          func(m *artifactBundleManifest, _ map[string][]byte) { m.Artifact.Tier = 3 },
		"render origin": func(m *artifactBundleManifest, _ map[string][]byte) { m.RenderOrigin += "/path" },
		"source URL":    func(m *artifactBundleManifest, _ map[string][]byte) { m.Artifact.SourceURL = "javascript:bad" },
		"permissions": func(m *artifactBundleManifest, _ map[string][]byte) {
			m.Artifact.NetworkAllowlist = []string{"https://bad.example/path"}
		},
		"share mode":              func(m *artifactBundleManifest, _ map[string][]byte) { m.Artifact.ShareStateMode = "bad" },
		"missing source":          func(m *artifactBundleManifest, f map[string][]byte) { delete(f, m.Pages[0].SourcePath) },
		"missing portable":        func(m *artifactBundleManifest, f map[string][]byte) { delete(f, m.Pages[0].Path) },
		"missing state":           func(m *artifactBundleManifest, f map[string][]byte) { delete(f, m.StatePath) },
		"missing asset":           func(m *artifactBundleManifest, f map[string][]byte) { delete(f, m.Assets[0].Path) },
		"duplicate manifest path": func(m *artifactBundleManifest, _ map[string][]byte) { m.Pages[0].SourcePath = m.Pages[0].Path },
		"duplicate ID":            func(m *artifactBundleManifest, _ map[string][]byte) { m.Assets[1].ID = m.Assets[0].ID },
		"wrong size":              func(m *artifactBundleManifest, _ map[string][]byte) { m.Assets[0].SizeBytes++ },
		"bad content type":        func(m *artifactBundleManifest, _ map[string][]byte) { m.Assets[0].ContentType = "image/png\r\nX: bad" },
		"bad asset URL":           func(m *artifactBundleManifest, _ map[string][]byte) { m.Assets[0].SourceURL = "/relative" },
		"too many assets": func(m *artifactBundleManifest, _ map[string][]byte) {
			for len(m.Assets) <= 200 {
				m.Assets = append(m.Assets, m.Assets[0])
			}
		},
		"unlisted":  func(_ *artifactBundleManifest, f map[string][]byte) { f["extra"] = []byte("unexpected") },
		"traversal": func(_ *artifactBundleManifest, f map[string][]byte) { f["../outside"] = []byte("unexpected") },
		"absolute":  func(_ *artifactBundleManifest, f map[string][]byte) { f["/absolute"] = []byte("unexpected") },
		"backslash": func(_ *artifactBundleManifest, f map[string][]byte) { f[`assets\file`] = []byte("unexpected") },
		"state object": func(m *artifactBundleManifest, f map[string][]byte) {
			f[m.StatePath] = []byte(`{"key":{"nested":"invalid"}}`)
		},
		"state duplicate": func(m *artifactBundleManifest, f map[string][]byte) { f[m.StatePath] = []byte(`{"key":"a","key":"b"}`) },
		"state trailing":  func(m *artifactBundleManifest, f map[string][]byte) { f[m.StatePath] = []byte(`{} {}`) },
		"source encoding": func(m *artifactBundleManifest, f map[string][]byte) { f[m.Pages[0].SourcePath] = []byte{0xff} },
		"missing source reference": func(m *artifactBundleManifest, f map[string][]byte) {
			f[m.Pages[0].SourcePath] = []byte(`/a/old-artifact/assets/image-longer`)
		},
		"missing widget reference": func(m *artifactBundleManifest, f map[string][]byte) {
			f[m.Widget.SourcePath] = []byte(`https://old-render.example/a/old-artifact/assets/unknown`)
		},
		"missing CSS reference": func(m *artifactBundleManifest, f map[string][]byte) {
			a := &m.Assets[2]
			f[a.Path] = []byte(`/a/old-artifact/assets/unknown`)
			a.SizeBytes = int64(len(f[a.Path]))
		},
	}
	ro, dir := newTestRouterWithBlobDir(t)
	u := enableBundleImport(t, ro)
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m, files := artifactBundleFixture()
			mutate(&m, files)
			w := postArtifactBundle(ro, u.ID, makeArtifactBundleZIP(t, m, files))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Empty(t, quotaBlobContents(t, dir))
		})
	}
	artifacts, err := ro.cfg.Store.ListArtifacts(t.Context(), store.ListOptions{OwnerID: u.ID})
	require.NoError(t, err)
	assert.Empty(t, artifacts)
}

func TestArtifactBundleImportChecksAllCRCsAndFileTypes(t *testing.T) {
	for _, name := range []string{"manifest.json", "pages/main.html", "source/main.html", "pages/widget.html", "source/widget.html", "state.json", "assets/image/payload"} {
		for _, zeroChecksum := range []bool{false, true} {
			t.Run(name+strconv.FormatBool(zeroChecksum), func(t *testing.T) {
				m, files := artifactBundleFixture()
				body := makeArtifactBundleZIP(t, m, files)
				if zeroChecksum {
					// Force a zero CRC in both headers: archive/zip itself skips
					// checking this value; readImportEntry must still refuse it.
					for index := 0; index+46 < len(body); index++ {
						if bytes.Equal(body[index:index+4], []byte{'P', 'K', 1, 2}) {
							length := int(binary.LittleEndian.Uint16(body[index+28 : index+30]))
							if index+46+length <= len(body) && string(body[index+46:index+46+length]) == name {
								binary.LittleEndian.PutUint32(body[index+16:index+20], 0)
							}
						}
					}
				} else {
					payload := files[name]
					if name == "manifest.json" {
						payload, _ = json.Marshal(m)
					}
					index := bytes.Index(body, payload)
					require.GreaterOrEqual(t, index, 0)
					body[index] ^= 1
				}
				_, err := readArtifactBundleImport(body, DefaultMaxRequestBodyBytes)
				require.Error(t, err)
			})
		}
	}
	for _, kind := range []string{"symlink", "directory", "duplicate"} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		header := &zip.FileHeader{Name: "manifest.json"}
		if kind == "symlink" {
			header.SetMode(0o777 | fs.ModeSymlink)
		}
		if kind == "directory" {
			header.Name = "directory/"
			header.SetMode(0o755 | fs.ModeDir)
		}
		entry, err := writer.CreateHeader(header)
		require.NoError(t, err)
		if kind != "directory" {
			_, err = entry.Write([]byte(`{}`))
			require.NoError(t, err)
		}
		if kind == "duplicate" {
			_, err = writer.Create("manifest.json")
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
		_, err = readArtifactBundleImport(buffer.Bytes(), DefaultMaxRequestBodyBytes)
		require.Error(t, err, kind)
	}
}

func TestArtifactBundleImportBoundsExpandedDataAndMalformedMetadata(t *testing.T) {
	for _, name := range []string{"pages/main.html", "source/main.html", "pages/widget.html", "source/widget.html", "state.json"} {
		m, files := artifactBundleFixture()
		files[name] = bytes.Repeat([]byte("x"), 1025)
		_, err := readArtifactBundleImport(makeArtifactBundleZIP(t, m, files), 1024)
		require.Error(t, err, name)
	}
	for _, metadata := range []string{`"tier":"2"`, `"title":null`, `"downloads_approved":null`, `"unknown":true`, `"title":"one","title":"two"`} {
		body := []byte(`{"artifact":{` + metadata + `}}`)
		var manifest artifactBundleManifest
		require.Error(t, decodeArtifactBundleManifest(body, &manifest), metadata)
	}
	m, files := artifactBundleFixture()
	state := map[string]string{}
	for i := 0; i <= store.MaxImportedStateKeys; i++ {
		state[strconv.Itoa(i)] = "value"
	}
	files[m.StatePath], _ = json.Marshal(state)
	_, err := readArtifactBundleImport(makeArtifactBundleZIP(t, m, files), DefaultMaxRequestBodyBytes)
	require.ErrorIs(t, err, store.ErrImportStateTooLarge)

	// Central-directory sizes are checked before allocating expanded bodies,
	// including a dishonest oversized metadata or asset header.
	for _, name := range []string{"manifest.json", "assets/image/payload"} {
		m, files := artifactBundleFixture()
		body := makeArtifactBundleZIP(t, m, files)
		for index := 0; index+46 < len(body); index++ {
			if bytes.Equal(body[index:index+4], []byte{'P', 'K', 1, 2}) {
				length := int(binary.LittleEndian.Uint16(body[index+28 : index+30]))
				if index+46+length <= len(body) && string(body[index+46:index+46+length]) == name {
					binary.LittleEndian.PutUint32(body[index+24:index+28], 97<<20)
				}
			}
		}
		_, err := readArtifactBundleImport(body, DefaultMaxRequestBodyBytes)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "size limit")
	}
}

type bundleImportFailingStore struct{ store.Store }

func (s bundleImportFailingStore) ImportArtifactBundle(context.Context, int64, store.ArtifactBundleImport) error {
	return errors.New("simulated publication failure")
}

type bundleImportPartialBlob struct {
	blob.Store
	writes int
}

func (s *bundleImportPartialBlob) Put(ctx context.Context, id string, body io.Reader) error {
	s.writes++
	if s.writes == 2 {
		return errors.New("simulated partial staging failure")
	}
	return s.Store.Put(ctx, id, body)
}

func TestArtifactBundleImportQuotaAndStagingFailuresPublishNothing(t *testing.T) {
	for _, failure := range []string{"quota", "publication", "partial staging"} {
		t.Run(failure, func(t *testing.T) {
			ro, dir := newTestRouterWithBlobDir(t)
			u := enableBundleImport(t, ro)
			realStore := ro.cfg.Store
			switch failure {
			case "quota":
				ro.cfg.Entitlements = enforcing(1)
			case "publication":
				ro.cfg.Store = bundleImportFailingStore{ro.cfg.Store}
			case "partial staging":
				ro.cfg.Blob = &bundleImportPartialBlob{Store: ro.cfg.Blob}
			}
			m, files := artifactBundleFixture()
			w := postArtifactBundle(ro, u.ID, makeArtifactBundleZIP(t, m, files))
			status := http.StatusInternalServerError
			if failure == "quota" {
				status = http.StatusRequestEntityTooLarge
			}
			require.Equal(t, status, w.Code, w.Body.String())
			artifacts, err := realStore.ListArtifacts(t.Context(), store.ListOptions{OwnerID: u.ID})
			require.NoError(t, err)
			assert.Empty(t, artifacts)
			assert.Empty(t, quotaBlobContents(t, dir))
			queued, err := realStore.PendingBlobDeletions(t.Context())
			require.NoError(t, err)
			assert.Empty(t, queued)
		})
	}
}

type bundleImportDeleteFailure struct{ blob.Store }

func (b bundleImportDeleteFailure) Delete(context.Context, string) error {
	return errors.New("simulated cleanup failure")
}

func TestArtifactBundleImportFailedCleanupRemainsDurablyReclaimable(t *testing.T) {
	ro, dir := newTestRouterWithBlobDir(t)
	u := enableBundleImport(t, ro)
	realStore, realBlob := ro.cfg.Store, ro.cfg.Blob
	ro.cfg.Store = bundleImportFailingStore{Store: realStore}
	ro.cfg.Blob = bundleImportDeleteFailure{Store: realBlob}
	m, files := artifactBundleFixture()
	w := postArtifactBundle(ro, u.ID, makeArtifactBundleZIP(t, m, files))
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	queued, err := realStore.PendingBlobDeletions(t.Context())
	require.NoError(t, err)
	require.Len(t, queued, 4)
	assert.Len(t, quotaBlobContents(t, dir), 4)
	_, err = realStore.DrainAllBlobDeletions(t.Context(), realBlob)
	require.NoError(t, err)
	assert.Empty(t, quotaBlobContents(t, dir))
	queued, err = realStore.PendingBlobDeletions(t.Context())
	require.NoError(t, err)
	assert.Empty(t, queued)
}

func TestArtifactBundleImportReusesOwnerPayloadsForQuotaAndFailedCleanup(t *testing.T) {
	ro, dir := newTestRouterWithBlobDir(t)
	u := enableBundleImport(t, ro)
	m, files := artifactBundleFixture()
	body := makeArtifactBundleZIP(t, m, files)
	w := postArtifactBundle(ro, u.ID, body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var first store.Artifact
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &first))
	assets, err := ro.cfg.Store.ListArtifactAssets(t.Context(), u.ID, first.ID)
	require.NoError(t, err)
	// Source/widget and CSS include new public identities on each import.
	// Binary payloads retain their content address within the target account.
	source, err := ro.readBlobString(t.Context(), first.SourceBlobID)
	require.NoError(t, err)
	widget, err := ro.readBlobString(t.Context(), first.WidgetBlobID)
	require.NoError(t, err)
	additional := int64(len(source) + len(widget))
	for _, asset := range assets {
		if asset.ContentType == "text/css" {
			additional += asset.SizeBytes
		}
	}
	used, err := ro.cfg.Store.StorageUsage(t.Context(), u.ID)
	require.NoError(t, err)
	previousEntitlements := ro.cfg.Entitlements
	ro.cfg.Entitlements = enforcing(used + additional)
	w = postArtifactBundle(ro, u.ID, body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	nowUsed, err := ro.cfg.Store.StorageUsage(t.Context(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, used+additional, nowUsed)

	// A later failed publication cannot reclaim payloads still named by the
	// successful imports, even when they were part of the failed staging set.
	ro.cfg.Entitlements = previousEntitlements
	before := quotaBlobContents(t, dir)
	ro.cfg.Store = bundleImportFailingStore{Store: ro.cfg.Store}
	w = postArtifactBundle(ro, u.ID, body)
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Equal(t, before, quotaBlobContents(t, dir))
}

func TestArtifactBundleImportedAssetsRestoreThroughStoredAssets(t *testing.T) {
	ro := newTestRouter(t)
	u := enableBundleImport(t, ro)
	require.Equal(t, defaultOwnerID, u.ID)
	m, files := artifactBundleFixture()
	w := postArtifactBundle(ro, u.ID, makeArtifactBundleZIP(t, m, files))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var imported store.Artifact
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &imported))
	exported := getAssetsExport(ro, imported.ID)
	require.Equal(t, http.StatusOK, exported.Code, exported.Body.String())
	w = postImport(ro, imported.ID, "assets", exported.Body.Bytes(), true)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

func TestArtifactBundleImportWithoutWidgetAssetsOrState(t *testing.T) {
	ro := newTestRouter(t)
	u := enableBundleImport(t, ro)
	m, files := artifactBundleFixture()
	delete(files, m.Widget.Path)
	delete(files, m.Widget.SourcePath)
	m.Widget = nil
	for _, asset := range m.Assets {
		delete(files, asset.Path)
	}
	m.Assets = []exportedAsset{}
	m.Artifact.Title = ""
	files[m.Pages[0].SourcePath] = []byte(" \n\t ")
	files[m.StatePath] = []byte(`{}`)
	w := postArtifactBundle(ro, u.ID, makeArtifactBundleZIP(t, m, files))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var artifact store.Artifact
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &artifact))
	assert.Empty(t, artifact.WidgetBlobID)
	assert.Empty(t, artifact.Title, "an ordinary pasted artifact can have no title")
	source, err := ro.readBlobString(t.Context(), artifact.SourceBlobID)
	require.NoError(t, err)
	assert.Equal(t, string(files[m.Pages[0].SourcePath]), source, "nonempty whitespace source remains exact")
}

func TestArtifactBundleImportRewritesWholeIDsAndEquivalentOriginsOnly(t *testing.T) {
	m, files := artifactBundleFixture()
	files[m.Pages[0].SourcePath] = []byte(`<input accept="image/*"><img src="https://OLD-RENDER.example:443/a/old-artifact/assets/image?x=1#preview">` +
		`<img src="https://old-render.example:0443/a/old-artifact/assets/image">` +
		`<img srcset="https://old-render.example/a/old-artifact/assets/image, https://old-render.example/a/old-artifact/assets/image-long 2x">` +
		`<img srcset="https://old-render.example/a/old-artifact/assets/image?v=1, https://old-render.example/a/old-artifact/assets/image-long?x=a,b 2x">` +
		`<img src="//old-render.example/a/old-artifact/assets/image-long">` +
		`<a href="https://other.example/a/old-artifact/assets/image">unrelated origin</a>` +
		`<a href="https://other.example/nested/a/old-artifact/assets/image">unrelated nested path</a>` +
		`<a href="https://old-render.example/nested/a/old-artifact/assets/image">unrelated same-origin path</a>` +
		`<a href="ftp://old-render.example/a/old-artifact/assets/image">unrelated protocol</a>` +
		`<a href="https://other.example/view(/a/old-artifact/assets/missing)">parentheses</a>` +
		`<a href="/nested/a/old-artifact/assets/missing">nested root path</a>` +
		`<a href="/a/old-artifact/assets/image.png">different identity</a>` +
		`<style>body{background:url("/a/old-artifact/assets/image?label=it\'s")}</style>` +
		`<style>p{background:url("https://other.example/it\'s/a/old-artifact/assets/missing")}</style>`)
	bundle, err := readArtifactBundleImport(makeArtifactBundleZIP(t, m, files), DefaultMaxRequestBodyBytes)
	require.NoError(t, err)
	result, content, err := bundle.prepare(1, "https://destination.example")
	require.NoError(t, err)
	source := string(content[result.BodyBlobID])
	assert.Contains(t, source, "https://destination.example/a/"+result.ID+"/assets/")
	assert.Contains(t, source, "?x=1#preview")
	assert.NotContains(t, source, "OLD-RENDER.example")
	assert.NotContains(t, source, ":0443")
	assert.NotContains(t, source, `src="//old-render.example/a/old-artifact/assets/image-long"`)
	assert.Contains(t, source, `https://other.example/a/old-artifact/assets/image`)
	assert.Contains(t, source, `https://other.example/nested/a/old-artifact/assets/image`)
	assert.Contains(t, source, `https://old-render.example/nested/a/old-artifact/assets/image`)
	assert.Contains(t, source, `ftp://old-render.example/a/old-artifact/assets/image`)
	assert.Contains(t, source, `https://other.example/view(/a/old-artifact/assets/missing)`)
	assert.Contains(t, source, `/nested/a/old-artifact/assets/missing`)
	assert.Contains(t, source, `/a/old-artifact/assets/image.png`)
	assert.Contains(t, source, `background:url("/a/`+result.ID+`/assets/`)
	assert.Contains(t, source, `?label=it\'s")`)
	assert.Contains(t, source, `https://other.example/it\'s/a/old-artifact/assets/missing`)
}

func TestArtifactBundleImportDistinguishesCommentsFromLiveURLs(t *testing.T) {
	for _, test := range artifactBundleCommentSources() {
		t.Run(test.name, func(t *testing.T) {
			manifest, files := artifactBundleFixture()
			prefix := "/a/" + manifest.ArtifactID + "/assets/"
			active := prefix + "image"
			source := strings.NewReplacer("{{asset}}", active, "{{missing}}", prefix+"missing").Replace(test.source)
			files[manifest.Pages[0].SourcePath] = []byte(source)
			bundle, err := readArtifactBundleImport(makeArtifactBundleZIP(t, manifest, files), DefaultMaxRequestBodyBytes)
			require.NoError(t, err)
			artifact, content, err := bundle.prepare(1, "https://destination.example")
			require.NoError(t, err)
			restored := string(content[artifact.BodyBlobID])
			assert.NotContains(t, restored, active, "the live reference must use the fresh identity")
			assert.Contains(t, restored, "/a/"+artifact.ID+"/assets/")
			if strings.Contains(source, prefix+"missing") {
				assert.Contains(t, restored, prefix+"missing", "inactive text remains editable as supplied")
			}
		})
	}
}

func TestArtifactBundleImportRejectsExcessiveNestedHTMLBeforeStaging(t *testing.T) {
	ro := newTestRouter(t)
	user := enableBundleImport(t, ro)
	manifest, files := artifactBundleFixture()
	files[manifest.Pages[0].SourcePath] = []byte(artifactBundleNestedHTML(64, "<p>nested</p>"))
	w := postArtifactBundle(ro, user.ID, makeArtifactBundleZIP(t, manifest, files))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "nesting or work limit")
	usage, err := ro.cfg.Store.StorageUsage(t.Context(), user.ID)
	require.NoError(t, err)
	require.Zero(t, usage)
}

type bundleImportRepeatedByte byte

func (value bundleImportRepeatedByte) Read(body []byte) (int, error) {
	for i := range body {
		body[i] = byte(value)
	}
	return len(body), nil
}

func TestArtifactBundleImportBoundsCompressedAssetExpansion(t *testing.T) {
	for _, test := range []struct {
		name   string
		count  int
		size   int64
		unique bool
	}{
		{"unique bytes", 2, 25 << 20, true},
		{"all entries", 3, 33 << 20, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, files := artifactBundleFixture()
			for _, asset := range m.Assets {
				delete(files, asset.Path)
			}
			m.Assets = []exportedAsset{}
			files[m.Pages[0].SourcePath] = []byte("plain source")
			files[m.Widget.SourcePath] = []byte("plain widget")
			for i := 0; i < test.count; i++ {
				id := "asset-" + strconv.Itoa(i)
				m.Assets = append(m.Assets, exportedAsset{assetView: assetView{ID: id, SourceURL: "https://example.com/" + id,
					ContentType: "application/octet-stream", SizeBytes: test.size}, Path: "assets/" + id + "/payload"})
			}
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			entry, err := writer.Create("manifest.json")
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(entry).Encode(m))
			for name, body := range files {
				entry, err := writer.Create(name)
				require.NoError(t, err)
				_, err = entry.Write(body)
				require.NoError(t, err)
			}
			for i, asset := range m.Assets {
				entry, err := writer.Create(asset.Path)
				require.NoError(t, err)
				value := bundleImportRepeatedByte('x')
				if test.unique {
					value += bundleImportRepeatedByte(i)
				}
				_, err = io.CopyN(entry, value, test.size)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			require.Less(t, int64(buffer.Len()), DefaultMaxRequestBodyBytes)
			_, err = readArtifactBundleImport(buffer.Bytes(), DefaultMaxRequestBodyBytes)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "size limit")
		})
	}
}
