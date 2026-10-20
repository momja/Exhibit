package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
)

type duplicateArtifactRequest struct {
	Title             string `json:"title"`
	CopyShareSettings bool   `json:"copy_share_settings"`
	CopyData          bool   `json:"copy_data"`
	RequestID         string `json:"request_id"`
}

// The request identity survives a lost response without needing another table:
// the clone's primary key is the receipt. Including owner and source prevents
// another account or another source from claiming the same receipt.
func duplicateArtifactID(ownerID int64, sourceID, requestID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("exhibit-duplicate:%d:%s:%s", ownerID, sourceID, requestID))).String()
}

func (ro *Router) duplicateArtifact(w http.ResponseWriter, r *http.Request) {
	var req duplicateArtifactRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	requestID, err := uuid.Parse(req.RequestID)
	if err != nil || requestID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "request_id must be a nonzero UUID")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	ownerID := ownerIDFromCtx(r.Context())
	sourceID := urlParamID(r, "artifactID")
	id := duplicateArtifactID(ownerID, sourceID, requestID.String())
	// A replay returns the saved result, even if the original was deleted or
	// changed after success. No quotas or source reads can turn it into a copy.
	existing, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "lookup duplicate receipt", err)
		return
	}
	if existing != nil {
		writeJSON(w, http.StatusOK, existing)
		return
	}
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, sourceID)
	if err != nil {
		serverError(w, r, "duplicate source lookup", err)
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	assets, err := ro.cfg.Store.ListArtifactAssets(r.Context(), ownerID, sourceID)
	if err != nil {
		serverError(w, r, "duplicate asset lookup", err)
		return
	}
	d := store.ArtifactDuplicate{
		ID: id, Title: req.Title, BodyBlobID: uuid.NewString(), SourceVersion: a.VersionSeq,
		SourceAssets: assets, CopyData: req.CopyData, CopyShareSettings: req.CopyShareSettings,
	}
	d.GenerationID, err = store.NewGenerationID()
	if err != nil {
		serverError(w, r, "duplicate generation identity", err)
		return
	}
	blobs, err := ro.duplicateContent(r.Context(), ownerID, a, &d)
	if err != nil {
		serverError(w, r, "prepare duplicate content", err)
		return
	}
	created, err := ro.storeDuplicate(r.Context(), ownerID, sourceID, d, blobs)
	if err != nil {
		// A simultaneous retry can lose a quota race after the first request
		// committed. Its existing receipt is success, regardless of the later
		// staging failure, and must not invite creation with another identity.
		if saved, lookupErr := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id); lookupErr == nil && saved != nil {
			writeJSON(w, http.StatusOK, saved)
			return
		}
		if err == store.ErrArtifactChanged {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeArtifactError(w, r, "duplicate artifact", err)
		return
	}
	result, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "read duplicate result", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, result)
}

func (ro *Router) storeDuplicate(ctx context.Context, ownerID int64, sourceID string, d store.ArtifactDuplicate, content map[string]string) (bool, error) {
	sizes := make(map[string]int64, len(content))
	for id, body := range content {
		sizes[id] = int64(len(body))
	}
	for _, asset := range d.Assets {
		sizes[asset.BlobID] = asset.SizeBytes
	}
	ids := make([]string, 0, len(sizes))
	for id := range sizes {
		ids = append(ids, id)
	}
	unlockStorage, err := ro.beginStorageWrite(ctx, ownerID, sizes)
	if err != nil {
		return false, err
	}
	defer unlockStorage()
	// Queue before writing, in all deployment modes: a failure cannot leave
	// either a partial artifact or bytes that no future cleanup knows about.
	created, err := func() (bool, error) {
		unlock := ro.cfg.Store.LockBlobs(ids...)
		defer unlock()
		if _, err := ro.cfg.Store.QueueUnreferencedBlobs(ctx, ids); err != nil {
			return false, err
		}
		for id, body := range content {
			if err := ro.putBlob(ctx, id, strings.NewReader(body)); err != nil {
				return false, err
			}
		}
		return ro.cfg.Store.DuplicateArtifact(ctx, ownerID, sourceID, d)
	}()
	// Cleanup errors after commit cannot turn success into a failed mutation.
	// The durable queue and reference check make this safe for shared assets.
	if cleanupErr := ro.reclaimBlobs(context.WithoutCancel(ctx), ids); cleanupErr != nil {
		slog.WarnContext(ctx, "reclaim staged duplicate blobs", slog.String("artifact_id", d.ID), slog.Any("error", cleanupErr))
	}
	return created, err
}
