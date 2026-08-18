// Per-owner storage accounting (av-fw1b).
//
// The question this file answers is "how many bytes is this owner holding",
// and the shape of the answer is deliberate: a length is recorded once, where
// the bytes are written, and the *total* is derived on read by joining those
// lengths to the rows that reference them (migration 021's blob_references
// view).
//
// Nothing here is an incremental counter, and that is the point. A counter has
// to be decremented by whoever deletes a thing, so it drifts the first time a
// caller forgets, a write crashes between the blob and the row, or a repair is
// done by hand. A derived total cannot drift out of step with the rows,
// because it *is* the rows — deleting an artifact stops its bytes being
// charged in the same statement that deletes it, with nothing to remember.
//
// What can still be wrong is a recorded length: a blob written whose size row
// never committed, or a body rewritten by something that did not come through
// the write funnel. RecomputeStorageUsage is the correction, and it is the
// only thing here that touches the blob store.
//
// This ticket produces the number and enforces nothing — no request is refused
// as a result of anything in this file. Limits are av-10bw's.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// StorageRecompute reports what a recompute pass did. Unreadable is the count
// of referenced blobs whose bytes could not be read; see RecomputeStorageUsage
// for why those keep their recorded size rather than being zeroed.
type StorageRecompute struct {
	Blobs      int   `json:"blobs"`
	Unreadable int   `json:"unreadable"`
	Bytes      int64 `json:"bytes"`
}

// OwnerStorage is one owner's total, for the whole-instance listing.
type OwnerStorage struct {
	OwnerID int64 `json:"owner_id"`
	Blobs   int64 `json:"blobs"`
	Bytes   int64 `json:"bytes"`
}

// ownerBlobs is the set of blobs charged to one owner: every blob the schema
// says the owner references, counted **once** however many of their artifacts
// reference it.
//
// The DISTINCT is where the shared-asset decision lives (av-20fk's refcounted
// assets can be referenced by several artifacts and several owners). Within an
// owner, a blob is charged once — they are storing one copy of it. Across
// owners, every referencing owner is charged the full size, because that is
// what each of them would have to store alone; it cannot be gamed by
// deduplicating against another tenant's uploads, and an owner's total never
// moves because a stranger deleted something. The rule is a property of this
// query rather than of whoever calls it: there is no way to ask for the other
// answer. architecture.md §3.3 records the rationale.
const ownerBlobs = `SELECT DISTINCT blob_id FROM blob_references WHERE owner_id = ?`

// RecordBlobSize persists a blob's byte length. Called by the write funnel
// that wraps every Blob.Put, and idempotent by upsert: rewriting a body in
// place (an edit, a refetch, a widget save all reuse their blob id) replaces
// the recorded length rather than adding a second row.
func (s *SQLiteStore) RecordBlobSize(ctx context.Context, blobID string, bytes int64) error {
	if blobID == "" {
		return fmt.Errorf("record blob size: empty blob id")
	}
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO blob_sizes (blob_id, bytes, updated_at) VALUES (?, ?, datetime('now'))
        ON CONFLICT(blob_id) DO UPDATE SET bytes = excluded.bytes, updated_at = excluded.updated_at`,
		blobID, bytes)
	return err
}

// ForgetBlobSizes drops the size rows for blob ids **nothing references any
// more**, and is how the table stays the size of the library rather than of
// its history.
//
// The reference check is not caution, it is correctness: with refcounted
// shared assets (av-20fk) one blob id can be named by rows belonging to
// several owners, so deleting an artifact must not delete a length another
// owner's total is still computed from. An id that is still referenced is
// therefore left alone, and callers can pass every id they just deleted the
// bytes for without knowing which of them were shared.
//
// Idempotent, like Blob.Delete: an id with no row is success. The rows are
// inert while they last — an unreferenced size is charged to nobody — so a
// failure here is untidiness rather than a wrong number.
func (s *SQLiteStore) ForgetBlobSizes(ctx context.Context, blobIDs []string) error {
	ids := nonEmpty(blobIDs)
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM blob_sizes WHERE blob_id IN (`+placeholders(len(ids))+`)
           AND blob_id NOT IN (SELECT blob_id FROM blob_references)`,
		anySlice(ids)...)
	return err
}

// StorageUsage returns the owner's total stored bytes: one query, no contact
// with the blob store.
//
// A referenced blob with no recorded size contributes nothing — the join drops
// it — which is the deliberate fail-quiet direction for a number nothing
// refuses on. It under-reports until a recompute measures the blob, rather
// than failing the read or inventing a size.
func (s *SQLiteStore) StorageUsage(ctx context.Context, ownerID int64) (int64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(s.bytes), 0) FROM (`+ownerBlobs+`) r
           JOIN blob_sizes s ON s.blob_id = r.blob_id`, ownerID).Scan(&total)
	return total, err
}

// ListStorageUsage is the whole instance, heaviest owner first — the
// self-hosted "what is actually using my disk" question, which had no answer
// before this ticket. Owners with no recorded bytes are omitted.
func (s *SQLiteStore) ListStorageUsage(ctx context.Context) ([]OwnerStorage, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT r.owner_id, COUNT(*), COALESCE(SUM(s.bytes), 0)
          FROM (SELECT DISTINCT owner_id, blob_id FROM blob_references) r
          JOIN blob_sizes s ON s.blob_id = r.blob_id
         GROUP BY r.owner_id
         ORDER BY 3 DESC, 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OwnerStorage
	for rows.Next() {
		var o OwnerStorage
		if err := rows.Scan(&o.OwnerID, &o.Blobs, &o.Bytes); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListStorageOwners is every owner the schema says references a blob — read
// from the references alone, not from blob_sizes, because an owner whose
// lengths were never recorded is precisely the one a recompute is for. It is
// also not read from `users`: owner 1 on a single-user instance has no row
// there and still has a library.
func (s *SQLiteStore) ListStorageOwners(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT owner_id FROM blob_references ORDER BY owner_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RecomputeStorageUsage re-measures every blob the owner references and
// rewrites the recorded lengths, returning the owner's total afterwards. It is
// the correction path: incremental records drift, so the number has to be
// rebuildable rather than authoritative by assumption.
//
// Idempotent — running it twice over an unchanged library writes the same
// lengths and returns the same total — and safe to run against a live
// instance, since it only ever replaces a length with the length the bytes
// actually have.
//
// It is the one accounting path that touches the blob store, and it reads each
// body to measure it (Blob.Store has no Size, and adding one is av-52ll's
// object-store implementation to weigh in on). That makes it O(bytes) and an
// explicitly-invoked repair rather than something on a request path.
//
// A blob that cannot be read **keeps its recorded size** and is counted in
// Unreadable. The alternative — treating an unreadable blob as zero bytes —
// makes a transient backend error silently shrink somebody's total, which is
// the one failure a repair tool must not have. Bytes genuinely missing from
// the blob store are an orphan/ledger problem, not a size problem.
func (s *SQLiteStore) RecomputeStorageUsage(ctx context.Context, ownerID int64, blobs blobGetter) (StorageRecompute, error) {
	var out StorageRecompute

	rows, err := s.db.QueryContext(ctx, ownerBlobs, ownerID)
	if err != nil {
		return out, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()

	for _, id := range ids {
		n, err := blobLength(ctx, blobs, id)
		if err != nil {
			out.Unreadable++
			slog.WarnContext(ctx, "recompute storage: blob unreadable, keeping recorded size",
				slog.Int64("owner_id", ownerID), slog.String("blob_id", id), slog.String("err", err.Error()))
			continue
		}
		if err := s.RecordBlobSize(ctx, id, n); err != nil {
			return out, fmt.Errorf("record size for blob %s: %w", id, err)
		}
		out.Blobs++
	}

	out.Bytes, err = s.StorageUsage(ctx, ownerID)
	return out, err
}

// blobLength measures a blob without holding it in memory: these are whole
// artifact bodies, and a snapshot's vendored wasm payload is the largest thing
// the system stores.
func blobLength(ctx context.Context, blobs blobGetter, id string) (int64, error) {
	rc, err := blobs.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	return io.Copy(io.Discard, rc)
}

// forgetBlobSizesTx is ForgetBlobSizes inside a caller's transaction, for
// DeleteAccount — which collects the ids it is about to orphan while the rows
// that name them still exist, and must drop their lengths in the same commit.
func forgetBlobSizesTx(ctx context.Context, tx *sql.Tx, blobIDs []string) error {
	ids := nonEmpty(blobIDs)
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`DELETE FROM blob_sizes WHERE blob_id IN (`+placeholders(len(ids))+`)
           AND blob_id NOT IN (SELECT blob_id FROM blob_references)`,
		anySlice(ids)...)
	return err
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func anySlice(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
