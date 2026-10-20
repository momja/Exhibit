package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

// Serve only the exported document: the originating render routes and upstream
// JSON/image URLs are unavailable. This checks executable portability, including
// nested JSON string syntax and MIME parameters, rather than just ZIP contents.
func TestArtifactBundlePortableJSONAndImageBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ro := newTestRouter(t)
	source := `<html><head></head><body><script>
const config = "{\"image\":\"__ASSET_URL__\"}";
fetch('https://upstream.invalid/data.json').then(response => response.json()).then(data => {
  if (JSON.parse(config).image !== data.image) throw new Error('escaped configuration was not materialized');
  const image = new Image();
  image.onload = () => { window.exportResult = { message: data.message, width: image.naturalWidth }; };
  image.src = data.image;
  document.body.append(image);
});
</script></body></html>`
	id := createArtifact(t, ro, map[string]any{"title": "Portable JSON", "body": source})
	source = strings.ReplaceAll(source, "__ASSET_URL__", ro.cfg.RenderOrigin+"/a/"+id+"/assets/image")
	updated := doJSON(t, ro, http.MethodPatch, "/api/artifacts/"+id, map[string]any{"body": source})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	jsonBytes, err := json.Marshal(map[string]string{
		"image": ro.cfg.RenderOrigin + "/a/" + id + "/assets/image", "message": "héllo 世界",
	})
	require.NoError(t, err)
	var assets []store.ArtifactAsset
	for _, payload := range []struct {
		id, sourceURL, contentType string
		body                       []byte
	}{
		{"image", "https://upstream.invalid/pixel.png", `image/png; name="a b"`, pngBytes.Bytes()},
		{"json", "https://upstream.invalid/data.json", `Application/JSON; charset="utf-8"`, jsonBytes},
	} {
		blobID := store.AssetBlobID(1, payload.body)
		require.NoError(t, ro.putBlob(t.Context(), blobID, bytes.NewReader(payload.body)))
		assets = append(assets, store.ArtifactAsset{ID: payload.id, BlobID: blobID,
			SourceURL: payload.sourceURL, ContentType: payload.contentType, SizeBytes: int64(len(payload.body))})
	}
	_, err = ro.cfg.Store.ReplaceArtifactAssets(t.Context(), 1, id, "portable-generation", assets)
	require.NoError(t, err)
	manifest, files := readArtifactBundle(t, getArtifactBundle(ro, id, ""))
	require.Equal(t, source, string(files[manifest.Pages[0].SourcePath]))
	portable := files[manifest.Pages[0].Path]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(portable)
	}))
	defer server.Close()
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(server.URL),
		chromedp.Poll[chromedp.Void](`window.exportResult && exportResult.width === 1 && exportResult.message === 'héllo 世界'`)))
	// MIME spelling and bytes in the archive remain suitable for exact restore.
	for _, asset := range manifest.Assets {
		if asset.ID == "image" {
			require.Equal(t, `image/png; name="a b"`, asset.ContentType)
		} else if asset.ID == "json" {
			require.Equal(t, string(jsonBytes), string(files[asset.Path]))
		}
	}
}
