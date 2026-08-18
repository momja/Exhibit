package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-fw1b. What these pin is not "the arithmetic works" but the three
// decisions the arithmetic encodes: a total is derived from the rows rather
// than counted up incrementally, a shared blob is charged in full to every
// owner that references it and once to each of them, and a recorded length can
// always be rebuilt from the bytes.

// fakeBlobs is a blobGetter over a map, so a recompute can be run against
// known lengths and against a blob that will not read.
type fakeBlobs struct {
	bodies map[string]string
	fail   map[string]bool
	// onGet runs at the moment a blob is read, which is where a racing write
	// lands in the real thing.
	onGet func(id string)
}

func (f *fakeBlobs) Get(_ context.Context, id string) (io.ReadCloser, error) {
	if f.onGet != nil {
		f.onGet(id)
	}
	if f.fail[id] {
		return nil, errors.New("backend unavailable")
	}
	body, ok := f.bodies[id]
	if !ok {
		return nil, errors.New("no such blob")
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

// putSized writes an artifact and records the lengths its blobs would have
// had, which is what the API's write funnel does for real.
func putSized(t *testing.T, s *SQLiteStore, id string, owner int64, bodyID string, bodyBytes int64, widgetID string, widgetBytes int64) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: id, OwnerID: owner, Title: id, SourceBlobID: bodyID, WidgetBlobID: widgetID, Tier: Tier1,
	}))
	require.NoError(t, s.RecordBlobSize(ctx, bodyID, bodyBytes))
	if widgetID != "" {
		require.NoError(t, s.RecordBlobSize(ctx, widgetID, widgetBytes))
	}
}

func usage(t *testing.T, s *SQLiteStore, owner int64) int64 {
	t.Helper()
	n, err := s.StorageUsage(context.Background(), owner)
	require.NoError(t, err)
	return n
}

// The body and the widget both count. The widget is the easy one to forget —
// it is a column rather than a row — and a snapshot's vendored payload will
// make the same point far more loudly once assets land.
func TestStorageUsageCountsBodiesAndWidgets(t *testing.T) {
	s := newTestStore(t)
	assert.Zero(t, usage(t, s, 1), "an owner holding nothing holds zero, not an error")

	putSized(t, s, "a1", 1, "a1-body", 1000, "a1-widget", 200)
	putSized(t, s, "a2", 1, "a2-body", 30, "", 0)

	assert.Equal(t, int64(1230), usage(t, s, 1))
}

// Owner scoping, on the number as much as on the rows: an owner's total is
// theirs alone, and an id belonging to nobody on this instance reads as zero
// exactly like an owner with an empty library.
func TestStorageUsageIsPerOwner(t *testing.T) {
	s := newTestStore(t)
	putSized(t, s, "mine", 1, "mine-body", 100, "", 0)
	putSized(t, s, "theirs", 2, "theirs-body", 999, "", 0)

	assert.Equal(t, int64(100), usage(t, s, 1))
	assert.Equal(t, int64(999), usage(t, s, 2))
	assert.Zero(t, usage(t, s, 77))
}

// The shared-blob decision, and the one worth breaking a test over: a blob two
// owners reference is charged to each of them at full size, and a blob one
// owner references twice is charged to them once.
//
// Both halves are the same rule seen from two sides — the charge is
// deduplicated within an owner and never across owners — and both are
// properties of the query rather than of the caller, so there is no way to ask
// for the other answer. It is the ungameable choice: an owner cannot shrink
// their total by uploading what another tenant already has, and their total
// never moves because a stranger deleted something.
func TestSharedBlobIsChargedInFullToEachOwnerAndOnceWithinOne(t *testing.T) {
	s := newTestStore(t)
	const shared = "shared-asset"

	putSized(t, s, "mine-1", 1, shared, 5000, "", 0)
	putSized(t, s, "mine-2", 1, shared, 5000, "", 0) // same owner, same blob
	putSized(t, s, "theirs", 2, shared, 5000, "", 0) // different owner, same blob

	assert.Equal(t, int64(5000), usage(t, s, 1), "one owner referencing a blob twice pays for it once")
	assert.Equal(t, int64(5000), usage(t, s, 2), "the second owner pays full size, not half")

	// And the stability half of the claim: owner 2 deleting their copy leaves
	// owner 1's total exactly where it was.
	require.NoError(t, s.DeleteArtifact(context.Background(), 2, "theirs"))
	assert.Equal(t, int64(5000), usage(t, s, 1))
	assert.Zero(t, usage(t, s, 2))
}

// Deleting an artifact stops its bytes being charged — with no decrement
// anywhere, because the total is the rows.
func TestDeletingAnArtifactDropsItsBytes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putSized(t, s, "a1", 1, "a1-body", 1000, "a1-widget", 200)
	putSized(t, s, "a2", 1, "a2-body", 40, "", 0)

	require.NoError(t, s.DeleteArtifact(ctx, 1, "a1"))
	assert.Equal(t, int64(40), usage(t, s, 1), "both of the artifact's blobs stop counting")
}

// Detaching a widget is the same story one column down: SetWidgetBlobID("")
// removes the reference, so the tile's bytes stop being charged even though
// the artifact stays.
func TestDetachingAWidgetDropsItsBytes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putSized(t, s, "a1", 1, "a1-body", 1000, "a1-widget", 200)

	require.NoError(t, s.SetWidgetBlobID(ctx, 1, "a1", ""))
	assert.Equal(t, int64(1000), usage(t, s, 1))
}

// ForgetBlobSizes is a prune, not a delete by id: a length still referenced by
// anyone survives it. That is what lets a caller pass every id it just removed
// the bytes for without first working out which of them were shared.
func TestForgetBlobSizesKeepsStillReferencedLengths(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const shared = "shared-asset"
	putSized(t, s, "mine", 1, shared, 5000, "", 0)
	putSized(t, s, "theirs", 2, shared, 5000, "", 0)
	putSized(t, s, "solo", 1, "solo-body", 70, "", 0)

	// Owner 1 deletes both of theirs and asks for every id to be forgotten.
	require.NoError(t, s.DeleteArtifact(ctx, 1, "mine"))
	require.NoError(t, s.DeleteArtifact(ctx, 1, "solo"))
	require.NoError(t, s.ForgetBlobSizes(ctx, []string{shared, "solo-body"}))

	assert.Equal(t, int64(5000), usage(t, s, 2), "the shared length survives, because owner 2 still references it")

	var solo int
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM blob_sizes WHERE blob_id = 'solo-body'`).Scan(&solo))
	assert.Zero(t, solo, "the unreferenced length is gone")

	assert.NoError(t, s.ForgetBlobSizes(ctx, []string{"never-recorded"}), "idempotent, like Blob.Delete")
	assert.NoError(t, s.ForgetBlobSizes(ctx, nil))
}

// Recompute is the correction path: it rebuilds the recorded lengths from the
// bytes, so a wrong number — a crash between the blob write and the row, a
// bug, a hand repair — is fixable rather than authoritative by assumption.
// Running it twice writes the same thing, and it never touches another owner.
func TestRecomputeRebuildsWrongSizesAndIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	blobs := &fakeBlobs{bodies: map[string]string{
		"a1-body":   strings.Repeat("x", 1234),
		"a1-widget": strings.Repeat("w", 56),
		"a2-body":   strings.Repeat("y", 9),
		"other":     strings.Repeat("z", 4),
	}}

	putSized(t, s, "a1", 1, "a1-body", 999999, "a1-widget", 0) // both wrong
	putSized(t, s, "a2", 1, "a2-body", 9, "", 0)               // already right
	putSized(t, s, "theirs", 2, "other", 400, "", 0)           // wrong, and not ours

	res, err := s.RecomputeStorageUsage(ctx, 1, blobs)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Blobs)
	assert.Zero(t, res.Unreadable)
	assert.Equal(t, int64(1234+56+9), res.Bytes)
	assert.Equal(t, int64(1234+56+9), usage(t, s, 1))
	assert.Equal(t, int64(400), usage(t, s, 2), "another owner's recorded sizes are untouched")

	again, err := s.RecomputeStorageUsage(ctx, 1, blobs)
	require.NoError(t, err)
	assert.Equal(t, res, again, "idempotent")
}

// A blob that will not read keeps the size it already had. Zeroing it instead
// would let one transient backend error silently shrink somebody's total,
// which is the one failure a repair tool must not have — and bytes genuinely
// missing from the blob store are an orphan problem, not a size problem.
func TestRecomputeKeepsTheRecordedSizeOfAnUnreadableBlob(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	blobs := &fakeBlobs{
		bodies: map[string]string{"good": strings.Repeat("x", 10)},
		fail:   map[string]bool{"bad": true},
	}
	putSized(t, s, "a1", 1, "good", 999, "", 0)
	putSized(t, s, "a2", 1, "bad", 500, "", 0)

	res, err := s.RecomputeStorageUsage(ctx, 1, blobs)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Blobs)
	assert.Equal(t, 1, res.Unreadable)
	assert.Equal(t, int64(10+500), res.Bytes, "the unreadable blob keeps its recorded 500")
}

// A referenced blob whose length was never recorded contributes nothing rather
// than failing the read or inventing a size — the deliberate fail-quiet
// direction for a number nothing refuses on — and a recompute is what makes it
// count.
func TestUnrecordedBlobUnderReportsUntilRecomputed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: "a1", OwnerID: 1, SourceBlobID: "a1-body", Tier: Tier1}))

	assert.Zero(t, usage(t, s, 1))

	blobs := &fakeBlobs{bodies: map[string]string{"a1-body": strings.Repeat("x", 77)}}
	_, err := s.RecomputeStorageUsage(ctx, 1, blobs)
	require.NoError(t, err)
	assert.Equal(t, int64(77), usage(t, s, 1))
}

// RecordBlobSize replaces rather than accumulates: bodies are rewritten in
// place under the same blob id by an edit, a refetch and a widget save alike,
// so an owner who edits an artifact ten times is holding one body.
func TestRecordBlobSizeReplacesTheLength(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putSized(t, s, "a1", 1, "a1-body", 100, "", 0)

	require.NoError(t, s.RecordBlobSize(ctx, "a1-body", 250))
	assert.Equal(t, int64(250), usage(t, s, 1))

	assert.Error(t, s.RecordBlobSize(ctx, "", 10), "a length with no blob to belong to is a bug, not a row")
}

// The instance-wide listing, which is the self-hosted "what is using my disk"
// question, and the owner list a recompute walks — which must come from the
// references rather than from `users`, since owner 1 on a single-user instance
// has no users row and still has a library.
func TestListStorageUsageAndOwners(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putSized(t, s, "a1", 1, "a1-body", 100, "a1-widget", 10)
	putSized(t, s, "a2", 2, "a2-body", 5000, "", 0)

	list, err := s.ListStorageUsage(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, OwnerStorage{OwnerID: 2, Blobs: 1, Bytes: 5000}, list[0], "heaviest first")
	assert.Equal(t, OwnerStorage{OwnerID: 1, Blobs: 2, Bytes: 110}, list[1])

	owners, err := s.ListStorageOwners(ctx)
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2}, owners)
}

// DELETE /api/account takes the total to zero — by construction, since the
// rows the total is derived from are what deletion removes. The shared half
// matters as much: another owner's total must not move because this account
// left.
func TestDeletingAnAccountTakesItsStorageToZero(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	admin, err := s.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	member, err := s.UpsertUser(ctx, "sub-member", "member@example.test")
	require.NoError(t, err)

	const shared = "shared-asset"
	putSized(t, s, "theirs", member.ID, "theirs-body", 800, "theirs-widget", 40)
	putSized(t, s, "shared-of-theirs", member.ID, shared, 5000, "", 0)
	putSized(t, s, "shared-of-admins", admin.ID, shared, 5000, "", 0)

	require.Equal(t, int64(5840), usage(t, s, member.ID))

	_, err = s.DeleteAccount(ctx, member.ID)
	require.NoError(t, err)

	assert.Zero(t, usage(t, s, member.ID))
	assert.Equal(t, int64(5000), usage(t, s, admin.ID),
		"the surviving owner still references the shared blob, so its length survives too")
}

// A library that predates migration 021 has bytes on disk and no recorded
// lengths, so it would report zero for everything until each artifact happened
// to be edited. The startup backfill is what stops an upgrade from silently
// answering "0 B" for a full shelf.
func TestBackfillMeasuresBlobsWithNoRecordedLength(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	blobs := &fakeBlobs{bodies: map[string]string{
		"old-body":   strings.Repeat("x", 300),
		"old-widget": strings.Repeat("w", 20),
		"new-body":   strings.Repeat("y", 5),
	}}

	// Two artifacts as they would look after an upgrade: rows, no lengths.
	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: "old", OwnerID: 1, SourceBlobID: "old-body", WidgetBlobID: "old-widget", Tier: Tier1}))
	// And one written since, whose length the write funnel already recorded.
	putSized(t, s, "new", 1, "new-body", 5, "", 0)
	require.Equal(t, int64(5), usage(t, s, 1))

	require.NoError(t, s.BackfillBlobSizes(ctx, blobs))
	assert.Equal(t, int64(300+20+5), usage(t, s, 1))

	// Idempotent, and free: a second pass finds nothing to measure. Proven by
	// removing the bodies — a backfill that tried to re-read them would fail
	// or zero them, and this one does neither.
	blobs.bodies = map[string]string{}
	require.NoError(t, s.BackfillBlobSizes(ctx, blobs))
	assert.Equal(t, int64(300+20+5), usage(t, s, 1))
}

// A recompute must never overwrite a length that an ordinary write recorded
// while the pass was reading the blob. The writer's number describes the body
// that is actually there; the measurement describes one that has been replaced
// — and on the filesystem backend, where Put truncates the very file being
// read, possibly neither.
func TestRecomputeDoesNotClobberAConcurrentWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putSized(t, s, "a1", 1, "a1-body", 100, "", 0)

	// A reader that performs the racing write at the moment the pass reads it
	// — the interleaving the real race produces, made deterministic.
	blobs := &fakeBlobs{bodies: map[string]string{"a1-body": strings.Repeat("x", 100)}}
	blobs.onGet = func(id string) {
		require.NoError(t, s.RecordBlobSize(ctx, id, 4242))
		// Also change what the blob returns, so the pass measures the old body.
		blobs.onGet = nil
	}

	res, err := s.RecomputeStorageUsage(ctx, 1, blobs)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Blobs, "nothing was rewritten by the pass")
	assert.Equal(t, 1, res.Superseded)
	assert.Equal(t, int64(4242), usage(t, s, 1), "the writer's length stands")
}

// The instance figure is over *distinct* blobs, so it describes a disk. The
// per-owner totals deliberately add up to more than it once a blob is shared —
// each owner is charged in full — and a line about disk usage must not be that
// sum.
func TestStoredBytesCountsSharedBlobsOnce(t *testing.T) {
	s := newTestStore(t)
	const shared = "shared-asset"
	putSized(t, s, "mine", 1, shared, 5000, "", 0)
	putSized(t, s, "theirs", 2, shared, 5000, "", 0)
	putSized(t, s, "solo", 1, "solo-body", 70, "", 0)

	assert.Equal(t, int64(5070), usage(t, s, 1))
	assert.Equal(t, int64(5000), usage(t, s, 2))

	blobs, bytes, err := s.StoredBytes(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(2), blobs)
	assert.Equal(t, int64(5070), bytes, "the shared blob is on the disk once, not twice")
}

// DeleteAccount hands the size prune every blob id the account named, and
// SQLite caps a statement at 32766 bound variables — so without chunking a
// large enough library could not be deleted at all: the prune would fail and
// roll the whole deletion back.
func TestDeletingALargeAccountStaysUnderTheVariableLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	admin, err := s.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	member, err := s.UpsertUser(ctx, "sub-member", "member@example.test")
	require.NoError(t, err)
	require.True(t, admin.IsAdmin)

	// 20k blob ids: over the limit as one IN-list, and cheap as rows.
	const artifacts = 10000
	for i := range artifacts {
		id := fmt.Sprintf("a%d", i)
		putSized(t, s, id, member.ID, id+"-body", 10, id+"-widget", 1)
	}
	require.Equal(t, int64(artifacts*11), usage(t, s, member.ID))

	blobIDs, err := s.DeleteAccount(ctx, member.ID)
	require.NoError(t, err)
	assert.Len(t, blobIDs, artifacts*2)
	assert.Zero(t, usage(t, s, member.ID))

	var left int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM blob_sizes`).Scan(&left))
	assert.Zero(t, left, "every length went with the account")
}
