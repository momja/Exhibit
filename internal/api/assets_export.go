package api

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/store"
)

type exportedAsset struct {
	assetView
	Path string `json:"path"`
}

type assetsManifest struct {
	ArtifactID string          `json:"artifact_id"`
	Assets     []exportedAsset `json:"assets"`
}

// exportAssets builds the archive before sending headers: an unreadable blob
// must produce an error, never a successful download containing only some assets.
// A temporary file keeps large binary payloads out of the server's heap.
func (ro *Router) exportAssets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "export assets: load artifact", err)
		return
	}
	if a == nil {
		http.NotFound(w, r)
		return
	}
	assets, err := ro.cfg.Store.ListArtifactAssets(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "export assets: list assets", err)
		return
	}
	file, err := os.CreateTemp("", "exhibit-assets-*.zip")
	if err != nil {
		serverError(w, r, "export assets: create archive", err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := ro.writeAssetsArchive(r.Context(), file, id, assets); err != nil {
		serverError(w, r, "export assets: build archive", err)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		serverError(w, r, "export assets: rewind archive", err)
		return
	}
	filename := id + "-assets.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filename, time.Time{}, file)
}

func (ro *Router) writeAssetsArchive(ctx context.Context, dst io.Writer, artifactID string, assets []store.ArtifactAsset) error {
	archive := zip.NewWriter(dst)
	defer archive.Close()
	manifest := assetsManifest{ArtifactID: artifactID, Assets: make([]exportedAsset, 0, len(assets))}
	views := toAssetViews(assets)
	for i, asset := range assets {
		if err := ctx.Err(); err != nil {
			return err
		}
		entryPath := assetArchivePath(asset)
		entry, err := archive.Create(entryPath)
		if err != nil {
			return err
		}
		body, err := ro.cfg.Blob.Get(ctx, asset.BlobID)
		if err != nil {
			return fmt.Errorf("read asset %s: %w", asset.ID, err)
		}
		_, copyErr := io.Copy(entry, body)
		closeErr := body.Close()
		if copyErr != nil {
			return fmt.Errorf("copy asset %s: %w", asset.ID, copyErr)
		}
		if closeErr != nil {
			return closeErr
		}
		manifest.Assets = append(manifest.Assets, exportedAsset{assetView: views.Assets[i], Path: entryPath})
	}
	entry, err := archive.Create("manifest.json")
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return err
	}
	return archive.Close()
}

// URLs are metadata, never archive paths. Separate directories by our opaque
// asset id, so even identical source filenames cannot overwrite each other.
func assetArchivePath(asset store.ArtifactAsset) string {
	name := "asset"
	if source, err := url.Parse(asset.SourceURL); err == nil {
		base := unsafeFilenameChars.ReplaceAllString(path.Base(source.Path), "-")
		base = strings.Trim(base, "-.")
		if len(base) > 80 {
			base = strings.Trim(base[:80], "-.")
		}
		if base != "" {
			name = base
		}
	}
	return "assets/" + asset.ID + "/" + name
}
