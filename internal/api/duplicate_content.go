package api

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
)

func textualAsset(contentType string) bool {
	return strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "javascript") || strings.Contains(contentType, "json") || strings.Contains(contentType, "svg")
}

// duplicateContent preserves the current runtime manifest and follows stored
// asset URLs into history: a refetch can leave the widget using an older asset.
// Only reachable historical assets join the copy, without copying its history.
func (ro *Router) duplicateContent(ctx context.Context, ownerID int64, source *store.Artifact, d *store.ArtifactDuplicate) (map[string]string, error) {
	body, err := ro.readBlobString(ctx, source.SourceBlobID)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	var widget string
	if source.WidgetBlobID != "" {
		widget, err = ro.readBlobString(ctx, source.WidgetBlobID)
		if err != nil {
			return nil, fmt.Errorf("read widget: %w", err)
		}
	}

	// Match whole path segments, so an unknown ID that starts with a known ID
	// is neither mistaken for that asset nor partially rewritten.
	prefix := "/a/" + source.ID + "/assets/"
	assetURL := regexp.MustCompile(regexp.QuoteMeta(prefix) + `[A-Za-z0-9_-]+`)
	assets := append([]store.ArtifactAsset(nil), d.SourceAssets...)
	seen := make(map[string]bool, len(assets))
	for _, asset := range assets {
		seen[asset.ID] = true
	}
	includeReferences := func(content string) error {
		for _, path := range assetURL.FindAllString(content, -1) {
			id := strings.TrimPrefix(path, prefix)
			if seen[id] {
				continue
			}
			seen[id] = true
			asset, err := ro.cfg.Store.GetArtifactAsset(ctx, ownerID, source.ID, id)
			if err != nil {
				return fmt.Errorf("lookup referenced asset %s: %w", id, err)
			}
			if asset != nil {
				assets = append(assets, *asset)
			}
		}
		return nil
	}
	for _, content := range []string{body, widget} {
		if err := includeReferences(content); err != nil {
			return nil, err
		}
	}
	var contents []string
	for i := 0; i < len(assets); i++ {
		content, err := ro.readBlobString(ctx, assets[i].BlobID)
		if err != nil {
			return nil, fmt.Errorf("read asset %s: %w", assets[i].ID, err)
		}
		contents = append(contents, content)
		if textualAsset(assets[i].ContentType) {
			if err := includeReferences(content); err != nil {
				return nil, err
			}
		}
	}

	replacements := make(map[string]string, len(assets))
	for _, asset := range assets {
		id, err := store.NewAssetID()
		if err != nil {
			return nil, fmt.Errorf("asset identity: %w", err)
		}
		replacements[prefix+asset.ID] = "/a/" + d.ID + "/assets/" + id
		asset.ID = id
		d.Assets = append(d.Assets, asset)
	}
	rewrite := func(content string) string {
		return assetURL.ReplaceAllStringFunc(content, func(path string) string {
			if replacement, ok := replacements[path]; ok {
				return replacement
			}
			return path
		})
	}
	body = rewrite(body)
	d.SourceText = store.ExtractSearchText(body)
	blobs := map[string]string{d.BodyBlobID: body}
	if source.WidgetBlobID != "" {
		d.WidgetBlobID = uuid.NewString()
		blobs[d.WidgetBlobID] = rewrite(widget)
	}
	for i, asset := range assets {
		content := contents[i]
		if textualAsset(asset.ContentType) {
			content = rewrite(content)
		}
		copied := &d.Assets[i]
		copied.BlobID = store.AssetBlobID(ownerID, []byte(content))
		copied.SourceURL = rewrite(asset.SourceURL)
		if i >= len(d.SourceAssets) {
			// Historical payloads are reached by stored URL. Giving them that
			// URL as their manifest key avoids shadowing a current asset from
			// the same upstream URL with bytes from an older generation.
			copied.SourceURL = ro.cfg.RenderOrigin + replacements[prefix+asset.ID]
		}
		copied.SizeBytes = int64(len(content))
		// Never overwrite immutable bytes shared with the source. Blob locks
		// protect the references until the clone transaction completes.
		if copied.BlobID != asset.BlobID {
			blobs[copied.BlobID] = content
		}
	}
	return blobs, nil
}
