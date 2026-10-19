package api

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/momja/Exhibit/internal/store"
)

type storageQuotaError struct {
	LimitBytes      int64 `json:"limit_bytes"`
	UsageBytes      int64 `json:"usage_bytes"`
	WriteBytes      int64 `json:"write_bytes"`
	AdditionalBytes int64 `json:"additional_bytes"`
}

func (e *storageQuotaError) Error() string {
	return fmt.Sprintf("storage quota exceeded: limit %d bytes, current usage %d bytes, refused write %d bytes (%d additional bytes)",
		e.LimitBytes, e.UsageBytes, e.WriteBytes, e.AdditionalBytes)
}

func (e *storageQuotaError) respond(w http.ResponseWriter) {
	writeJSON(w, http.StatusRequestEntityTooLarge, struct {
		Error string `json:"error"`
		Code  string `json:"code"`
		*storageQuotaError
	}{e.Error(), "storage_quota_exceeded", e})
}

// snapshotStorageBlobs describes the assembled bytes before any are written.
// Content addresses deduplicate both repeated assets in this write and assets
// this owner already holds (the latter is resolved by ProjectStorageUsage).
func snapshotStorageBlobs(ownerID int64, bodyID, body string, assets []snapshot.RuntimeAsset) map[string]int64 {
	blobs := make(map[string]int64, len(assets)+1)
	if bodyID != "" {
		blobs[bodyID] = int64(len(body))
	}
	for _, asset := range assets {
		blobs[store.AssetBlobID(ownerID, asset.Body)] = int64(len(asset.Body))
	}
	return blobs
}

// beginStorageWrite holds the owner's lock until the caller has committed all
// references. Fetching and assembling snapshots happens before this, so a slow
// source does not block the owner's other saves. Historical versions keep their
// bytes: replacing a body or tile adds its whole size, not the size difference.
func (ro *Router) beginStorageWrite(ctx context.Context, ownerID int64, blobs map[string]int64) (func(), error) {
	noop := func() {}
	if !ro.cfg.Entitlements.Enforced {
		return noop, nil
	}
	unlock := ro.cfg.Store.LockStorage(ownerID)
	allowance, err := ro.resolveAllowance(ctx, ownerID)
	if err != nil {
		unlock()
		return noop, err
	}
	current, additional, err := ro.cfg.Store.ProjectStorageUsage(ctx, ownerID, blobs)
	if err != nil {
		unlock()
		return noop, err
	}
	var size int64
	for _, n := range blobs {
		if n < 0 || n > math.MaxInt64-size {
			unlock()
			return noop, fmt.Errorf("invalid storage write size")
		}
		size += n
	}
	if additional > math.MaxInt64-current || !allowance.Storage.Allows(current+additional) {
		unlock()
		return noop, &storageQuotaError{
			LimitBytes: allowance.Storage.Bytes, UsageBytes: current,
			WriteBytes: size, AdditionalBytes: additional,
		}
	}
	return unlock, nil
}
