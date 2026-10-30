package store

import "context"

// RepairConnection replaces only credential material. Identity, policy, grants,
// schedules and quota counters survive. A flow begun before deletion, a policy
// change or another credential replacement cannot resurrect or overwrite it.
func (s *SQLiteStore) RepairConnection(ctx context.Context, expected *ConnectionRecord, encrypted, externalRef string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE connections SET encrypted_secret=?,external_ref=?,status='active'
 WHERE id=? AND owner_id=? AND kind=? AND provider=? AND engine=? AND recipe_json=? AND encrypted_secret=? AND status IN ('active','reconnect')`, encrypted, externalRef, expected.ID, expected.OwnerID, expected.Kind, expected.Provider, expected.Engine, expected.RecipeJSON, expected.EncryptedSecret)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM connection_cache WHERE connection_id=?", expected.ID); err != nil {
		return err
	}
	return tx.Commit()
}
