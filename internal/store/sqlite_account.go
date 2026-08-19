// Erasing an account and the library it owns (av-4wyq, epic av-g2dx).
//
// This is sqlite_users.go's opposite number the way /profile is admin.go's: the
// mutators there are an admin acting on somebody else's account, and the one
// here is a person acting on their own. It is also the most destructive
// statement group in the system, so it is written in one place, against the
// schema as it actually is, with a test that fails when the schema grows a
// table this file has not been taught about (sqlite_account_test.go).
package store

import (
	"context"
	"database/sql"
)

// AccountSummary is what deleting an account would destroy, counted so the
// confirmation can say it rather than gesture at it.
//
// Shares are here for a reason the other counts do not share. An artifact and
// its state belong to the person deleting them; a share is a capability URL
// somebody *else* may be holding, with no account on this instance and no way
// to be told it stopped working. Revoking them all at once is the right
// behaviour — the alternative is orphaned links to a library that no longer
// exists — but it is the one consequence of this operation that lands on a
// third party, so the number is surfaced instead of discovered.
//
// StorageBytes is here for a plainer reason (av-fw1b): it is the size of what
// is about to be erased, and it is the same number /profile shows in ordinary
// use. Nothing refuses on it.
type AccountSummary struct {
	Artifacts int64 `json:"artifacts"`
	Shares    int64 `json:"shares"`
	// StorageBytes is the account's total stored bytes — bodies and widgets
	// today, and whatever else blob_references grows to name. Derived from
	// the rows, so deleting the account takes it to zero by construction
	// rather than by a decrement anybody has to remember.
	StorageBytes int64 `json:"storage_bytes"`
}

// GetAccountSummary counts the owner's artifacts, the live shares over them,
// and the bytes they are holding.
func (s *SQLiteStore) GetAccountSummary(ctx context.Context, userID int64) (AccountSummary, error) {
	var sum AccountSummary
	err := s.db.QueryRowContext(ctx, `
        SELECT (SELECT COUNT(*) FROM artifacts WHERE owner_id = ?1),
               (SELECT COUNT(*) FROM shares
                 WHERE artifact_id IN (SELECT id FROM artifacts WHERE owner_id = ?1))`,
		userID).Scan(&sum.Artifacts, &sum.Shares)
	if err != nil {
		return sum, err
	}
	sum.StorageBytes, err = s.StorageUsage(ctx, userID)
	return sum, err
}

// DeleteAccount erases everything this instance holds for userID and returns
// the blob ids whose bytes the caller must now remove.
//
// # Why it returns blob ids rather than deleting them
//
// The store owns rows; the blob store owns bytes, behind its own interface
// (architecture §3.3). Collecting the ids *inside* the transaction is what
// makes the pair reliable: an artifact created between a separate "list my
// blobs" call and this one would have its row deleted here and its bytes
// missed, and there is no later reader that could ever find them again.
//
// The caller deletes the bytes after this returns, which is the same row-first
// order deleteArtifactBlobs takes and for the same reason: bytes-first risks a
// live row pointing at a body that no longer exists, and only that failure is
// unrepairable.
//
// # What it deletes, and what deletes itself
//
// Written against the schema rather than a remembered list, and leaning on the
// cascades that already exist instead of duplicating them — a duplicate delete
// is a second place to update when the schema moves, and the one that gets
// forgotten is the one that leaves data behind.
//
//   - `artifacts` (owner_id) — takes artifact_tags, artifact_collections,
//     artifact_network_origins, shares, agent_transcripts and artifact_state
//     with it by ON DELETE CASCADE, and keeps artifacts_fts in step through
//     the triggers migration 010 installed.
//   - `tags`, `collections`, `agent_keys` (owner_id) — no cascade reaches
//     these, since nothing about an artifact owns them; deleted here.
//   - `blob_sizes` (blob_id) — the recorded length of each blob the account's
//     rows named, deleted last and only for ids no surviving row references
//     (av-fw1b). Not owner-scoped: a length is a fact about bytes, and with
//     refcounted shared assets the same id can be named by another owner.
//   - `users` (id) — takes `sessions` by ON DELETE CASCADE, which is what
//     signs the account out everywhere rather than only in the browser that
//     asked, and takes every `artifact_state` row this user wrote through
//     migration 014's AFTER DELETE trigger. That trigger matters more than it
//     looks: it reaches state rows on **another owner's** artifact, which is
//     the one place a user's data lives outside their own library.
//
// # Refusal
//
// ErrLastAdmin when the account is the instance's only enabled admin, the same
// answer SetUserAdmin and SetUserDisabled give — and for a stronger version of
// the same reason. Demoting the last admin leaves an instance nobody can
// administer; deleting them leaves that *and* no row to promote back. An
// operator who really means it still has the CLI. ErrNotFound when there is no
// such account.
func (s *SQLiteStore) DeleteAccount(ctx context.Context, userID int64) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	// The blob ids first, while the rows that name them still exist.
	blobIDs, err := artifactBlobIDs(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	// The users row next, because it carries the guard: if this instance may
	// not lose its last enabled admin, nothing else should have run either.
	// Being inside the transaction makes that true regardless, but failing on
	// the first statement is what keeps the refusal cheap and the reason
	// legible.
	res, err := tx.ExecContext(ctx,
		"DELETE FROM users WHERE id = ?1 AND "+lastEnabledAdminGuard, userID)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		// Nothing was deleted, so the row is still there to be asked about:
		// refused (ErrLastAdmin) or absent (ErrNotFound).
		return nil, refusedOrMissing(ctx, tx, userID)
	}

	// Then the owner-scoped tables no cascade reaches. `artifacts` is first so
	// its cascades run before the tables they might have referenced go.
	for _, stmt := range []string{
		"DELETE FROM artifacts WHERE owner_id = ?",
		"DELETE FROM tags WHERE owner_id = ?",
		"DELETE FROM collections WHERE owner_id = ?",
		"DELETE FROM agent_keys WHERE owner_id = ?",
	} {
		if _, err := tx.ExecContext(ctx, stmt, userID); err != nil {
			return nil, err
		}
	}

	// The lengths recorded for those blobs, last, once every row that could
	// have referenced them is gone — forgetBlobSizesTx keeps any id another
	// owner still references, which is the whole reason it is a reference
	// check rather than a delete by id (av-fw1b). Same argument as the ids
	// themselves: after this commits nothing can name these rows again.
	if err := forgetBlobSizesTx(ctx, tx, blobIDs); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return blobIDs, nil
}

// artifactBlobIDs is every blob id an owner's artifacts name: the body of each,
// plus the widget of any that has one. Both are rewritten in place across the
// artifact's life (an edit reuses source_blob_id, a widget save reuses
// widget_blob_id), so this is the complete set and not the newest of a series.
func artifactBlobIDs(ctx context.Context, tx *sql.Tx, userID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT source_blob_id, widget_blob_id FROM artifacts WHERE owner_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var body, widget string
		if err := rows.Scan(&body, &widget); err != nil {
			return nil, err
		}
		if body != "" {
			ids = append(ids, body)
		}
		if widget != "" {
			ids = append(ids, widget)
		}
	}
	return ids, rows.Err()
}
