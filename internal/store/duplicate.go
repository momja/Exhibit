package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrArtifactChanged refuses a clone assembled from a source whose content or
// assets changed before commit. The caller can safely retry from a fresh read.
var ErrArtifactChanged = errors.New("artifact changed while duplicating; try again")

// ArtifactDuplicate contains staged immutable content, not metadata supplied by
// the client. Metadata, optional owner state and shares are copied inside the
// transaction that creates the artifact. SourceAssets and SourceVersion pin the
// content read before staging; histories and conversations are never copied.
type ArtifactDuplicate struct {
	ID, Title, BodyBlobID, WidgetBlobID, SourceText, GenerationID string
	SourceVersion                                                 int
	SourceAssets, Assets                                          []ArtifactAsset
	CopyData, CopyShareSettings                                   bool
}

// DuplicateArtifact publishes a complete clone or nothing. A repeated ID is
// successful without modifying the already-created artifact, including after
// the source has been removed. The API derives this ID from the authenticated
// owner, source and a random request ID so retries cannot create more copies.
func (s *SQLiteStore) DuplicateArtifact(ctx context.Context, ownerID int64, sourceID string, d ArtifactDuplicate) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)`, d.ID, ownerID).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	a, err := scanArtifact(tx.QueryRowContext(ctx, `SELECT `+artifactCols+` FROM artifacts WHERE id=? AND owner_id=?`, sourceID, ownerID))
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, artifact_id, generation_id, source_url, blob_id, content_type, size_bytes, created_at FROM artifact_assets WHERE artifact_id=? ORDER BY created_at, source_url`, sourceID)
	if err != nil {
		return false, err
	}
	assets, err := scanAssets(rows)
	rows.Close()
	if err != nil {
		return false, err
	}
	if a.VersionSeq != d.SourceVersion || !sameDuplicateSourceAssets(assets, d.SourceAssets) {
		return false, ErrArtifactChanged
	}

	// Let the initial-version trigger establish the clone's own history. Sharing
	// indicators are derived on read; searchable tags keep their existing triggers.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO artifacts
	 (id, owner_id, title, source_blob_id, source_url, tier, downloads_approved, clipboard_approved,
	 links_approved, camera_approved, microphone_approved, geolocation_approved, widget_blob_id, source_text, share_state_mode, created_at, updated_at)
	 SELECT ?, owner_id, ?, ?, source_url, tier, downloads_approved, clipboard_approved,
	 links_approved, camera_approved, microphone_approved, geolocation_approved, ?, ?, CASE WHEN ? THEN share_state_mode ELSE 'own' END, ?, ?
	 FROM artifacts WHERE id=? AND owner_id=?`, d.ID, d.Title, d.BodyBlobID, d.WidgetBlobID, d.SourceText, d.CopyShareSettings, now, now, sourceID, ownerID)
	if err != nil {
		return false, err
	}
	for _, statement := range []string{
		`INSERT INTO artifact_network_origins (artifact_id, origin, decision, source) SELECT ?, origin, decision, source FROM artifact_network_origins WHERE artifact_id=?`,
		`INSERT INTO artifact_tags (artifact_id, tag_id) SELECT ?, tag_id FROM artifact_tags WHERE artifact_id=?`,
		`INSERT INTO artifact_collections (artifact_id, collection_id) SELECT ?, collection_id FROM artifact_collections WHERE artifact_id=?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, d.ID, sourceID); err != nil {
			return false, err
		}
	}
	if _, err := replaceArtifactAssets(ctx, tx, d.ID, d.GenerationID, d.Assets); err != nil {
		return false, err
	}
	if d.CopyData {
		if _, err := tx.ExecContext(ctx, `INSERT INTO artifact_state (artifact_id, user_id, key, value) SELECT ?, user_id, key, value FROM artifact_state WHERE artifact_id=? AND user_id=?`, d.ID, sourceID, ownerID); err != nil {
			return false, err
		}
	}
	if d.CopyShareSettings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO share_grants (artifact_id, recipient_id, created_at)
            SELECT ?, recipient_id, created_at FROM share_grants WHERE artifact_id=?`, d.ID, sourceID); err != nil {
			return false, err
		}
		id := uuid.New().String()
		if _, err := tx.ExecContext(ctx, `INSERT INTO share_links (id, artifact_id)
            SELECT ?, ? FROM share_links WHERE artifact_id=?`, id, d.ID, sourceID); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// Asset row order is immaterial; two URLs can tie on every ORDER BY column.
func sameDuplicateSourceAssets(current, expected []ArtifactAsset) bool {
	if len(current) != len(expected) {
		return false
	}
	byID := make(map[string]ArtifactAsset, len(expected))
	for _, asset := range expected {
		byID[asset.ID] = asset
	}
	for _, asset := range current {
		if saved, ok := byID[asset.ID]; !ok || saved != asset {
			return false
		}
	}
	return true
}
