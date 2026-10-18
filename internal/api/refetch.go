package api

import (
	"bytes"
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/momja/Exhibit/internal/store"
)

// storeRefetchedSnapshot stages every blob before atomically switching the
// body and asset rows. A failed write or transaction leaves the old snapshot
// intact; its unused staged blobs go through the reference-checked drain.
func (ro *Router) storeRefetchedSnapshot(ctx context.Context, ownerID int64, id, body string, bodyChanged bool, collected []snapshot.RuntimeAsset, provenance ...store.Provenance) error {
	replacement := store.ArtifactSnapshot{Provenance: store.Provenance{Origin: store.VersionRefetch}}
	if len(provenance) > 0 {
		replacement.Provenance = provenance[0]
	}
	var staged []string
	if bodyChanged {
		replacement.SourceBlobID = uuid.New().String()
		replacement.SourceText = store.ExtractSearchText(body)
		staged = append(staged, replacement.SourceBlobID)
	}
	for _, asset := range collected {
		staged = append(staged, store.AssetBlobID(ownerID, asset.Body))
	}
	queued, err := func() ([]string, error) {
		unlock := ro.cfg.Store.LockBlobs(staged...)
		defer unlock()
		if _, err := ro.cfg.Store.QueueUnreferencedBlobs(ctx, staged); err != nil {
			return nil, err
		}
		if bodyChanged {
			if err := putBlob(ctx, ro.cfg.Store, ro.cfg.Blob, replacement.SourceBlobID, bytes.NewReader([]byte(body))); err != nil {
				return nil, err
			}
		}
		if len(collected) > 0 {
			var err error
			replacement.GenerationID, err = store.NewGenerationID()
			if err != nil {
				return nil, err
			}
			replacement.Assets, err = ro.writeRuntimeAssets(ctx, ownerID, collected)
			if err != nil {
				return nil, err
			}
		}
		return ro.cfg.Store.ReplaceArtifactSnapshot(ctx, ownerID, id, replacement)
	}()
	if err != nil {
		var queueErr error
		queued, queueErr = ro.cfg.Store.QueueUnreferencedBlobs(context.WithoutCancel(ctx), staged)
		if queueErr != nil {
			slog.WarnContext(ctx, "queue unused refetch blobs", slog.String("err", queueErr.Error()))
		}
	}
	// Release the writer locks before draining: the drain takes those locks
	// too. Cleanup must still run when the request was cancelled.
	if cleanupErr := ro.reclaimBlobs(context.WithoutCancel(ctx), append(queued, staged...)); cleanupErr != nil {
		slog.WarnContext(ctx, "reclaim refetch blobs",
			slog.String("artifact_id", id), slog.String("err", cleanupErr.Error()))
	}
	return err
}
