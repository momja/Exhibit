package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ArtifactBundleImport contains validated, staged content. Imported permissions
// are intentionally absent: new artifacts always start private and unapproved.
type ArtifactBundleImport struct {
	ID, Title, SourceURL, BodyBlobID, WidgetBlobID, SourceText, GenerationID string
	Tier                                                                     Tier
	Assets                                                                   []ArtifactAsset
	State                                                                    map[string]string
}

// ImportArtifactBundle never overwrites an artifact or reassigns an immutable
// public asset identity. The target must still exist and be enabled at commit.
func (s *SQLiteStore) ImportArtifactBundle(ctx context.Context, ownerID int64, bundle ArtifactBundleImport) error {
	if len(bundle.State) > MaxImportedStateKeys {
		return ErrImportStateTooLarge
	}
	if len(bundle.Assets) > 200 {
		return errors.New("artifact bundle contains too many assets")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM users WHERE id=? AND disabled_at IS NULL", ownerID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	// This insert also establishes version 1 with both source and widget
	// references. Assets and live owner state belong to this head, and will be
	// snapshotted by CommitVersion when it is first superseded.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO artifacts
	 (id, owner_id, title, source_url, tier, source_blob_id, widget_blob_id, source_text, created_at, updated_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, bundle.ID, ownerID, bundle.Title, bundle.SourceURL,
		bundle.Tier, bundle.BodyBlobID, bundle.WidgetBlobID, bundle.SourceText, now, now)
	if err != nil {
		return err
	}
	for _, asset := range bundle.Assets {
		var known bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM asset_identities WHERE id=?)", asset.ID).Scan(&known); err != nil {
			return err
		}
		if known {
			return fmt.Errorf("%w: %s", ErrImportAssetConflict, asset.ID)
		}
	}
	if _, err := replaceArtifactAssets(ctx, tx, bundle.ID, bundle.GenerationID, bundle.Assets); err != nil {
		return err
	}
	insert, err := tx.PrepareContext(ctx, "INSERT INTO artifact_state (artifact_id, user_id, key, value) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer insert.Close()
	for key, value := range bundle.State {
		if _, err := insert.ExecContext(ctx, bundle.ID, ownerID, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}
