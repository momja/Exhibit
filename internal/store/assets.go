// Out-of-line artifact assets (av-20fk).
//
// The binary payloads a page fetches at run time — a wasm module, an
// Emscripten .data heap — used to live inside the artifact body as base64
// data: URIs. That cost ~1.33x their size in every place the body travels: the
// agent's context on each read and each write, CodeMirror, and the wire on
// every single render, since the render document is necessarily no-store.
//
// Here they are blobs of their own, and the artifact body keeps the original
// fetch literals it was ingested with. Nothing is substituted at ingest; the
// render surface injects a manifest that redirects the fetch at call time. So
// an agent rewriting the whole document cannot break asset loading, because
// there is nothing in the document to break.
//
// Deletability is the subtle part, and the rule is that only questions we
// recorded the answer to may authorize a delete:
//
//   - the artifact is gone — certain, and handled by DeleteArtifact;
//   - a newer generation superseded this one — certain, because generations
//     are ids we minted (ReplaceArtifactAssets);
//   - the owner asked — certain (DeleteArtifactAsset).
//
// And never "the body no longer fetches this URL". Every asset does originate
// from a fetch literal, so that much is discoverable — but the render wrapper
// matches on the *resolved* URL at call time, so a body rewritten to build the
// same URL from parts still consumes an asset whose original literal is gone.
// A scan proving the literal vanished does not prove the fetch did.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ArtifactAsset is one out-of-line payload belonging to an artifact.
type ArtifactAsset struct {
	ID           string    `json:"id"`
	ArtifactID   string    `json:"artifact_id"`
	GenerationID string    `json:"generation_id"`
	SourceURL    string    `json:"source_url"`
	BlobID       string    `json:"-"`
	ContentType  string    `json:"content_type"`
	SizeBytes    int64     `json:"size_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}

// AssetBlobID is the content address of an asset's bytes, scoped to one owner.
//
// Per-owner and deliberately not global. Deduplication inside a library is
// free and useful — the same ffmpeg.wasm ingested five times is one blob — but
// sharing bytes *across* owners would make deleting one account able to strip
// the payload out of another's artifact unless the refcount is exactly right in
// every delete path, forever. Scoping the address removes that failure mode by
// construction, at the cost of duplicate storage on multi-user instances and
// nothing at all on a single-user one.
func AssetBlobID(ownerID int64, body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("asset-%d-%s", ownerID, hex.EncodeToString(sum[:]))
}

// NewAssetID mints the random, unguessable id an asset is served under.
//
// This id is the credential on the render surface's asset route: those bytes
// are served without a render token, because a short-lived token in the URL
// would change that URL on every render and destroy the cross-view caching
// that is half the reason the assets left the body. Reading one therefore
// takes both the artifact id and this id, neither of which is guessable, and
// the response is opaque binary content carrying no state and no policy.
func NewAssetID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewGenerationID mints the id grouping every asset produced by one ingest or
// refetch. Assets change only when the *source* is re-fetched — never on an
// ordinary body edit — which is why generations stay rare and need no pruning
// policy of their own.
func NewGenerationID() (string, error) { return NewAssetID() }

// ReplaceArtifactAssets makes assets the artifact's current generation and
// retires whatever preceded it, returning the blob ids it enqueued for
// deletion (blobqueue.go) for the caller to drain.
//
// Replacing rather than appending is what makes a refetch idempotent in
// storage terms: [[av-b17a]]'s path would otherwise accumulate a full asset set
// every time it ran. The superseded set is safe to retire as a unit because the
// body it belonged to has been replaced too — this is a claim about the
// document's lifetime, not about its contents, which is exactly why it can be
// made without reading a line of the artifact's code.
//
// The delete, the inserts and the refcount all share one transaction, so the
// intent to delete the old bytes becomes durable at the same instant the last
// row naming them disappears.
func (s *SQLiteStore) ReplaceArtifactAssets(ctx context.Context, ownerID int64, artifactID, generationID string, assets []ArtifactAsset) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	// Owner scoping is enforced here rather than on each statement below: an
	// asset row's owner is its artifact's, and confirming the artifact exists
	// for this owner is the one place that can be checked.
	var exists int
	err = tx.QueryRowContext(ctx,
		"SELECT 1 FROM artifacts WHERE id=? AND owner_id=?", artifactID, ownerID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	superseded, err := assetBlobIDs(ctx, tx, artifactID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM artifact_assets WHERE artifact_id=?", artifactID); err != nil {
		return nil, err
	}

	for _, a := range assets {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO artifact_assets
             (id, artifact_id, generation_id, source_url, blob_id, content_type, size_bytes)
             VALUES (?, ?, ?, ?, ?, ?, ?)`,
			a.ID, artifactID, generationID, a.SourceURL, a.BlobID, a.ContentType, a.SizeBytes); err != nil {
			return nil, fmt.Errorf("insert asset %s: %w", a.SourceURL, err)
		}
	}

	// After the inserts, so a payload carried over unchanged between
	// generations counts as still referenced and its bytes are never queued.
	queued, err := enqueueUnreferencedBlobs(ctx, tx, superseded...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return queued, nil
}

// ListArtifactAssets returns one artifact's assets, newest generation first by
// insertion order. It drives the edit page's asset panel and the metadata the
// agent is given in place of the bytes.
func (s *SQLiteStore) ListArtifactAssets(ctx context.Context, ownerID int64, artifactID string) ([]ArtifactAsset, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.artifact_id, a.generation_id, a.source_url, a.blob_id,
                a.content_type, a.size_bytes, a.created_at
           FROM artifact_assets a
           JOIN artifacts art ON art.id = a.artifact_id
          WHERE a.artifact_id = ? AND art.owner_id = ?
          ORDER BY a.created_at, a.source_url`, artifactID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssets(rows)
}

// ArtifactAssetsUnscoped returns one artifact's assets without an owner.
//
// This is the render surface's read, and one of the deliberate unscoped
// accessors named in architecture.md §3.3 — the share path serves an artifact
// to someone who has no account at all, so there is no owner to scope by. It
// leaks nothing an artifact's own render does not already expose: these are
// precisely the bytes the document is about to fetch.
func (s *SQLiteStore) ArtifactAssetsUnscoped(ctx context.Context, artifactID string) ([]ArtifactAsset, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, artifact_id, generation_id, source_url, blob_id,
                content_type, size_bytes, created_at
           FROM artifact_assets
          WHERE artifact_id = ?
          ORDER BY created_at, source_url`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssets(rows)
}

// GetArtifactAssetUnscoped resolves one asset for the render surface's asset
// route, which serves it to whoever holds the id.
//
// The artifact id is required and checked, so an asset can only ever be
// reached through the artifact that owns it — one artifact cannot address
// another's bytes even knowing both ids. Unscoped for the same reason as
// ArtifactAssetsUnscoped: shares have no owner.
func (s *SQLiteStore) GetArtifactAssetUnscoped(ctx context.Context, artifactID, assetID string) (*ArtifactAsset, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, artifact_id, generation_id, source_url, blob_id,
                content_type, size_bytes, created_at
           FROM artifact_assets
          WHERE artifact_id = ? AND id = ?`, artifactID, assetID)

	var a ArtifactAsset
	var createdAt any
	if err := row.Scan(&a.ID, &a.ArtifactID, &a.GenerationID, &a.SourceURL,
		&a.BlobID, &a.ContentType, &a.SizeBytes, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	a.CreatedAt = anyToTime(createdAt)
	return &a, nil
}

// DeleteArtifactAsset removes one asset at the owner's request and returns the
// blob ids it enqueued.
//
// This is the escape hatch for the one case no rule can decide: the owner
// edited away the feature that used a payload, and only they can know it is
// dead. Deleting one that is still in use breaks the artifact at render, which
// is why the panel above this shows each asset's source URL — that is what a
// person matches against their own code.
func (s *SQLiteStore) DeleteArtifactAsset(ctx context.Context, ownerID int64, artifactID, assetID string) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var blobID string
	err = tx.QueryRowContext(ctx,
		`SELECT a.blob_id FROM artifact_assets a
           JOIN artifacts art ON art.id = a.artifact_id
          WHERE a.id = ? AND a.artifact_id = ? AND art.owner_id = ?`,
		assetID, artifactID, ownerID).Scan(&blobID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM artifact_assets WHERE id=? AND artifact_id=?", assetID, artifactID); err != nil {
		return nil, err
	}
	queued, err := enqueueUnreferencedBlobs(ctx, tx, blobID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return queued, nil
}

// assetBlobIDs reads the blob ids an artifact's assets name, inside a caller's
// transaction and before the rows are removed. Once they are gone nothing can
// reconstruct the list, which is why every delete path reads it first.
func assetBlobIDs(ctx context.Context, tx *sql.Tx, artifactID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT blob_id FROM artifact_assets WHERE artifact_id = ?", artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanAssets(rows *sql.Rows) ([]ArtifactAsset, error) {
	var out []ArtifactAsset
	for rows.Next() {
		var a ArtifactAsset
		var createdAt any
		if err := rows.Scan(&a.ID, &a.ArtifactID, &a.GenerationID, &a.SourceURL,
			&a.BlobID, &a.ContentType, &a.SizeBytes, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt = anyToTime(createdAt)
		out = append(out, a)
	}
	return out, rows.Err()
}
