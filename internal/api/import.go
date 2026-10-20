package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/momja/Exhibit/internal/store"
)

func (ro *Router) importState(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	if !ro.importTargetExists(w, r, id) {
		return
	}
	body, ok := readImportFile(w, r)
	if !ok {
		return
	}
	state, err := readStateImport(body)
	if err != nil {
		if errors.Is(err, store.ErrImportStateTooLarge) {
			writeImportError(w, r, err)
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	err = ro.cfg.Store.ImportState(r.Context(), ownerIDFromCtx(r.Context()), id, state, r.URL.Query().Get("replace") == "true")
	if writeImportError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Decode one flat object incrementally. Reject nested values and too many
// entries before allocating them or taking the database's write connection.
func readStateImport(body []byte) (map[string]string, error) {
	invalid := errors.New("Choose a JSON object of string values exported from Stored state.")
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, invalid
	}
	state := make(map[string]string)
	for decoder.More() {
		if len(state) >= store.MaxImportedStateKeys {
			return nil, store.ErrImportStateTooLarge
		}
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, invalid
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, invalid
		}
		if _, duplicate := state[key]; duplicate {
			return nil, errors.New("state import contains a duplicate key")
		}
		valueToken, err := decoder.Token()
		value, ok := valueToken.(string)
		if err != nil || !ok {
			return nil, invalid
		}
		state[key] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return nil, invalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, invalid
	}
	return state, nil
}

func (ro *Router) importTargetExists(w http.ResponseWriter, r *http.Request, id string) bool {
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), id)
	if err != nil {
		serverError(w, r, "import target lookup", err)
		return false
	}
	if a == nil {
		http.NotFound(w, r)
		return false
	}
	return true
}

func writeImportError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrImportConfirmation) {
		writeJSON(w, http.StatusConflict, struct {
			Error                string `json:"error"`
			ConfirmationRequired bool   `json:"confirmation_required"`
		}{err.Error(), true})
	} else if errors.Is(err, store.ErrImportAssetConflict) {
		writeError(w, http.StatusBadRequest, err.Error())
	} else if errors.Is(err, store.ErrImportStateTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	} else {
		writeArtifactError(w, r, "import stored data", err)
	}
	return true
}

func (ro *Router) importAssets(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	if !ro.importTargetExists(w, r, id) {
		return
	}
	body, ok := readImportFile(w, r)
	if !ok {
		return
	}
	assets, err := readAssetsImport(body, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	confirmed := r.URL.Query().Get("replace") == "true"
	// Avoid staging bytes before showing the prompt. The store repeats this
	// check in the replacement transaction, covering a concurrent first write.
	if !confirmed {
		current, err := ro.cfg.Store.ListArtifactAssets(r.Context(), ownerIDFromCtx(r.Context()), id)
		if err != nil {
			serverError(w, r, "import assets lookup", err)
			return
		}
		if len(current) > 0 {
			writeImportError(w, r, store.ErrImportConfirmation)
			return
		}
	}
	if writeImportError(w, r, ro.storeImportedAssets(r.Context(), ownerIDFromCtx(r.Context()), id, assets, confirmed)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var importAssetID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// The archive is never extracted to disk. Bound the expanded data as well as
// the HTTP upload, validate every entry/CRC before staging, and retain ids so
// markup references continue to load the restored payloads.
func readAssetsImport(body []byte, artifactID string) ([]snapshot.RuntimeAsset, error) {
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("Choose a ZIP exported from Stored assets: %w", err)
	}
	limits := snapshot.DefaultLimits()
	// Each fetched URL can produce a markup row and a runtime row. Bound the
	// whole exported archive, including both collectors, without splitting it.
	maxAssets := 2 * limits.MaxAssets
	if len(archive.File) > maxAssets+1 {
		return nil, fmt.Errorf("asset archive has too many entries (maximum %d assets)", maxAssets)
	}
	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		if !fs.ValidPath(file.Name) || strings.Contains(file.Name, `\`) || !file.Mode().IsRegular() || files[file.Name] != nil {
			return nil, fmt.Errorf("invalid or duplicate archive path: %q", file.Name)
		}
		files[file.Name] = file
	}
	manifestFile := files["manifest.json"]
	if manifestFile == nil {
		return nil, errors.New("asset archive is missing manifest.json")
	}
	manifestBody, err := readImportEntry(manifestFile, 1<<20)
	if err != nil {
		return nil, err
	}
	var manifest assetsManifest
	if err := json.Unmarshal(manifestBody, &manifest); err != nil || manifest.Assets == nil {
		return nil, errors.New("invalid asset manifest")
	}
	if manifest.ArtifactID != artifactID {
		return nil, errors.New("This asset ZIP belongs to a different artifact. Choose an export of this artifact.")
	}
	if len(manifest.Assets) > maxAssets || len(files) != len(manifest.Assets)+1 {
		return nil, errors.New("archive entries do not match its manifest")
	}
	// Markup and runtime collection can produce distinct IDs for one source
	// URL, so validate identity and archive paths without deduplicating URLs.
	seenIDs, seenPaths := map[string]bool{}, map[string]bool{}
	assets := make([]snapshot.RuntimeAsset, 0, len(manifest.Assets))
	remaining := 2 * limits.MaxTotalBytes
	uniqueRemaining := limits.MaxTotalBytes
	payloads := make(map[[32]byte][]byte)
	for _, asset := range manifest.Assets {
		source, err := url.Parse(asset.SourceURL)
		if !importAssetID.MatchString(asset.ID) || seenIDs[asset.ID] || seenPaths[asset.Path] ||
			err != nil || source.Host == "" || (source.Scheme != "https" && source.Scheme != "http") ||
			asset.ContentType == "" || strings.ContainsAny(asset.ContentType, "\r\n") ||
			!strings.HasPrefix(asset.Path, "assets/"+asset.ID+"/") || files[asset.Path] == nil || asset.SizeBytes < 0 {
			return nil, errors.New("invalid or duplicate asset metadata in manifest")
		}
		payload, err := readImportEntry(files[asset.Path], remaining)
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
		seenIDs[asset.ID], seenPaths[asset.Path] = true, true
		assets = append(assets, snapshot.RuntimeAsset{AssetID: asset.ID, SourceURL: asset.SourceURL, ContentType: asset.ContentType, Body: payload})
	}
	return assets, nil
}

func readImportEntry(file *zip.File, limit int64) ([]byte, error) {
	if file.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("expanded asset archive exceeds its size limit at %s", file.Name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("invalid archive entry %s: %w", file.Name, err)
	}
	if int64(len(body)) > limit {
		return nil, errors.New("expanded asset archive exceeds its size limit")
	}
	// archive/zip skips its CRC comparison when the stored checksum is zero.
	// Zero is a valid checksum too, so imports compare it unconditionally.
	if crc32.ChecksumIEEE(body) != file.CRC32 {
		return nil, fmt.Errorf("invalid archive entry %s: checksum mismatch", file.Name)
	}
	return body, nil
}

func (ro *Router) storeImportedAssets(ctx context.Context, ownerID int64, id string, assets []snapshot.RuntimeAsset, confirmed bool) error {
	blobs := snapshotStorageBlobs(ownerID, "", "", assets)
	unlockStorage, err := ro.beginAssetReplacementStorageWrite(ctx, ownerID, id, blobs)
	if err != nil {
		return err
	}
	defer unlockStorage()
	generation, err := store.NewGenerationID()
	if err != nil {
		return err
	}
	staged := make([]string, 0, len(blobs))
	for blobID := range blobs {
		staged = append(staged, blobID)
	}
	queued, err := func() ([]string, error) {
		unlock := ro.cfg.Store.LockBlobs(staged...)
		defer unlock()
		if _, err := ro.cfg.Store.QueueUnreferencedBlobs(ctx, staged); err != nil {
			return nil, err
		}
		rows, err := ro.writeRuntimeAssets(ctx, ownerID, assets)
		if err != nil {
			return nil, err
		}
		return ro.cfg.Store.ImportArtifactAssets(ctx, ownerID, id, generation, rows, confirmed)
	}()
	if cleanupErr := ro.reclaimBlobs(context.WithoutCancel(ctx), append(queued, staged...)); cleanupErr != nil {
		slog.WarnContext(ctx, "reclaim imported asset blobs", slog.String("err", cleanupErr.Error()))
	}
	return err
}
