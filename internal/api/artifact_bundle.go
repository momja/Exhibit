package api

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/momja/Exhibit/internal/export"
	"github.com/momja/Exhibit/internal/origin"
	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/momja/Exhibit/internal/store"
)

// Match import's per-document/upload ceiling and Stored assets' row/unique
// budgets. Historical dependencies consume the same budgets as current assets.
// Fixed page/widget/state counts also bound the archive's total expanded size.
const artifactBundleExportManifestLimit = 1 << 20

type artifactBundleContent struct {
	source       string
	widget       string
	assets       []store.ArtifactAsset
	payloads     map[string][]byte // original bytes, keyed by asset identity
	currentCount int
	limit        int64
	origin       string
	prefix       string
}

func (ro *Router) loadArtifactBundleContent(ctx context.Context, ownerID int64, a *store.Artifact, includeWidget bool) (*artifactBundleContent, error) {
	c := &artifactBundleContent{
		limit: ro.cfg.maxRequestBodyBytes(), origin: strings.TrimRight(ro.cfg.RenderOrigin, "/"),
		prefix: "/a/" + a.ID + "/assets/", payloads: make(map[string][]byte),
	}
	if includeWidget {
		if err := validateArtifactBundleExportMetadata(a, c.origin); err != nil {
			return nil, err
		}
	}
	body, err := ro.readBundleBlob(ctx, a.SourceBlobID, c.limit)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	c.source = string(body)
	if includeWidget && a.WidgetBlobID != "" {
		body, err = ro.readBundleBlob(ctx, a.WidgetBlobID, c.limit)
		if err != nil {
			return nil, fmt.Errorf("read widget: %w", err)
		}
		c.widget = string(body)
	}
	if includeWidget && (!utf8.ValidString(c.source) || !utf8.ValidString(c.widget) || c.source == "") {
		return nil, fmt.Errorf("artifact bundle requires nonempty UTF-8 source and UTF-8 widget content")
	}
	c.assets, err = ro.cfg.Store.ListArtifactAssets(ctx, ownerID, a.ID)
	if err != nil {
		return nil, err
	}
	c.currentCount = len(c.assets)
	limits := snapshot.DefaultLimits()
	seen := make(map[string]bool, len(c.assets))
	for _, asset := range c.assets {
		if seen[asset.ID] {
			return nil, fmt.Errorf("duplicate asset identity %s", asset.ID)
		}
		seen[asset.ID] = true
	}
	includeReferences := func(content, contentType string) error {
		scanner := newArtifactBundleReferenceScanner(content, contentType)
		for reference, ok := scanner.next(); ok; reference, ok = scanner.next() {
			id := c.referenceID(reference.url)
			if id == "" || seen[id] {
				continue
			}
			asset, err := ro.cfg.Store.GetArtifactAsset(ctx, ownerID, a.ID, id)
			if err != nil {
				return fmt.Errorf("lookup referenced asset %s: %w", id, err)
			}
			if asset == nil {
				return fmt.Errorf("referenced asset %s is missing", id)
			}
			seen[id] = true
			c.assets = append(c.assets, *asset)
			if len(c.assets) > 2*limits.MaxAssets {
				return fmt.Errorf("artifact bundle exceeds %d assets", 2*limits.MaxAssets)
			}
		}
		return scanner.err
	}
	for _, content := range []string{c.source, c.widget} {
		if err := includeReferences(content, "text/html"); err != nil {
			return nil, err
		}
	}
	remaining, uniqueRemaining := 2*limits.MaxTotalBytes, limits.MaxTotalBytes
	unique := make(map[[32]byte][]byte)
	for i := 0; i < len(c.assets); i++ {
		asset := c.assets[i]
		_, _, contentTypeErr := mime.ParseMediaType(asset.ContentType)
		if len(c.assets) > 2*limits.MaxAssets || !importAssetID.MatchString(asset.ID) ||
			!artifactBundleExportHTTPURL(asset.SourceURL, false) || contentTypeErr != nil ||
			strings.ContainsAny(asset.ContentType, "\r\n") || asset.SizeBytes < 0 || asset.SizeBytes > remaining {
			return nil, fmt.Errorf("invalid or oversized asset metadata: %s", asset.ID)
		}
		payload, err := ro.readBundleBlob(ctx, asset.BlobID, remaining)
		if err != nil {
			return nil, fmt.Errorf("read asset %s: %w", asset.ID, err)
		}
		if int64(len(payload)) != asset.SizeBytes {
			return nil, fmt.Errorf("asset size does not match metadata: %s", asset.ID)
		}
		remaining -= int64(len(payload))
		digest := sha256.Sum256(payload)
		if existing, ok := unique[digest]; ok {
			payload = existing
		} else {
			uniqueRemaining -= int64(len(payload))
			if uniqueRemaining < 0 {
				return nil, fmt.Errorf("unique asset payloads exceed the snapshot size limit")
			}
			unique[digest] = payload
		}
		c.payloads[asset.ID] = payload
		if textualAsset(strings.ToLower(asset.ContentType)) {
			if err := includeReferences(string(payload), asset.ContentType); err != nil {
				return nil, err
			}
		}
	}
	return c, nil
}

func validateArtifactBundleExportMetadata(a *store.Artifact, renderOrigin string) error {
	if !importAssetID.MatchString(a.ID) || !artifactBundleExportHTTPURL(renderOrigin, true) ||
		(a.SourceURL != "" && !artifactBundleExportHTTPURL(a.SourceURL, false)) ||
		(a.Tier != store.Tier1 && a.Tier != store.Tier2) || !store.ValidShareStateMode(a.ShareStateMode) {
		return fmt.Errorf("invalid artifact bundle metadata")
	}
	if _, err := origin.NormalizeOrigins(a.NetworkAllowlist); err != nil {
		return fmt.Errorf("invalid artifact bundle allowlist: %w", err)
	}
	return nil
}

func artifactBundleExportHTTPURL(raw string, originOnly bool) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && u.User == nil && (u.Scheme == "http" || u.Scheme == "https") &&
		(!originOnly || ((u.Path == "" || u.Path == "/") && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""))
}

// A bounded read catches wrong size metadata and a reader that fails partway
// through. Close failures matter too: no successful ZIP may hide a failed read.
func (ro *Router) readBundleBlob(ctx context.Context, id string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, err := ro.cfg.Blob.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	payload, readErr := io.ReadAll(io.LimitReader(body, limit+1))
	closeErr := body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("artifact bundle content exceeds its size limit")
	}
	return payload, ctx.Err()
}

func (c *artifactBundleContent) referenceID(reference string) string {
	u, err := url.Parse(reference)
	if err != nil {
		return ""
	}
	if u.IsAbs() || u.Host != "" {
		candidate := reference
		if strings.HasPrefix(candidate, "//") {
			source, err := url.Parse(c.origin)
			if err != nil {
				return ""
			}
			candidate = source.Scheme + ":" + candidate
		}
		if artifactBundleOrigin(candidate) != artifactBundleOrigin(c.origin) {
			return ""
		}
	}
	if !strings.HasPrefix(u.EscapedPath(), c.prefix) {
		return ""
	}
	id := strings.TrimPrefix(u.EscapedPath(), c.prefix)
	if !importAssetID.MatchString(id) {
		return ""
	}
	return id
}

// Rewrite incrementally, checking the result before appending. A small source
// can reference the same large asset thousands of times; raw byte budgets alone
// do not bound that materialization's memory use.
func (c *artifactBundleContent) rewriteReferences(content, contentType string, replace func(string) (string, error)) (string, error) {
	var result strings.Builder
	scanner := newArtifactBundleReferenceScanner(content, contentType)
	copied := 0
	for reference, ok := scanner.next(); ok; reference, ok = scanner.next() {
		replacement := content[reference.start:reference.end]
		if id := c.referenceID(reference.url); id != "" {
			var err error
			replacement, err = replace(id)
			if err != nil {
				return "", err
			}
			if fragment := strings.IndexByte(reference.url, '#'); fragment >= 0 {
				replacement += reference.url[fragment:]
			}
			replacement += reference.separator
		}
		if int64(result.Len())+int64(reference.start-copied)+int64(len(replacement)) > c.limit {
			return "", fmt.Errorf("portable HTML exceeds its size limit")
		}
		result.WriteString(content[copied:reference.start])
		result.WriteString(replacement)
		copied = reference.end
	}
	if scanner.err != nil {
		return "", scanner.err
	}
	if int64(result.Len())+int64(len(content)-copied) > c.limit {
		return "", fmt.Errorf("portable HTML exceeds its size limit")
	}
	result.WriteString(content[copied:])
	return result.String(), nil
}

// Keep editable sources and archived payloads byte-identical. Only the HTML
// preview uses recursively inlined textual dependencies and data URIs.
func (c *artifactBundleContent) portableHTML(body string) (string, error) {
	if len(c.assets) == 0 {
		return body, nil
	}
	assets := append([]store.ArtifactAsset(nil), c.assets...)
	byID := make(map[string]store.ArtifactAsset, len(assets))
	for i := range assets {
		if i >= c.currentCount {
			// An older generation from the same upstream URL must not shadow
			// the current runtime payload in Materialize's fetch manifest.
			assets[i].SourceURL = c.origin + c.prefix + assets[i].ID
		}
		byID[assets[i].ID] = assets[i]
		// Materialize's reader is keyed by blob ID; distinct identities may
		// share a blob, so give the preview a local identity per asset.
		assets[i].BlobID = assets[i].ID
	}
	portable := make(map[string][]byte, len(assets))
	visiting := make(map[string]bool, len(assets))
	uriCache := make(map[string]string, len(assets))
	var cachedBytes int64
	var dataURI func(string) (string, error)
	dataURI = func(id string) (string, error) {
		if uri, ok := uriCache[id]; ok {
			return uri, nil
		}
		asset, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("referenced asset %s is missing", id)
		}
		payload, ok := portable[id]
		if !ok {
			if visiting[id] {
				return "", fmt.Errorf("cyclic asset references cannot be materialized: %s", id)
			}
			visiting[id] = true
			payload = c.payloads[id]
			if textualAsset(strings.ToLower(asset.ContentType)) {
				text, err := c.rewriteReferences(string(payload), asset.ContentType, dataURI)
				if err != nil {
					return "", err
				}
				payload = []byte(text)
			}
			portable[id] = payload
			delete(visiting, id)
		}
		prefix := export.DataURIPrefix(asset.ContentType)
		uriBytes := int64(len(prefix)) + int64(base64.StdEncoding.EncodedLen(len(payload)))
		if uriBytes > c.limit-cachedBytes {
			return "", fmt.Errorf("portable asset exceeds its size limit: %s", id)
		}
		uri := prefix + base64.StdEncoding.EncodeToString(payload)
		uriCache[id] = uri
		cachedBytes += int64(len(uri))
		return uri, nil
	}
	// Temporary delimited references let Materialize replace whole identities:
	// one asset ID can be the prefix of another. They never leave this call.
	materializeURL := func(asset store.ArtifactAsset) string {
		return "\x00exhibit-asset:" + asset.ID + "\x00"
	}
	body, err := c.rewriteReferences(body, "text/html", func(id string) (string, error) {
		asset, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("referenced asset %s is missing", id)
		}
		return materializeURL(asset), nil
	})
	if err != nil {
		return "", err
	}
	// Preflight replacements and the runtime manifest before allocating the
	// final HTML. Metadata can expand sixfold when JSON-escaped.
	estimate := int64(len(body))
	runtimeManifest := false
	for _, asset := range assets {
		uri, err := dataURI(asset.ID)
		if err != nil {
			return "", err
		}
		url := materializeURL(asset)
		if count := strings.Count(body, url); count > 0 {
			estimate += int64(count) * int64(len(uri)-len(url))
		} else {
			runtimeManifest = true
			estimate += int64(len(uri)) + 6*int64(len(asset.SourceURL)+len(asset.ContentType)) + 32
		}
		if estimate > c.limit {
			return "", fmt.Errorf("portable HTML exceeds its size limit")
		}
	}
	if runtimeManifest && estimate+(8<<10) > c.limit {
		return "", fmt.Errorf("portable HTML exceeds its size limit")
	}

	doc, err := export.Materialize(body, assets, materializeURL, func(id string) ([]byte, error) { return portable[id], nil })
	if err == nil && int64(len(doc)) > c.limit {
		return "", fmt.Errorf("portable HTML exceeds its size limit")
	}
	return doc, err
}

func (ro *Router) exportArtifactZIP(w http.ResponseWriter, r *http.Request, a *store.Artifact, content *artifactBundleContent) {
	file, err := os.CreateTemp("", "exhibit-artifact-*.zip")
	if err != nil {
		serverError(w, r, "export: create archive", err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := ro.writeArtifactBundle(r.Context(), file, a, content); err != nil {
		serverError(w, r, "export: build archive", err)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		serverError(w, r, "export: rewind archive", err)
		return
	}
	filename := strings.TrimSuffix(exportFilename(a.Title), ".html") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filename, time.Time{}, file)
}

func (ro *Router) writeArtifactBundle(ctx context.Context, dst io.Writer, a *store.Artifact, content *artifactBundleContent) error {
	archive := zip.NewWriter(&artifactBundleSizeWriter{dst: dst, remaining: content.limit})
	defer archive.Close()
	paths := make(map[string]bool)
	write := func(name string, body []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !fs.ValidPath(name) || strings.Contains(name, `\`) || paths[name] {
			return fmt.Errorf("invalid or duplicate archive path: %s", name)
		}
		paths[name] = true
		entry, err := archive.Create(name)
		if err != nil {
			return err
		}
		_, err = entry.Write(body)
		return err
	}
	manifest := artifactBundleManifest{
		Format: artifactBundleFormat, Version: artifactBundleVersion, ArtifactID: a.ID,
		RenderOrigin: content.origin, EntryPage: "main", StatePath: "state.json",
		Pages:  []artifactBundlePage{{ID: "main", Path: "index.html", SourcePath: "source.html"}},
		Assets: make([]exportedAsset, 0, len(content.assets)),
		Artifact: artifactBundleMetadata{
			Title: a.Title, SourceURL: a.SourceURL, Tier: a.Tier, NetworkAllowlist: a.NetworkAllowlist,
			DownloadsApproved: a.DownloadsApproved, ClipboardApproved: a.ClipboardApproved,
			LinksApproved: a.LinksApproved, CameraApproved: a.CameraApproved,
			MicrophoneApproved: a.MicrophoneApproved, GeolocationApproved: a.GeolocationApproved,
			ShareStateMode: a.ShareStateMode,
		},
	}
	if manifest.Artifact.NetworkAllowlist == nil {
		manifest.Artifact.NetworkAllowlist = []string{}
	}
	writePage := func(page artifactBundlePage, source string) error {
		doc, err := content.portableHTML(source)
		if err != nil {
			return err
		}
		if !utf8.ValidString(doc) {
			return fmt.Errorf("portable HTML is not UTF-8")
		}
		if err := write(page.SourcePath, []byte(source)); err != nil {
			return err
		}
		return write(page.Path, []byte(doc))
	}
	if err := writePage(manifest.Pages[0], content.source); err != nil {
		return err
	}
	if a.WidgetBlobID != "" {
		manifest.Widget = &artifactBundlePage{ID: "widget", Path: "widget.html", SourcePath: "widget-source.html"}
		if err := writePage(*manifest.Widget, content.widget); err != nil {
			return err
		}
	}
	state, err := ro.cfg.Store.GetState(ctx, store.OwnerID(a.OwnerID), a.ID, store.ViewerID(a.OwnerID))
	if err != nil {
		return err
	}
	if state == nil {
		state = map[string]string{}
	}
	if len(state) > store.MaxImportedStateKeys {
		return store.ErrImportStateTooLarge
	}
	// Bound raw strings before JSON escaping allocates its output. The final
	// encoded length check also accounts for escaping keys and values.
	var stateBytes int64
	for key, value := range state {
		stateBytes += int64(len(key)) + int64(len(value))
		if stateBytes > content.limit {
			return fmt.Errorf("artifact bundle state exceeds its size limit")
		}
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if int64(len(stateJSON)) > content.limit {
		return fmt.Errorf("artifact bundle state exceeds its size limit")
	}
	if err := write(manifest.StatePath, stateJSON); err != nil {
		return err
	}
	views := toAssetViews(content.assets)
	for i, asset := range content.assets {
		row := exportedAsset{assetView: views.Assets[i], Path: assetArchivePath(asset)}
		if i >= content.currentCount {
			row.SourceURL = content.origin + content.prefix + asset.ID
		}
		if err := write(row.Path, content.payloads[asset.ID]); err != nil {
			return err
		}
		manifest.Assets = append(manifest.Assets, row)
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if len(manifestJSON)+1 > artifactBundleExportManifestLimit {
		return fmt.Errorf("artifact bundle manifest exceeds its size limit")
	}
	if err := write("manifest.json", append(manifestJSON, '\n')); err != nil {
		return err
	}
	return archive.Close()
}

// Bound the compressed archive too, including its directory on Close, so the
// completed file always fits the same instance's configured import body limit.
type artifactBundleSizeWriter struct {
	dst       io.Writer
	remaining int64
}

func (w *artifactBundleSizeWriter) Write(body []byte) (int, error) {
	if int64(len(body)) > w.remaining {
		return 0, fmt.Errorf("artifact bundle ZIP exceeds the configured upload size limit")
	}
	n, err := w.dst.Write(body)
	w.remaining -= int64(n)
	return n, err
}
