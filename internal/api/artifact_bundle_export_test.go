package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getArtifactBundle(ro *Router, id, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+id+"/export"+query, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

func readArtifactBundle(t *testing.T, w *httptest.ResponseRecorder) (artifactBundleManifest, map[string][]byte) {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/zip", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	archive, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)
	files := make(map[string][]byte)
	for _, file := range archive.File {
		assert.True(t, fs.ValidPath(file.Name))
		assert.True(t, file.Mode().IsRegular())
		require.NotContains(t, files, file.Name, "every entry has a unique path")
		rc, err := file.Open()
		require.NoError(t, err)
		files[file.Name], err = io.ReadAll(rc)
		require.NoError(t, err, "every CRC is checked")
		require.NoError(t, rc.Close())
	}
	var manifest artifactBundleManifest
	require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
	assert.Contains(t, string(files["manifest.json"]), "\n  ", "manifest is readable when opened directly")
	wantPaths := map[string]bool{"manifest.json": true, manifest.StatePath: true}
	for _, page := range manifest.Pages {
		wantPaths[page.Path], wantPaths[page.SourcePath] = true, true
	}
	if page := manifest.Widget; page != nil {
		wantPaths[page.Path], wantPaths[page.SourcePath] = true, true
	}
	for _, asset := range manifest.Assets {
		wantPaths[asset.Path] = true
	}
	require.Len(t, files, len(wantPaths), "archive contains exactly the manifest's entries")
	for name := range wantPaths {
		require.Contains(t, files, name)
	}
	return manifest, files
}

func TestArtifactBundleDefaultsToZIPAndPreservesEditableSnapshot(t *testing.T) {
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{
		"title": "Portable tool", "body": "old source", "url": "https://example.com/tool",
	})
	assets, bodies := seedExportAssets(t, ro, id)
	prefix := ro.cfg.RenderOrigin + "/a/" + id + "/assets/"
	source := `<!doctype html><html><head></head><body><img src="` + prefix + assets[1].ID + `"><script>fetch('` + assets[0].SourceURL + `')</script></body></html>`
	widget := `<p>Widget</p><img src="` + prefix + assets[1].ID + `">`
	patchArtifact(t, ro, id, map[string]any{
		"body": source, "network_allowlist": []string{"https://example.com"},
		"downloads_approved": true, "clipboard_approved": true, "links_approved": true,
		"camera_approved": true, "microphone_approved": true, "geolocation_approved": true,
		"share_state_mode": store.ShareStateShared,
	})
	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, widget).Code)
	ownerState := map[string]string{"": "empty key", "preferences": `{"color":"blue"}`, "unicode": "☃ <>&"}
	for key, value := range ownerState {
		require.NoError(t, ro.cfg.Store.SetState(t.Context(), 1, id, 1, key, value))
	}
	require.NoError(t, ro.cfg.Store.SetState(t.Context(), 1, id, 9, "private-viewer-state", "never exported"))
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	for _, query := range []string{"", "?format=zip"} {
		w := getArtifactBundle(ro, id, query)
		manifest, files := readArtifactBundle(t, w)
		assert.Contains(t, w.Header().Get("Content-Disposition"), "Portable-tool.zip")
		assert.Equal(t, artifactBundleFormat, manifest.Format)
		assert.Equal(t, artifactBundleVersion, manifest.Version)
		assert.Equal(t, id, manifest.ArtifactID)
		assert.Equal(t, ro.cfg.RenderOrigin, manifest.RenderOrigin)
		require.Len(t, manifest.Pages, 1)
		page := manifest.Pages[0]
		assert.Equal(t, "main", manifest.EntryPage)
		assert.Equal(t, "main", page.ID)
		assert.NotEqual(t, page.Path, page.SourcePath)
		assert.Equal(t, source, string(files[page.SourcePath]))
		assertSelfContained(t, string(files[page.Path]), id)
		assert.Contains(t, string(files[page.Path]), "data:application/wasm;base64,"+base64.StdEncoding.EncodeToString(bodies[0]))
		require.NotNil(t, manifest.Widget)
		assert.Equal(t, widget, string(files[manifest.Widget.SourcePath]))
		assertSelfContained(t, string(files[manifest.Widget.Path]), id)
		var state map[string]string
		require.NoError(t, json.Unmarshal(files[manifest.StatePath], &state))
		assert.Equal(t, ownerState, state)
		assert.Equal(t, artifactBundleMetadata{
			Title: "Portable tool", SourceURL: "https://example.com/tool", Tier: store.Tier1,
			NetworkAllowlist: []string{"https://example.com"}, DownloadsApproved: true, ClipboardApproved: true,
			LinksApproved: true, CameraApproved: true, MicrophoneApproved: true, GeolocationApproved: true,
			ShareStateMode: store.ShareStateShared,
		}, manifest.Artifact)
		require.Len(t, manifest.Assets, 2)
		for _, row := range manifest.Assets {
			for i, asset := range assets {
				if row.ID != asset.ID {
					continue
				}
				assert.Equal(t, bodies[i], files[row.Path], "original binary payloads survive unchanged")
				assert.Equal(t, asset.SourceURL, row.SourceURL)
				assert.NotContains(t, string(files["manifest.json"]), asset.BlobID)
			}
		}
	}
	remaining, err := os.ReadDir(temp)
	require.NoError(t, err)
	assert.Empty(t, remaining)

	legacy := exportGet(t, ro, id)
	require.Equal(t, http.StatusOK, legacy.Code)
	assert.NotContains(t, legacy.Body.String(), "preferences", "HTML never embeds owner state")
	assert.NotContains(t, legacy.Body.String(), "Widget", "legacy HTML contains only the artifact")
}

func TestArtifactBundleEmptyStateAndAssetsAreObjectsAndArrays(t *testing.T) {
	ro := newTestRouter(t)
	id := createTestArtifact(t, ro, "Empty")
	manifest, files := readArtifactBundle(t, getArtifactBundle(ro, id, ""))
	assert.Nil(t, manifest.Widget)
	assert.NotNil(t, manifest.Assets)
	assert.Empty(t, manifest.Assets)
	assert.NotContains(t, string(files["manifest.json"]), ": null")
	assert.JSONEq(t, `{}`, string(files[manifest.StatePath]))
	assert.Equal(t, files[manifest.Pages[0].SourcePath], files[manifest.Pages[0].Path])
}

// Use the same source contexts for actual export and import regressions. Asset
// lookups must ignore inactive code without losing the live dependency nearby.
func artifactBundleCommentSources() []struct{ name, source string } {
	return []struct{ name, source string }{
		{"CSS identifier", `<style>body/* {{missing}} */{background:url('{{asset}}')}</style>`},
		{"CSS parenthesis", `<style>:is(body)/* {{missing}} */{background:url('{{asset}}')}</style>`},
		{"HTML inch mark", `<p>Size: 6"</p><style>/* {{missing}} */body{background:url('{{asset}}')}</style>`},
		{"HTML possessive", `<p>James' photo</p><style>/* {{missing}} */body{background:url('{{asset}}')}</style>`},
		{"inline CSS", `<div style="/* background:url('{{missing}}') */ background:url('{{asset}}')"></div>`},
		{"MIME wildcard before comment", `<input accept="image/*"><img src="{{asset}}"><style>/* {{missing}} */body{}</style>`},
		{"script comment literal", `<script>const text = "/*"; fetch('{{asset}}'); /* {{missing}} */</script>`},
		{"encoded inline CSS quote", `<div style="content: &quot;/*&quot;; background:url('{{asset}}')"></div>`},
		{"script regular expression", `<script>const pattern = /[/*]/; fetch('{{asset}}'); /* {{missing}} */</script>`},
		{"CSS URL wildcard", `<style>body{background:url({{asset}}?label=/*)}</style>`},
		{"HTML comment", `<!-- {{missing}} --><img src="{{asset}}">`},
		{"encoded quote in foreign URL", `<img src="https://other.example/?q=&quot;{{missing}}"><img src="{{asset}}">`},
		{"encoded quote in local query", `<img src="{{asset}}?q=&quot;value&quot;">`},
		{"JS CSS URL", `<script>element.style.backgroundImage = 'url({{asset}})';</script>`},
		{"JS CSS template URL", "<script>element.style.backgroundImage = `url({{asset}})`;</script>"},
		{"duplicate script type", `<script type="module" type="text/plain">/* {{missing}} */fetch('{{asset}}');</script>`},
		{"boolean script type", `<script type type="text/plain">/* {{missing}} */fetch('{{asset}}');</script>`},
		{"optional catch regex", `<script>try {} catch {} /[/*]/.test('x'); fetch('{{asset}}'); /* {{missing}} */</script>`},
		{"labeled block regex", `<script>label: {} /[/*]/.test('x'); fetch('{{asset}}'); /* {{missing}} */</script>`},
		{"class static block regex", `<script>class C { static { {} /[/*]/.test('x'); fetch('{{asset}}'); } } /* {{missing}} */</script>`},
		{"JavaScript MIME alias", `<script type="text/x-javascript">/* {{missing}} */fetch('{{asset}}');</script>`},
		{"versioned JavaScript MIME alias", `<script type="text/javascript1.5">/* {{missing}} */fetch('{{asset}}');</script>`},
		{"hashbang JavaScript", "<script>#!/usr/bin/env node\nfetch('{{asset}}'); /* {{missing}} */</script>"},
		{"JSON data attribute", `<div data-config='{"image":"{{asset}}"}'></div>`},
		{"srcdoc attribute", `<iframe srcdoc='<img src="{{asset}}">'></iframe>`},
		{"class field ASI", "<script>class C { x = 1\nclass() { return /[/*]/; } } fetch('{{asset}}'); /* {{missing}} */</script>"},
		{"class property newline", "<script>class C { x = obj.\nvalue instanceof /[/*]/; } fetch('{{asset}}'); /* {{missing}} */</script>"},
		{"encoded quote in ping", `<a ping="https://other.example/?q=&quot;{{missing}}"><img src="{{asset}}">`},
		{"encoded quote in attribution", `<img attributionsrc="https://other.example/?q=&quot;{{missing}}" src="{{asset}}">`},
		{"encoded quote in itemid", `<div itemid="https://other.example/?q=&quot;{{missing}}"><img src="{{asset}}"></div>`},
		{"srcdoc HTML comment", `<iframe srcdoc='<!-- "{{missing}}" --><img src="{{asset}}">'></iframe>`},
		{"srcdoc CSS comment", `<iframe srcdoc='<style>/* "{{missing}}" */body{background:url("{{asset}}")}</style>'></iframe>`},
		{"srcdoc JS comment", `<iframe srcdoc='<script>/* "{{missing}}" */fetch("{{asset}}")</script>'></iframe>`},
		{"encoded srcdoc", artifactBundleNestedHTML(1, `<!-- "{{missing}}" --><img src="{{asset}}">`)},
		{"two srcdoc layers", artifactBundleNestedHTML(2, `<!-- "{{missing}}" --><div style="background:url('{{asset}}')"></div>`)},
	}
}

func artifactBundleNestedHTML(depth int, source string) string {
	for i := 0; i < depth; i++ {
		source = `<iframe srcdoc="` + html.EscapeString(source) + `"></iframe>`
	}
	return source
}

func TestArtifactBundleExportRejectsExcessiveNestedHTMLBeforeHeaders(t *testing.T) {
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"body": artifactBundleNestedHTML(64, "<p>nested</p>")})
	w := getArtifactBundle(ro, id, "")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Empty(t, w.Header().Get("Content-Disposition"))
}

func TestArtifactBundleExportDistinguishesCommentsFromLiveURLs(t *testing.T) {
	for _, test := range artifactBundleCommentSources() {
		t.Run(test.name, func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Comments")
			assets, payloads := seedExportAssets(t, ro, id)
			prefix := "/a/" + id + "/assets/"
			active := prefix + assets[1].ID
			source := strings.NewReplacer("{{asset}}", active, "{{missing}}", prefix+"missing").Replace(test.source)
			patchArtifact(t, ro, id, map[string]any{"body": source})
			manifest, files := readArtifactBundle(t, getArtifactBundle(ro, id, ""))
			assert.Equal(t, source, string(files[manifest.Pages[0].SourcePath]))
			portable := string(files[manifest.Pages[0].Path])
			assert.NotContains(t, portable, active, "the live reference must be materialized")
			assert.Contains(t, portable, "data:"+assets[1].ContentType+";base64,"+base64.StdEncoding.EncodeToString(payloads[1]))
		})
	}
}

func TestArtifactBundlePreservesEmptyTitleAndWhitespaceSource(t *testing.T) {
	ro := newTestRouter(t)
	_, result := postArtifact(t, ro, map[string]any{"body": " \n\t "})
	manifest, files := readArtifactBundle(t, getArtifactBundle(ro, result.Artifact.ID, ""))
	assert.Empty(t, manifest.Artifact.Title)
	assert.Equal(t, " \n\t ", string(files[manifest.Pages[0].SourcePath]))
}

func TestArtifactBundleRejectsMetadataAndEncodingThatCannotBeRestored(t *testing.T) {
	for _, kind := range []string{"source URL", "tier", "source encoding", "widget encoding", "asset MIME"} {
		t.Run(kind, func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Invalid")
			a, err := ro.cfg.Store.GetArtifact(t.Context(), 1, id)
			require.NoError(t, err)
			switch kind {
			case "source URL":
				require.NoError(t, ro.cfg.Store.UpdateArtifact(t.Context(), 1, id, map[string]any{"source_url": "https://name:password@example.com/page"}))
			case "tier":
				require.NoError(t, ro.cfg.Store.UpdateArtifact(t.Context(), 1, id, map[string]any{"tier": 3}))
			case "source encoding":
				require.NoError(t, ro.putBlob(t.Context(), a.SourceBlobID, bytes.NewReader([]byte{0xff})))
			case "widget encoding":
				require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, "widget").Code)
				a, err = ro.cfg.Store.GetArtifact(t.Context(), 1, id)
				require.NoError(t, err)
				require.NoError(t, ro.putBlob(t.Context(), a.WidgetBlobID, bytes.NewReader([]byte{0xff})))
			case "asset MIME":
				ro.cfg.Store = bundleExportAssetsStore{Store: ro.cfg.Store, assets: []store.ArtifactAsset{{
					ID: "bad", SourceURL: "https://example.com/payload", ContentType: "not a MIME type",
				}}}
			}
			assertFailedArtifactBundle(t, getArtifactBundle(ro, id, ""))
		})
	}
}

func TestArtifactBundleIncludesReachableHistoryWithoutShadowingCurrentRuntimeAssets(t *testing.T) {
	ro := newTestRouter(t)
	ctx := t.Context()
	id := createTestArtifact(t, ro, "History")
	prefix := ro.cfg.RenderOrigin + "/a/" + id + "/assets/"
	imageURL := "https://example.com/image.png"
	oldImage, newImage := "old-png", "new-png"
	oldCSS := `body{background:url("` + prefix + `image?size=small#preview")}`
	svg := `<svg><image href="/a/` + id + `/assets/image"/></svg>`
	currentCSS := `/*theme*/body{mask-image:url("` + prefix + `svg")}`
	makeAsset := func(assetID, sourceURL, contentType, body string) store.ArtifactAsset {
		blobID := store.AssetBlobID(1, []byte(body))
		require.NoError(t, ro.putBlob(ctx, blobID, strings.NewReader(body)))
		return store.ArtifactAsset{ID: assetID, SourceURL: sourceURL, ContentType: contentType, BlobID: blobID, SizeBytes: int64(len(body))}
	}
	_, err := ro.cfg.Store.ReplaceArtifactAssets(ctx, 1, id, "old", []store.ArtifactAsset{
		makeAsset("image", imageURL, "image/png", oldImage),
		makeAsset("style", "https://example.com/style.css", "Text/CSS; charset=utf-8", oldCSS),
		makeAsset("svg", "https://example.com/mask.svg", "Image/SVG+XML", svg),
		makeAsset("unused", "https://example.com/unused", "text/plain", "not reachable"),
	})
	require.NoError(t, err)
	widget := `<link rel="stylesheet" href="` + prefix + `style"><img src="` + prefix + `image">`
	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, widget).Code)
	source := `<link rel="stylesheet" href="/a/` + id + `/assets/current-style"><script>fetch('` + imageURL + `')</script>`
	blobID := uuid.NewString()
	require.NoError(t, ro.putBlob(ctx, blobID, strings.NewReader(source)))
	_, err = ro.cfg.Store.ReplaceArtifactSnapshot(ctx, 1, id, store.ArtifactSnapshot{
		SourceBlobID: blobID, GenerationID: "new", Assets: []store.ArtifactAsset{
			makeAsset("image-longer", imageURL, "image/png", newImage),
			makeAsset("current-style", "https://example.com/style.css", "TEXT/CSS", currentCSS),
		},
	})
	require.NoError(t, err)
	manifest, files := readArtifactBundle(t, getArtifactBundle(ro, id, ""))
	require.Len(t, manifest.Assets, 5)
	byID := make(map[string]exportedAsset)
	for _, asset := range manifest.Assets {
		byID[asset.ID] = asset
	}
	assert.NotContains(t, byID, "unused")
	for _, historical := range []string{"image", "style", "svg"} {
		assert.Equal(t, prefix+historical, byID[historical].SourceURL)
	}
	assert.Equal(t, imageURL, byID["image-longer"].SourceURL)
	assert.Equal(t, oldImage, string(files[byID["image"].Path]))
	assert.Equal(t, oldCSS, string(files[byID["style"].Path]), "archived textual assets retain editable original bytes")
	assert.Equal(t, source, string(files[manifest.Pages[0].SourcePath]))
	assert.Equal(t, widget, string(files[manifest.Widget.SourcePath]))
	for _, htmlPath := range []string{manifest.Pages[0].Path, manifest.Widget.Path} {
		doc := string(files[htmlPath])
		// Historical render URLs are lookup keys in the offline fetch map,
		// never requests back to the service. Every mapped value is local.
		pattern := regexp.MustCompile(`(?m)^  var M = (.*);$`)
		match := pattern.FindStringSubmatch(doc)
		require.Len(t, match, 2)
		var runtime map[string]string
		require.NoError(t, json.Unmarshal([]byte(match[1]), &runtime))
		assert.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte(newImage)), runtime[imageURL])
		for _, uri := range runtime {
			assert.True(t, strings.HasPrefix(uri, "data:"))
		}
		assertSelfContained(t, pattern.ReplaceAllString(doc, ""), id)
		assertPortableTextAssets(t, doc, id, 0)
	}
}

// Inspect nested data URIs, not just the outer document: base64 must not hide
// a CSS/SVG dependency that still points at this instance.
func assertPortableTextAssets(t *testing.T, doc, id string, depth int) {
	t.Helper()
	require.Less(t, depth, 10)
	pattern := regexp.MustCompile(`(?i)data:(?:text/css|image/svg\+xml)(?:;[^,]+)?;base64,([A-Za-z0-9+/=]+)`)
	for _, match := range pattern.FindAllStringSubmatch(doc, -1) {
		decoded, err := base64.StdEncoding.DecodeString(match[1])
		require.NoError(t, err)
		assertSelfContained(t, string(decoded), id)
		assertPortableTextAssets(t, string(decoded), id, depth+1)
	}
}

func TestArtifactBundleMaterializesEquivalentOriginsAndPreservesForeignURLs(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.RenderOrigin = "https://render.example"
	id := createTestArtifact(t, ro, "Origin variants")
	assets, payloads := seedExportAssets(t, ro, id)
	prefix := "/a/" + id + "/assets/"
	localReferences := []string{
		"https://render.example" + prefix + assets[0].ID,
		"//render.example" + prefix + assets[0].ID,
		"HTTPS://RENDER.example:443" + prefix + assets[0].ID + "?variant=one#preview",
		"https://render.example:0443" + prefix + assets[0].ID,
		prefix + assets[1].ID,
	}
	foreignReferences := []string{
		"https://other.example" + prefix + "missing-asset",
		"https://other.example" + prefix + assets[0].ID,
		"https://other.example/nested" + prefix + assets[0].ID,
		"https://render.example/nested" + prefix + assets[0].ID,
		"ftp://render.example" + prefix + assets[0].ID,
		"https://other.example/view(" + prefix + "missing-asset)",
		"/nested" + prefix + "missing-asset",
		prefix + assets[0].ID + ".png",
	}
	body := `<html><head></head><body><input accept="image/*">`
	for _, reference := range append(append([]string{}, localReferences...), foreignReferences...) {
		body += `<img src="` + reference + `">`
	}
	body += "</body></html>"
	body += `<img srcset="https://render.example` + prefix + assets[0].ID + `, https://render.example` + prefix + assets[1].ID + ` 2x">`
	body += `<img srcset="https://render.example` + prefix + assets[0].ID + `?v=1, https://render.example` + prefix + assets[1].ID + `?x=a,b 2x">`
	body += `<style>body{background:url("` + prefix + assets[0].ID + `?label=it\'s")}</style>`
	body += `<style>p{background:url("https://other.example/it\'s` + prefix + `missing-asset")}</style>`
	w := doJSON(t, ro, http.MethodPatch, "/api/artifacts/"+id, map[string]any{"body": body})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	for _, query := range []string{"", "?format=html"} {
		w := getArtifactBundle(ro, id, query)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		portable := w.Body.String()
		if query == "" {
			manifest, files := readArtifactBundle(t, w)
			portable = string(files[manifest.Pages[0].Path])
			assert.Equal(t, body, string(files[manifest.Pages[0].SourcePath]))
		}
		for i, reference := range localReferences {
			index := 0
			if i == len(localReferences)-1 {
				index = 1
			}
			expected := "data:" + assets[index].ContentType + ";base64," + base64.StdEncoding.EncodeToString(payloads[index])
			if strings.Contains(reference, "#preview") {
				expected += "#preview"
			}
			assert.Contains(t, portable, `src="`+expected+`"`)
		}
		for _, reference := range foreignReferences {
			assert.Contains(t, portable, `src="`+reference+`"`)
		}
		first := "data:" + assets[0].ContentType + ";base64," + base64.StdEncoding.EncodeToString(payloads[0])
		second := "data:" + assets[1].ContentType + ";base64," + base64.StdEncoding.EncodeToString(payloads[1])
		assert.Contains(t, portable, `srcset="`+first+`, `+second+` 2x"`)
		assert.Contains(t, portable, `background:url("`+first+`")`)
		assert.Contains(t, portable, `https://other.example/it\'s`+prefix+`missing-asset`)
	}
}

func TestArtifactBundleRejectsUnknownFormatsAndForeignArtifacts(t *testing.T) {
	ro := newTestRouter(t)
	id := createTestArtifact(t, ro, "Mine")
	foreign := seedForeignArtifact(t, ro)
	for _, format := range []string{"pdf", "HTML", "Zip", "html.zip"} {
		w := getArtifactBundle(ro, id, "?format="+format)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, w.Header().Get("Content-Disposition"))
	}
	for _, query := range []string{"", "?format=zip", "?format=html"} {
		for _, missing := range []string{"missing", foreign} {
			assert.Equal(t, http.StatusNotFound, getArtifactBundle(ro, missing, query).Code)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+id+"/export"+query, nil)
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestArtifactBundleMissingContentFailsBeforeDownloadHeaders(t *testing.T) {
	for _, missing := range []string{"source", "widget", "asset", "asset size", "referenced row", "textual dependency"} {
		t.Run(missing, func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Missing")
			assets, _ := seedExportAssets(t, ro, id)
			require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, "widget").Code)
			a, err := ro.cfg.Store.GetArtifact(t.Context(), 1, id)
			require.NoError(t, err)
			switch missing {
			case "source":
				require.NoError(t, ro.cfg.Blob.Delete(t.Context(), a.SourceBlobID))
			case "widget":
				require.NoError(t, ro.cfg.Blob.Delete(t.Context(), a.WidgetBlobID))
			case "asset":
				require.NoError(t, ro.cfg.Blob.Delete(t.Context(), assets[1].BlobID))
			case "asset size":
				require.NoError(t, ro.putBlob(t.Context(), assets[1].BlobID, strings.NewReader("bad")))
			case "referenced row":
				patchArtifact(t, ro, id, map[string]any{"body": `<img src="/a/` + id + `/assets/not-there">`})
			case "textual dependency":
				payload := `<svg><image href="/a/` + id + `/assets/not-there"/></svg>`
				asset := store.ArtifactAsset{ID: "svg", SourceURL: "https://example.com/mask.svg", ContentType: "image/svg+xml", BlobID: "mask", SizeBytes: int64(len(payload))}
				require.NoError(t, ro.putBlob(t.Context(), asset.BlobID, strings.NewReader(payload)))
				_, err := ro.cfg.Store.ReplaceArtifactAssets(t.Context(), 1, id, "bad", []store.ArtifactAsset{asset})
				require.NoError(t, err)
			}
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			assertFailedArtifactBundle(t, getArtifactBundle(ro, id, ""))
			remaining, err := os.ReadDir(temp)
			require.NoError(t, err)
			assert.Empty(t, remaining)
			if missing == "widget" {
				assert.Equal(t, http.StatusOK, exportGet(t, ro, id).Code, "HTML does not read widget bytes")
			}
		})
	}
}

func assertFailedArtifactBundle(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Empty(t, w.Header().Get("Content-Disposition"))
	assert.NotContains(t, w.Header().Get("Content-Type"), "zip")
	assert.False(t, bytes.HasPrefix(w.Body.Bytes(), []byte("PK")))
}

type bundleExportFaultBlob struct {
	blob.Store
	faultID string
	close   bool
}

func (b bundleExportFaultBlob) Get(ctx context.Context, id string) (io.ReadCloser, error) {
	if id == b.faultID {
		if b.close {
			return bundleExportCloseFailure{Reader: strings.NewReader("source")}, nil
		}
		return io.NopCloser(io.MultiReader(strings.NewReader("partial"), bundleExportReadFailure{})), nil
	}
	return b.Store.Get(ctx, id)
}

type bundleExportReadFailure struct{}

func (bundleExportReadFailure) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type bundleExportCloseFailure struct{ io.Reader }

func (bundleExportCloseFailure) Close() error { return errors.New("close failed") }

func TestArtifactBundleReaderFailuresNeverBecomeSuccessfulDownloads(t *testing.T) {
	for _, closeFailure := range []bool{false, true} {
		ro := newTestRouter(t)
		id := createTestArtifact(t, ro, "Failure")
		a, err := ro.cfg.Store.GetArtifact(t.Context(), 1, id)
		require.NoError(t, err)
		ro.cfg.Blob = bundleExportFaultBlob{Store: ro.cfg.Blob, faultID: a.SourceBlobID, close: closeFailure}
		assertFailedArtifactBundle(t, getArtifactBundle(ro, id, ""))
	}
}

func TestArtifactBundleBoundsSourceStateManifestAndFinishedZIP(t *testing.T) {
	for _, kind := range []string{"source", "state", "state keys", "manifest", "compressed ZIP"} {
		t.Run(kind, func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Limits")
			switch kind {
			case "source":
				patchArtifact(t, ro, id, map[string]any{"body": strings.Repeat("x", 1025)})
				ro.cfg.MaxRequestBodyBytes = 1024
			case "state":
				require.NoError(t, ro.cfg.Store.SetState(t.Context(), 1, id, 1, "too-large", strings.Repeat("x", 2048)))
				ro.cfg.MaxRequestBodyBytes = 1024
			case "state keys":
				state := make(map[string]string, store.MaxImportedStateKeys+1)
				for i := 0; i <= store.MaxImportedStateKeys; i++ {
					state[fmt.Sprint(i)] = ""
				}
				ro.cfg.Store = bundleExportStateStore{Store: ro.cfg.Store, state: state}
			case "manifest":
				err := ro.cfg.Store.UpdateArtifact(t.Context(), 1, id, map[string]any{"title": strings.Repeat("x", artifactBundleExportManifestLimit)})
				require.NoError(t, err)
			case "compressed ZIP":
				ro.cfg.MaxRequestBodyBytes = 512
			}
			assertFailedArtifactBundle(t, getArtifactBundle(ro, id, ""))
		})
	}
}

type bundleExportStateStore struct {
	store.Store
	state map[string]string
}

func (s bundleExportStateStore) GetState(context.Context, store.OwnerID, string, store.ViewerID) (map[string]string, error) {
	return s.state, nil
}

func TestArtifactBundleBoundsPortableExpansionAndAssetCycles(t *testing.T) {
	for _, cycle := range []bool{false, true} {
		ro := newTestRouter(t)
		id := createTestArtifact(t, ro, "Expansion")
		prefix := ro.cfg.RenderOrigin + "/a/" + id + "/assets/"
		payload, contentType := strings.Repeat("x", 1024), "text/plain"
		if cycle {
			payload, contentType = `body{background:url("`+prefix+`asset")}`, "text/css"
		}
		asset := store.ArtifactAsset{ID: "asset", SourceURL: "https://example.com/payload", ContentType: contentType, BlobID: "payload", SizeBytes: int64(len(payload))}
		require.NoError(t, ro.putBlob(t.Context(), asset.BlobID, strings.NewReader(payload)))
		_, err := ro.cfg.Store.ReplaceArtifactAssets(t.Context(), 1, id, "generation", []store.ArtifactAsset{asset})
		require.NoError(t, err)
		patchArtifact(t, ro, id, map[string]any{"body": strings.Repeat(`<img src="`+prefix+`asset">`, 20)})
		ro.cfg.MaxRequestBodyBytes = 16 << 10
		assertFailedArtifactBundle(t, getArtifactBundle(ro, id, ""))
	}
}

type bundleExportAssetsStore struct {
	store.Store
	assets []store.ArtifactAsset
}

func (s bundleExportAssetsStore) ListArtifactAssets(context.Context, int64, string) ([]store.ArtifactAsset, error) {
	return s.assets, nil
}

type bundleExportMemoryBlob struct {
	blob.Store
	payload string
}

func (b bundleExportMemoryBlob) Get(ctx context.Context, id string) (io.ReadCloser, error) {
	if id == "budget-payload" {
		return io.NopCloser(strings.NewReader(b.payload)), nil
	}
	return b.Store.Get(ctx, id)
}

func TestArtifactBundleAssetBudgetsMatchStoredImport(t *testing.T) {
	for _, tc := range []struct {
		name    string
		count   int
		size    int
		message string
	}{
		{"asset rows", 201, 0, "invalid or oversized asset metadata"},
		{"expanded rows", 97, 1 << 20, "invalid or oversized asset metadata"},
		{"unique payloads", 1, 49 << 20, "unique asset payloads"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Asset limits")
			assets := make([]store.ArtifactAsset, tc.count)
			for i := range assets {
				assets[i] = store.ArtifactAsset{
					ID: fmt.Sprintf("asset-%d", i), BlobID: "budget-payload", SourceURL: fmt.Sprintf("https://example.com/%d", i),
					ContentType: "application/octet-stream", SizeBytes: int64(tc.size),
				}
			}
			ro.cfg.Store = bundleExportAssetsStore{Store: ro.cfg.Store, assets: assets}
			ro.cfg.Blob = bundleExportMemoryBlob{Store: ro.cfg.Blob, payload: strings.Repeat("x", tc.size)}
			a, err := ro.cfg.Store.GetArtifact(t.Context(), 1, id)
			require.NoError(t, err)
			_, err = ro.loadArtifactBundleContent(t.Context(), 1, a, true)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestArtifactBundleAcceptsExactCompressedUploadCeiling(t *testing.T) {
	ro := newTestRouter(t)
	id := createTestArtifact(t, ro, "Exact ZIP limit")
	first := getArtifactBundle(ro, id, "")
	require.Equal(t, http.StatusOK, first.Code)
	ro.cfg.MaxRequestBodyBytes = int64(first.Body.Len())
	second := getArtifactBundle(ro, id, "")
	readArtifactBundle(t, second)
	assert.Equal(t, first.Body.Len(), second.Body.Len())
	ro.cfg.MaxRequestBodyBytes--
	assertFailedArtifactBundle(t, getArtifactBundle(ro, id, ""))
}
