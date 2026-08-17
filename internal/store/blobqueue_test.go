package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-8gyd. The queue exists for exactly one failure — a process that dies
// between the delete transaction and the unlink — so the tests here are
// written against the *filesystem* and against a store that is closed and
// reopened, not against a convenient in-memory double. A test that only asked
// the queue what it contains would pass against an implementation that never
// removes a file.

// queueFixture is a store on a real path plus a real filesystem blob store, so
// a test can close the first (the crash), reopen it (the restart), and still
// look at the second.
type queueFixture struct {
	dbPath  string
	blobDir string
	store   *SQLiteStore
	blobs   *blob.FSStore
}

func newQueueFixture(t *testing.T) *queueFixture {
	t.Helper()
	dir := t.TempDir()
	fx := &queueFixture{dbPath: filepath.Join(dir, "app.db"), blobDir: filepath.Join(dir, "blobs")}

	var err error
	fx.blobs, err = blob.NewFSStore(fx.blobDir)
	require.NoError(t, err)
	fx.store = fx.open(t)
	t.Cleanup(func() { _ = fx.store.Close() })
	return fx
}

func (fx *queueFixture) open(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLite(fx.dbPath)
	require.NoError(t, err)
	return s
}

// restart stands in for the process dying and coming back: the old handle is
// closed without draining anything, and a fresh one is opened over the same
// file. Whatever survives is what a real restart would find.
func (fx *queueFixture) restart(t *testing.T) {
	t.Helper()
	require.NoError(t, fx.store.Close())
	fx.store = fx.open(t)
}

// putBody writes bytes into the blob store and returns the id, so an artifact
// row and the file it names are seeded together.
func (fx *queueFixture) putBody(t *testing.T, id, body string) string {
	t.Helper()
	require.NoError(t, fx.blobs.Put(context.Background(), id, strings.NewReader(body)))
	return id
}

func (fx *queueFixture) path(id string) string { return filepath.Join(fx.blobDir, id) }

func TestDeleteArtifactQueuesItsBlobsAndDrainRemovesThem(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()

	fx.putBody(t, "body-1", "<html>tool</html>")
	fx.putBody(t, "widget-1", "<b>42 km</b>")
	require.NoError(t, fx.store.PutArtifact(ctx, &Artifact{
		ID: "a1", OwnerID: 1, Title: "Tracker", SourceBlobID: "body-1", WidgetBlobID: "widget-1", Tier: Tier1,
	}))

	queued, err := fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"body-1", "widget-1"}, queued,
		"both of an artifact's blobs lose their last reference at once")

	// Before the drain the intent is durable and the bytes are still there —
	// which is the whole window the queue covers.
	pending, err := fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"body-1", "widget-1"}, pending)
	require.FileExists(t, fx.path("body-1"))

	n, err := fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	assert.NoFileExists(t, fx.path("body-1"))
	assert.NoFileExists(t, fx.path("widget-1"))
	pending, err = fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending, "a drained id must not stay queued")
}

// The reason this ticket exists. Kill the process after the delete commits and
// before the unlink: the files are still on disk with no row naming them, and
// only the queue can still find them.
func TestCrashBetweenDeleteAndUnlinkIsFinishedByTheNextStartup(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()

	fx.putBody(t, "body-crash", "<html>tool</html>")
	fx.putBody(t, "widget-crash", "<b>tile</b>")
	require.NoError(t, fx.store.PutArtifact(ctx, &Artifact{
		ID: "a1", OwnerID: 1, Title: "Doomed", SourceBlobID: "body-crash", WidgetBlobID: "widget-crash", Tier: Tier1,
	}))

	// The transaction commits…
	_, err := fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	// …and the process dies here, before anything is unlinked.
	fx.restart(t)

	require.FileExists(t, fx.path("body-crash"), "the crash is only a crash if the bytes survived it")
	a, err := fx.store.GetArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	require.Nil(t, a, "the row is gone, so nothing but the queue can name those bytes")

	n, err := fx.store.DrainAllBlobDeletions(ctx, fx.blobs)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	assert.NoFileExists(t, fx.path("body-crash"))
	assert.NoFileExists(t, fx.path("widget-crash"))
	pending, err := fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

// The drain is retried by every startup, so being repeatable is not a nicety.
// The second run finds nothing queued; a run over an id whose file somebody
// already removed still succeeds and still retires the row, because
// Blob.Delete is idempotent for a missing id (av-7jcq).
func TestDrainingTwiceIsHarmless(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()

	fx.putBody(t, "body-2", "<html>tool</html>")
	require.NoError(t, fx.store.PutArtifact(ctx, &Artifact{
		ID: "a1", OwnerID: 1, Title: "Twice", SourceBlobID: "body-2", Tier: Tier1,
	}))
	queued, err := fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)

	// A drain that unlinked the file and then died before deleting its queue
	// row: the row is still there and the file is not.
	require.NoError(t, fx.blobs.Delete(ctx, "body-2"))
	pending, err := fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"body-2"}, pending)

	n, err := fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err, "a queued id whose file is already gone must drain successfully")
	assert.Equal(t, 1, n)

	n, err = fx.store.DrainAllBlobDeletions(ctx, fx.blobs)
	require.NoError(t, err)
	assert.Zero(t, n, "the second drain has nothing left to do")
}

// The property that makes automatic draining safe: a blob id only reaches the
// queue when the row that named it was the last one. Per-owner content
// addressing (av-20fk) lets two artifacts in one library share a body, and an
// unconditional enqueue would strip it out of the one that survived.
func TestSharedBlobIsQueuedOnlyWhenItsLastReferenceGoes(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()

	fx.putBody(t, "shared-body", "<html>the same tool twice</html>")
	for _, id := range []string{"a1", "a2"} {
		require.NoError(t, fx.store.PutArtifact(ctx, &Artifact{
			ID: id, OwnerID: 1, Title: id, SourceBlobID: "shared-body", Tier: Tier1,
		}))
	}

	queued, err := fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Empty(t, queued, "a2 still names those bytes")

	pending, err := fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending, "the queue must never hold an id a live row still names")

	// And the survivor is intact: row, blob id, and bytes.
	n, err := fx.store.DrainAllBlobDeletions(ctx, fx.blobs)
	require.NoError(t, err)
	assert.Zero(t, n)
	surviving, err := fx.store.GetArtifact(ctx, 1, "a2")
	require.NoError(t, err)
	require.NotNil(t, surviving)
	assert.Equal(t, "shared-body", surviving.SourceBlobID)
	require.FileExists(t, fx.path("shared-body"))

	// The second delete takes the last reference with it.
	queued, err = fx.store.DeleteArtifact(ctx, 1, "a2")
	require.NoError(t, err)
	assert.Equal(t, []string{"shared-body"}, queued)

	n, err = fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.NoFileExists(t, fx.path("shared-body"))
}

// The refcount spans columns, not just rows: an id used as one artifact's body
// and another's widget is referenced twice, and detaching the widget must not
// condemn the body.
func TestDetachingAWidgetSharedWithABodyQueuesNothing(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()

	fx.putBody(t, "dual-use", "<html>both</html>")
	require.NoError(t, fx.store.PutArtifact(ctx, &Artifact{
		ID: "a1", OwnerID: 1, Title: "body", SourceBlobID: "dual-use", Tier: Tier1,
	}))
	require.NoError(t, fx.store.PutArtifact(ctx, &Artifact{
		ID: "a2", OwnerID: 1, Title: "tile", SourceBlobID: "own-body", WidgetBlobID: "dual-use", Tier: Tier1,
	}))

	queued, err := fx.store.DeleteWidget(ctx, 1, "a2")
	require.NoError(t, err)
	assert.Empty(t, queued, "a1's body is still that blob")
	require.FileExists(t, fx.path("dual-use"))

	// Detaching is still idempotent, and an artifact that has no widget
	// queues nothing rather than failing.
	queued, err = fx.store.DeleteWidget(ctx, 1, "a2")
	require.NoError(t, err)
	assert.Empty(t, queued)

	// Once the other reference goes, the id is condemned exactly once.
	queued, err = fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, []string{"dual-use"}, queued)
}

// Erasing an account is the third delete path, and the one that condemns a
// whole library at once. A crash between its commit and its unlinks is
// finished by the same startup drain.
func TestDeleteAccountQueuesEveryBlobItOrphans(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()

	admin, err := fx.store.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	require.True(t, admin.IsAdmin)
	member, err := fx.store.UpsertUser(ctx, "sub-member", "member@example.test")
	require.NoError(t, err)

	fx.putBody(t, "member-body", "<html>theirs</html>")
	fx.putBody(t, "member-widget", "<b>tile</b>")
	require.NoError(t, fx.store.PutArtifact(ctx, &Artifact{
		ID: "m1", OwnerID: member.ID, Title: "Theirs",
		SourceBlobID: "member-body", WidgetBlobID: "member-widget", Tier: Tier1,
	}))

	queued, err := fx.store.DeleteAccount(ctx, member.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"member-body", "member-widget"}, queued)

	// The process dies before the handler drains; the restart finishes it.
	fx.restart(t)
	n, err := fx.store.DrainAllBlobDeletions(ctx, fx.blobs)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.NoFileExists(t, fx.path("member-body"))
	assert.NoFileExists(t, fx.path("member-widget"))
}

// A blob store that refuses leaves the queue row in place, because the retry
// is the next drain rather than something the caller has to arrange. The error
// still surfaces: a disk that would not delete is worth reporting even though
// the work is not lost.
func TestAFailedUnlinkKeepsItsQueueRow(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()

	fx.putBody(t, "body-3", "<html>tool</html>")
	require.NoError(t, fx.store.PutArtifact(ctx, &Artifact{
		ID: "a1", OwnerID: 1, Title: "Stubborn", SourceBlobID: "body-3", Tier: Tier1,
	}))
	queued, err := fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)

	n, err := fx.store.DrainBlobDeletions(ctx, refusingBlobs{}, queued)
	require.Error(t, err)
	assert.Zero(t, n)

	pending, err := fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"body-3"}, pending, "a failed drain must leave the intent recorded")

	// The next drain, against a working blob store, finishes it.
	n, err = fx.store.DrainAllBlobDeletions(ctx, fx.blobs)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.NoFileExists(t, fx.path("body-3"))
}

// refusingBlobs stands in for a read-only volume: every delete fails, and none
// of them is the "already missing" case FSStore absorbs.
type refusingBlobs struct{}

func (refusingBlobs) Delete(context.Context, string) error { return os.ErrPermission }
