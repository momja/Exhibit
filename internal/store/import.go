package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrImportConfirmation means replacement would erase existing data. It is
// checked inside the transaction, including for a previously empty panel.
var ErrImportConfirmation = errors.New("import requires confirmation to erase existing data")
var ErrImportAssetConflict = errors.New("an imported asset id already identifies different stored content")

const MaxImportedStateKeys = 10000

var ErrImportStateTooLarge = fmt.Errorf("state import exceeds the maximum of %d keys", MaxImportedStateKeys)

// ImportState replaces only the owner's localStorage map, leaving recipients'
// private state and every other part of the artifact alone.
func (s *SQLiteStore) ImportState(ctx context.Context, ownerID int64, artifactID string, state map[string]string, replace bool) error {
	if len(state) > MaxImportedStateKeys {
		return ErrImportStateTooLarge
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := checkImport(ctx, tx, ownerID, artifactID, replace, "SELECT COUNT(*) FROM artifact_state WHERE artifact_id=? AND user_id=?", artifactID, ownerID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM artifact_state WHERE artifact_id=? AND user_id=?", artifactID, ownerID); err != nil {
		return err
	}
	insert, err := tx.PrepareContext(ctx, "INSERT INTO artifact_state (artifact_id, user_id, key, value) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer insert.Close()
	for key, value := range state {
		if _, err := insert.ExecContext(ctx, artifactID, ownerID, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func checkImport(ctx context.Context, tx *sql.Tx, ownerID int64, artifactID string, replace bool, countQuery string, args ...any) error {
	var exists int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM artifacts WHERE id=? AND owner_id=?", artifactID, ownerID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !replace {
		var count int
		if err := tx.QueryRowContext(ctx, countQuery, args...).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return ErrImportConfirmation
		}
	}
	return nil
}

// ImportArtifactAssets retains archive ids because markup may address them
// directly. An immutable asset URL can never be reassigned to different bytes,
// another artifact, or a different content type, including in retained versions.
func (s *SQLiteStore) ImportArtifactAssets(ctx context.Context, ownerID int64, artifactID, generationID string, assets []ArtifactAsset, replace bool) ([]string, error) {
	return s.replaceOwnedArtifactAssets(ctx, ownerID, artifactID, generationID, assets, &replace)
}
