// The blob deletion queue (av-8gyd).
//
// Removing an artifact's bytes touches two stores that cannot commit together:
// rows in SQLite and files behind the Blob interface. The row must go first —
// the reverse leaves a live artifact whose only copy of itself is gone, which
// nothing on the instance can repair — and that ordering opens a window where
// a crash used to leak the file permanently, with nothing able to name it
// afterwards.
//
// The answer is not to go looking for strays later. The deleting code already
// knows exactly which blobs it meant to remove, so it writes that down: the
// same transaction that drops the referencing row inserts the blob id here.
// What the commit makes durable is the *intent*, not the outcome; everything
// after it — unlink the file, delete the queue row — is idempotent work, so
// there is no state a crash can leave that a later drain cannot finish.
//
// Two properties follow, and both are the reason this is a queue rather than a
// reconciler that scans the blob store for unreferenced files:
//
//   - It contains only ids something already decided to delete, so a bug in
//     the drain can reach nothing but condemned bytes. A scan infers
//     deletability from a missing reference, and a bug in *that* inference —
//     a table it forgot to join, a query returning nothing under load —
//     deletes live artifacts.
//   - It costs nothing when idle, because it is normally empty. A scan's cost
//     grows with the library.
//
// There is deliberately no ticker, no worker pool and no scheduler: the drain
// runs synchronously after a delete (for that operation's own ids) and over
// the whole queue at startup, which is where a crashed process's leftovers are
// reclaimed. A crashed process gets restarted, so the restart is the natural
// pairing.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// BlobDeleter is the write side of blob.Store, named here so the drain can
// take a blob store without the store package importing one (the same seam
// blobGetter gives BackfillSourceText). blob.Store satisfies it.
//
// Its Delete is idempotent for an id that is not there (av-7jcq), which is
// what lets a drain be repeated: re-running one that already unlinked the file
// but crashed before deleting the queue row costs nothing and needs no
// compensating existence check.
type BlobDeleter interface {
	Delete(ctx context.Context, id string) error
}

// blobReferenceCount counts the rows that still name a blob id. It is the
// refcount the enqueue is conditional on, and the one query to extend when a
// future table starts referencing blobs: a column left out here is a blob
// deleted while something still points at it.
//
// Two tables name blobs today — an artifact's own body and widget, and the
// out-of-line assets of av-20fk. Assets are the reason the count cannot be
// skipped: they are content-addressed per owner, so one library's two
// artifacts that both load the same ffmpeg.wasm share a single blob, and an
// unconditional enqueue on deleting either would strip the payload out of the
// survivor.
const blobReferenceCount = `
    SELECT (SELECT COUNT(*) FROM artifacts
             WHERE source_blob_id = ?1 OR widget_blob_id = ?1)
         + (SELECT COUNT(*) FROM artifact_assets WHERE blob_id = ?1)`

// enqueueUnreferencedBlobs records the intent to delete each of ids whose last
// reference has just gone, and returns the subset it enqueued.
//
// It must be called inside the transaction that removed the referencing rows,
// after those statements have run: the count it takes is of the rows that
// remain, so evaluating it there is what makes the decision race-free and what
// makes "the queue never holds a blob id a live row still names" a property of
// the transaction rather than of the caller's timing.
//
// The count is why enqueuing is conditional at all. Content addressing
// (av-20fk) lets two artifacts in one library legitimately share a blob, and
// an unconditional enqueue would strip the payload out of the artifact that
// survived.
func enqueueUnreferencedBlobs(ctx context.Context, tx *sql.Tx, ids ...string) ([]string, error) {
	var queued []string
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		var refs int
		if err := tx.QueryRowContext(ctx, blobReferenceCount, id).Scan(&refs); err != nil {
			return nil, fmt.Errorf("count references to blob %s: %w", id, err)
		}
		if refs > 0 {
			continue // still somebody's body; the bytes stay
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pending_blob_deletions (blob_id) VALUES (?)
             ON CONFLICT(blob_id) DO NOTHING`, id); err != nil {
			return nil, fmt.Errorf("enqueue blob %s for deletion: %w", id, err)
		}
		queued = append(queued, id)
	}
	return queued, nil
}

// PendingBlobDeletions is the whole queue: every blob id whose bytes are
// condemned but not yet confirmed gone. Ordinary operation leaves it empty, so
// a non-empty read means either a drain in progress or a process that died
// mid-drain.
//
// It is not owner-scoped, and cannot be: a blob id reaches this table only
// once the last row naming it — and therefore the last thing that knew whose
// it was — has been deleted.
func (s *SQLiteStore) PendingBlobDeletions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT blob_id FROM pending_blob_deletions ORDER BY created_at, blob_id")
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

// DrainBlobDeletions removes the bytes of each queued id and then its queue
// row, in that order, and returns how many rows it retired.
//
// The order is the whole safety argument. Unlink first and the row survives a
// crash, so the next drain repeats a delete that already succeeded — which
// costs nothing, because Blob.Delete is idempotent. Delete the row first and a
// crash strands the file with nothing left to name it, which is the leak this
// queue exists to close.
//
// Every id is attempted even after one fails, so a single unremovable file
// cannot strand the rest, and a failure leaves that id's row in place — the
// retry is the next drain, not a compensating action any caller has to take.
// The first error is still returned rather than swallowed: a disk that refused
// a delete is worth surfacing even though the queue will keep trying.
//
// Callers pass the ids the delete operation just enqueued, so a request drains
// its own work and never walks a backlog.
func (s *SQLiteStore) DrainBlobDeletions(ctx context.Context, blobs BlobDeleter, ids []string) (int, error) {
	var (
		drained   int
		forgotten []string
		firstErr  error
	)
	for _, id := range ids {
		if err := blobs.Delete(ctx, id); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("blob %s: %w", id, err)
			}
			continue // the queue row stays; the next drain retries it
		}
		if _, err := s.db.ExecContext(ctx,
			"DELETE FROM pending_blob_deletions WHERE blob_id = ?", id); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("dequeue blob %s: %w", id, err)
			}
			continue
		}
		drained++
		forgotten = append(forgotten, id)
	}

	// The recorded lengths of the bytes that are now actually gone (av-fw1b).
	// This is the right place for it and the only one: a blob id reaches this
	// queue precisely when the last row referencing it went away, so by the
	// time it drains there is nobody left to charge for it — and every path
	// that condemns bytes goes through here, so none of them has to remember.
	//
	// A failure is logged, not returned, and never displaces firstErr. The two
	// halves are not equally bad: a byte left on disk is disk somebody is still
	// paying for, while a size row nothing references is charged to nobody and
	// is untidiness only. ForgetBlobSizes keeps any id another row still names,
	// so passing drained ids wholesale is safe even for a shared blob.
	if len(forgotten) > 0 {
		if err := s.ForgetBlobSizes(ctx, forgotten); err != nil {
			slog.WarnContext(ctx, "forget blob sizes", slog.String("err", err.Error()))
		}
	}
	return drained, firstErr
}

// DrainAllBlobDeletions drains the entire queue. This is the startup pass, and
// the point at which a crash's leftovers are reclaimed: whatever a dead
// process enqueued and did not finish is still here, and finishing it is the
// same idempotent work the original request would have done.
//
// It is the only drain that reads the queue rather than being handed ids,
// because it is the only one that has to find work nobody told it about.
func (s *SQLiteStore) DrainAllBlobDeletions(ctx context.Context, blobs BlobDeleter) (int, error) {
	ids, err := s.PendingBlobDeletions(ctx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	slog.InfoContext(ctx, "draining queued blob deletions", slog.Int("queued", len(ids)))
	return s.DrainBlobDeletions(ctx, blobs, ids)
}
