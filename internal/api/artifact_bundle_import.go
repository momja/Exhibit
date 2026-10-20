package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/origin"
	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/momja/Exhibit/internal/store"
)

var artifactBundleImportPath = regexp.MustCompile(`^/api/admin/users/[^/]+/artifacts/import$`)

// Run before the global upload limiter: even a declared oversized body must
// not distinguish a disabled/unauthorized endpoint from a nonexistent one.
func (ro *Router) artifactBundleImportGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if artifactBundleImportPath.MatchString(r.URL.Path) {
			candidate := sha256.Sum256([]byte(bearerToken(r)))
			expected := sha256.Sum256([]byte(ro.cfg.ArtifactImportAdminToken))
			matches := subtle.ConstantTimeCompare(candidate[:], expected[:])
			if ro.cfg.ArtifactImportAdminToken == "" || matches != 1 {
				http.NotFound(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (ro *Router) importArtifactBundle(w http.ResponseWriter, r *http.Request) {
	ownerID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || ownerID <= 0 {
		http.NotFound(w, r)
		return
	}
	u, err := ro.cfg.Store.GetUser(r.Context(), ownerID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (u == nil || u.Disabled)) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, "bundle import target lookup", err)
		return
	}
	body, ok := readImportFile(w, r)
	if !ok {
		return
	}
	bundle, err := readArtifactBundleImport(body, ro.cfg.maxRequestBodyBytes())
	if err != nil {
		if errors.Is(err, store.ErrImportStateTooLarge) {
			writeImportError(w, r, err)
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	staged, content, err := bundle.prepare(ownerID, ro.cfg.RenderOrigin)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if writeImportError(w, r, ro.storeArtifactBundle(r.Context(), ownerID, staged, content)) {
		return
	}
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, staged.ID)
	if err != nil {
		serverError(w, r, "read imported artifact", err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

type artifactBundleImport struct {
	manifest       artifactBundleManifest
	source, widget []byte
	state          map[string]string
	assets         [][]byte
}

// Archives are read in memory, never extracted. Validate the complete directory
// first, then read every named regular entry through the existing CRC-checking
// helper. Portable HTML is checked but never used to reconstruct editable code.
func readArtifactBundleImport(body []byte, documentLimit int64) (*artifactBundleImport, error) {
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("invalid artifact ZIP: %w", err)
	}
	limits := snapshot.DefaultLimits()
	maxAssets := 2 * limits.MaxAssets
	if len(archive.File) > maxAssets+6 {
		return nil, errors.New("artifact ZIP has too many entries")
	}
	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		if !safeArtifactBundlePath(file.Name) || !file.Mode().IsRegular() || files[file.Name] != nil {
			return nil, fmt.Errorf("invalid or duplicate archive path: %q", file.Name)
		}
		files[file.Name] = file
	}
	manifestFile := files["manifest.json"]
	if manifestFile == nil {
		return nil, errors.New("artifact ZIP is missing manifest.json")
	}
	manifestBody, err := readImportEntry(manifestFile, 1<<20)
	if err != nil {
		return nil, err
	}
	bundle := &artifactBundleImport{}
	if err := decodeArtifactBundleManifest(manifestBody, &bundle.manifest); err != nil {
		return nil, err
	}
	m := &bundle.manifest
	if m.Format != artifactBundleFormat || m.Version != artifactBundleVersion {
		return nil, errors.New("unsupported artifact bundle format or version")
	}
	if !importAssetID.MatchString(m.ArtifactID) || !bundleHTTPURL(m.RenderOrigin, true) ||
		(m.Artifact.Tier != store.Tier1 && m.Artifact.Tier != store.Tier2) ||
		(m.Artifact.SourceURL != "" && !bundleHTTPURL(m.Artifact.SourceURL, false)) ||
		!store.ValidShareStateMode(m.Artifact.ShareStateMode) {
		return nil, errors.New("invalid artifact metadata")
	}
	if _, err := origin.NormalizeOrigins(m.Artifact.NetworkAllowlist); err != nil {
		return nil, fmt.Errorf("invalid artifact metadata: %w", err)
	}
	if len(m.Pages) != 1 || m.EntryPage != "main" || m.Pages[0].ID != "main" ||
		(m.Widget != nil && m.Widget.ID != "widget") {
		return nil, errors.New("only the main artifact page and optional widget are supported")
	}
	if m.Assets == nil || len(m.Assets) > maxAssets {
		return nil, errors.New("invalid artifact assets or too many assets")
	}
	used := map[string]bool{"manifest.json": true}
	read := func(path string, limit int64) ([]byte, error) {
		if !safeArtifactBundlePath(path) || used[path] || files[path] == nil {
			return nil, fmt.Errorf("missing, unsafe or duplicate manifest path: %q", path)
		}
		used[path] = true
		return readImportEntry(files[path], limit)
	}
	page := m.Pages[0]
	portable, err := read(page.Path, documentLimit)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(portable) {
		return nil, errors.New("portable page is not UTF-8")
	}
	bundle.source, err = read(page.SourcePath, documentLimit)
	if err != nil {
		return nil, err
	}
	if m.Widget != nil {
		portable, err = read(m.Widget.Path, documentLimit)
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(portable) {
			return nil, errors.New("portable widget is not UTF-8")
		}
		bundle.widget, err = read(m.Widget.SourcePath, documentLimit)
		if err != nil {
			return nil, err
		}
	}
	if !utf8.Valid(bundle.source) || !utf8.Valid(bundle.widget) || len(bundle.source) == 0 {
		return nil, errors.New("invalid editable source")
	}
	stateBody, err := read(m.StatePath, documentLimit)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(stateBody) {
		return nil, errors.New("invalid state encoding")
	}
	bundle.state, err = readStateImport(stateBody)
	if err != nil {
		return nil, err
	}
	remaining, uniqueRemaining := 2*limits.MaxTotalBytes, limits.MaxTotalBytes
	payloads := make(map[[32]byte][]byte)
	seenIDs := make(map[string]bool, len(m.Assets))
	for _, asset := range m.Assets {
		_, _, contentTypeErr := mime.ParseMediaType(asset.ContentType)
		if !importAssetID.MatchString(asset.ID) || seenIDs[asset.ID] || !bundleHTTPURL(asset.SourceURL, false) ||
			contentTypeErr != nil || strings.ContainsAny(asset.ContentType, "\r\n") || asset.SizeBytes < 0 ||
			!strings.HasPrefix(asset.Path, "assets/"+asset.ID+"/") {
			return nil, errors.New("invalid or duplicate asset metadata")
		}
		payload, err := read(asset.Path, remaining)
		if err != nil {
			return nil, err
		}
		if int64(len(payload)) != asset.SizeBytes {
			return nil, fmt.Errorf("asset size does not match manifest: %s", asset.Path)
		}
		remaining -= int64(len(payload))
		digest := sha256.Sum256(payload)
		if existing, ok := payloads[digest]; ok {
			payload = existing
		} else {
			uniqueRemaining -= int64(len(payload))
			if uniqueRemaining < 0 {
				return nil, errors.New("unique asset payloads exceed the snapshot size limit")
			}
			payloads[digest] = payload
		}
		seenIDs[asset.ID] = true
		bundle.assets = append(bundle.assets, payload)
	}
	if len(used) != len(files) {
		return nil, errors.New("archive contains entries not listed in its manifest")
	}
	return bundle, nil
}

func safeArtifactBundlePath(path string) bool {
	return fs.ValidPath(path) && path != "." && !strings.ContainsAny(path, "\\:\x00")
}

func bundleHTTPURL(raw string, originOnly bool) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && u.User == nil && (u.Scheme == "http" || u.Scheme == "https") &&
		(!originOnly || ((u.Path == "" || u.Path == "/") && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""))
}

func decodeArtifactBundleManifest(body []byte, manifest *artifactBundleManifest) error {
	if !utf8.Valid(body) {
		return errors.New("invalid manifest encoding")
	}
	// encoding/json otherwise silently accepts duplicate keys, and null for
	// booleans/strings. Reject ambiguous metadata before decoding the schema.
	decoder := json.NewDecoder(bytes.NewReader(body))
	var checkValue func(int) error
	checkValue = func(depth int) error {
		if depth > 32 {
			return errors.New("manifest nesting is too deep")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token == nil {
			return errors.New("null manifest value")
		}
		if delimiter, ok := token.(json.Delim); ok {
			seen := map[string]bool{}
			for decoder.More() {
				if delimiter == '{' {
					key, err := decoder.Token()
					if err != nil {
						return err
					}
					name, ok := key.(string)
					if !ok || seen[name] {
						return errors.New("duplicate or invalid manifest key")
					}
					seen[name] = true
				}
				if err := checkValue(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		}
		return nil
	}
	if err := checkValue(0); err != nil {
		return fmt.Errorf("invalid artifact manifest: %w", err)
	}
	decoder = json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(manifest); err != nil {
		return fmt.Errorf("invalid artifact manifest: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("invalid trailing manifest data")
	}
	return nil
}

// Match a whole asset id, consuming an absolute origin too when present. This
// lets one pass rebind absolute URLs and paths without accidentally retaining
// the exporting server's origin or rewriting another origin's same path.
func (bundle *artifactBundleImport) prepare(ownerID int64, renderOrigin string) (store.ArtifactBundleImport, map[string][]byte, error) {
	m := bundle.manifest
	result := store.ArtifactBundleImport{ID: uuid.NewString(), Title: m.Artifact.Title, SourceURL: m.Artifact.SourceURL,
		Tier: m.Artifact.Tier, BodyBlobID: uuid.NewString(), State: bundle.state}
	generation, err := store.NewGenerationID()
	if err != nil {
		return result, nil, err
	}
	result.GenerationID = generation
	newIDs := make(map[string]string, len(m.Assets))
	for _, asset := range m.Assets {
		id, err := store.NewAssetID()
		if err != nil {
			return result, nil, err
		}
		newIDs[asset.ID] = id
	}
	prefix := "/a/" + m.ArtifactID + "/assets/"
	sourceOrigin := artifactBundleOrigin(m.RenderOrigin)
	sourceURL, _ := url.Parse(m.RenderOrigin) // validated by the archive reader
	var referenceErr error
	rewriteReference := func(reference string) string {
		parsed, err := url.Parse(reference)
		if err != nil || !strings.HasPrefix(parsed.EscapedPath(), prefix) {
			return reference
		}
		oldID := strings.TrimPrefix(parsed.EscapedPath(), prefix)
		if !importAssetID.MatchString(oldID) {
			return reference
		}
		index := strings.Index(reference, prefix)
		oldOrigin := reference[:index]
		if oldOrigin != "" {
			comparison := oldOrigin
			if strings.HasPrefix(comparison, "//") {
				comparison = sourceURL.Scheme + ":" + comparison
			}
			if artifactBundleOrigin(comparison) != sourceOrigin {
				return reference
			}
		}
		id, ok := newIDs[oldID]
		if !ok {
			referenceErr = fmt.Errorf("bundle is missing referenced asset %s", oldID)
			return reference
		}
		newPath := "/a/" + result.ID + "/assets/" + id
		suffix := reference[index+len(prefix)+len(oldID):]
		if oldOrigin != "" {
			return strings.TrimRight(renderOrigin, "/") + newPath + suffix
		}
		return newPath + suffix
	}
	rewrite := func(content, contentType string) string {
		var result strings.Builder
		scanner := newArtifactBundleReferenceScanner(content, contentType)
		copied := 0
		for reference, ok := scanner.next(); ok; reference, ok = scanner.next() {
			result.WriteString(content[copied:reference.start])
			result.WriteString(rewriteReference(reference.url))
			result.WriteString(reference.separator)
			copied = reference.end
		}
		if scanner.err != nil {
			referenceErr = scanner.err
		}
		result.WriteString(content[copied:])
		return result.String()
	}
	source := rewrite(string(bundle.source), "text/html")
	result.SourceText = store.ExtractSearchText(source)
	content := map[string][]byte{result.BodyBlobID: []byte(source)}
	if m.Widget != nil {
		result.WidgetBlobID = uuid.NewString()
		content[result.WidgetBlobID] = []byte(rewrite(string(bundle.widget), "text/html"))
	}
	for i, asset := range m.Assets {
		payload := bundle.assets[i]
		if textualAsset(strings.ToLower(asset.ContentType)) {
			payload = []byte(rewrite(string(payload), asset.ContentType))
		}
		blobID := store.AssetBlobID(ownerID, payload)
		content[blobID] = payload
		result.Assets = append(result.Assets, store.ArtifactAsset{ID: newIDs[asset.ID], BlobID: blobID,
			SourceURL: rewrite(asset.SourceURL, ""), ContentType: asset.ContentType, SizeBytes: int64(len(payload))})
	}
	return result, content, referenceErr
}

func (ro *Router) storeArtifactBundle(ctx context.Context, ownerID int64, bundle store.ArtifactBundleImport, content map[string][]byte) error {
	sizes := make(map[string]int64, len(content))
	ids := make([]string, 0, len(content))
	for id, body := range content {
		sizes[id] = int64(len(body))
		ids = append(ids, id)
	}
	unlockStorage, err := ro.beginStorageWrite(ctx, ownerID, sizes)
	if err != nil {
		return err
	}
	defer unlockStorage()
	err = func() error {
		unlock := ro.cfg.Store.LockBlobs(ids...)
		defer unlock()
		if _, err := ro.cfg.Store.QueueUnreferencedBlobs(ctx, ids); err != nil {
			return err
		}
		for id, body := range content {
			if err := ro.putBlob(ctx, id, bytes.NewReader(body)); err != nil {
				return err
			}
		}
		return ro.cfg.Store.ImportArtifactBundle(ctx, ownerID, bundle)
	}()
	if cleanupErr := ro.reclaimBlobs(context.WithoutCancel(ctx), ids); cleanupErr != nil {
		slog.WarnContext(ctx, "reclaim staged artifact bundle", slog.String("artifact_id", bundle.ID), slog.Any("error", cleanupErr))
	}
	return err
}
