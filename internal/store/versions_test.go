package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Version history. What these pin is the design, not the SQL: the head is a
// version, a change snapshots the state of the version it replaces in the same
// transaction, blobs are never overwritten, and a restore pushes a copy rather
// than rewinding.

func putVersioned(t *testing.T, s *SQLiteStore, id string) {
	t.Helper()
	require.NoError(t, s.PutArtifact(context.Background(), &Artifact{
		ID: id, OwnerID: 1, Title: id, SourceBlobID: id + "-body-1", Tier: Tier1,
	}))
}

func stateOf(t *testing.T, s *SQLiteStore, id string) map[string]string {
	t.Helper()
	state, err := s.GetState(context.Background(), 1, id, 1)
	require.NoError(t, err)
	return state
}

func snapshotOf(t *testing.T, s *SQLiteStore, id string, seq int) map[string]string {
	t.Helper()
	var raw string
	require.NoError(t, s.db.QueryRow(
		`SELECT state_json FROM artifact_versions WHERE artifact_id = ? AND seq = ?`, id, seq).Scan(&raw))
	var out map[string]string
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	return out
}

// Every artifact has version 1, whichever way its row got there, so "the head
// is the highest seq" needs no caller to remember to create one.
func TestEveryArtifactStartsWithAnInitialVersion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")

	vs, err := s.ListVersions(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, vs, 1)
	assert.Equal(t, 1, vs[0].Seq)
	assert.Equal(t, VersionInitial, vs[0].Origin)
	assert.Equal(t, "a1-body-1", vs[0].BodyBlobID)
	assert.True(t, vs[0].Current)
	assert.False(t, vs[0].HasState, "the head's state is the live rows, not a snapshot")

	// An insert that bypasses PutArtifact gets one too: the trigger owns it.
	_, err = s.db.Exec(`INSERT INTO artifacts (id, owner_id, title, source_blob_id) VALUES ('raw', 1, 'raw', 'raw-body')`)
	require.NoError(t, err)
	vs, err = s.ListVersions(ctx, 1, "raw")
	require.NoError(t, err)
	assert.Len(t, vs, 1)
}

func TestPutArtifactRecordsProvenanceOnTheFirstVersion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: "agented", OwnerID: 1, Title: "t", SourceBlobID: "b", Tier: Tier1,
		Provenance: Provenance{Origin: VersionAgent, Message: "build a timer", SessionID: "sess-1"},
	}))
	v, err := s.GetVersion(ctx, 1, "agented", 1)
	require.NoError(t, err)
	require.NotNil(t, v)
	assert.Equal(t, VersionAgent, v.Origin)
	assert.Equal(t, "build a timer", v.Message)
	assert.Equal(t, "sess-1", v.SessionID)
}

// The head's pointers and the new row move together, and the old blob is left
// exactly where it was.
func TestCommitVersionMakesTheChangeTheHeadAndKeepsTheOldBlob(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")

	v, err := s.CommitVersion(ctx, 1, "a1", VersionChange{
		Provenance: Provenance{Origin: VersionEdit}, BodyBlobID: strPtr("a1-body-2"), SourceText: strPtr("second"),
	})
	require.NoError(t, err)
	assert.Equal(t, 2, v.Seq)
	assert.True(t, v.Current)

	a, err := s.GetArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, "a1-body-2", a.SourceBlobID)

	vs, err := s.ListVersions(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, vs, 2)
	assert.Equal(t, 2, vs[0].Seq, "newest first")
	assert.True(t, vs[0].Current)
	assert.False(t, vs[1].Current)
	assert.Equal(t, "a1-body-1", vs[1].BodyBlobID, "the replaced body is still named by version 1")
	assert.True(t, vs[1].HasState, "a superseded version carries its snapshot")
}

// A change that names only the widget leaves the body as the head had it, and
// the reverse — read inside the transaction, so two writers cannot undo one
// another.
func TestCommitVersionInheritsWhatItDoesNotName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")

	_, err := s.CommitVersion(ctx, 1, "a1", VersionChange{WidgetBlobID: strPtr("tile-1")})
	require.NoError(t, err)
	_, err = s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2")})
	require.NoError(t, err)

	a, err := s.GetArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, "a1-body-2", a.SourceBlobID)
	assert.Equal(t, "tile-1", a.WidgetBlobID, "the body save did not drop the widget")

	// An empty widget pointer removes it.
	_, err = s.CommitVersion(ctx, 1, "a1", VersionChange{WidgetBlobID: strPtr("")})
	require.NoError(t, err)
	a, err = s.GetArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Empty(t, a.WidgetBlobID)
	assert.Equal(t, "a1-body-2", a.SourceBlobID)
}

// The requirement this whole design is shaped around: the state is captured as
// it stood right before the change, and lands on the version being left.
func TestAChangeSnapshotsTheStateOfTheVersionItReplaces(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "todos", `["one"]`))

	_, err := s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2")})
	require.NoError(t, err)
	// State written after the change belongs to the new version.
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "todos", `["one","two"]`))

	assert.Equal(t, map[string]string{"todos": `["one"]`}, snapshotOf(t, s, "a1", 1))

	_, err = s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-3")})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"todos": `["one","two"]`}, snapshotOf(t, s, "a1", 2))
	assert.Equal(t, map[string]string{"todos": `["one"]`}, snapshotOf(t, s, "a1", 1), "an older snapshot is never rewritten")
}

// An artifact with no state still gets a snapshot — an empty one — so "no
// snapshot" can only mean "this is the head".
func TestAnEmptyStateStillSnapshots(t *testing.T) {
	s := newTestStore(t)
	putVersioned(t, s, "a1")
	_, err := s.CommitVersion(context.Background(), 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2")})
	require.NoError(t, err)
	assert.Empty(t, snapshotOf(t, s, "a1", 1))
}

// Only the owner's rows are the artifact's state. A recipient's own-mode rows
// are theirs and never enter the owner's history.
func TestSnapshotsHoldTheOwnersRowsOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedOwnerAccounts(t, s)
	putVersioned(t, s, "a1")
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "mine", "1"))
	require.NoError(t, s.SetState(ctx, 1, "a1", 2, "theirs", "2"))

	_, err := s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2")})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"mine": "1"}, snapshotOf(t, s, "a1", 1))
}

// Restore returns to an earlier version as a new one: code and state go back,
// nothing is discarded, and the state being left is itself snapshotted so the
// restore can be undone.
func TestRestoreVersionPushesACopyAndReplacesTheState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "score", "10"))

	_, err := s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2"), WidgetBlobID: strPtr("tile-2")})
	require.NoError(t, err)
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "score", "99"))
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "extra", "x"))

	v, err := s.RestoreVersion(ctx, 1, "a1", 1,
		Provenance{Origin: VersionRestore, Message: "Restored v1"}, "first")
	require.NoError(t, err)
	assert.Equal(t, 3, v.Seq, "a restore is a new version, not a rewind")
	assert.Equal(t, VersionRestore, v.Origin)

	a, err := s.GetArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, "a1-body-1", a.SourceBlobID, "the body is version 1's again")
	assert.Empty(t, a.WidgetBlobID, "and so is the widget — version 1 had none")
	assert.Equal(t, map[string]string{"score": "10"}, stateOf(t, s, "a1"),
		"the live state is what version 1 left behind, with the later key gone")

	// The state the restore replaced was captured first.
	assert.Equal(t, map[string]string{"score": "99", "extra": "x"}, snapshotOf(t, s, "a1", 2))

	vs, err := s.ListVersions(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Len(t, vs, 3, "nothing was discarded")
}

// Looking at a version is not restoring it, and shows the same thing: the code
// as it was and the data that version left behind. That is the snapshot a
// restore puts back, which is what makes "this is what I will get" true of what
// a person was shown.
func TestAVersionIsViewedWithTheDataItLeftBehind(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "score", "10"))

	_, err := s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2")})
	require.NoError(t, err)
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "score", "99"))
	require.NoError(t, s.SetState(ctx, 1, "a1", 1, "extra", "x"))

	view, err := s.GetVersionView(ctx, 1, "a1", 1)
	require.NoError(t, err)
	assert.Equal(t, "a1-body-1", view.BodyBlobID)
	assert.Equal(t, map[string]string{"score": "10"}, view.State,
		"the data version 1 left, not the data the artifact holds now")

	// Viewing wrote nothing: the live state and the history are as they were.
	assert.Equal(t, map[string]string{"score": "99", "extra": "x"}, stateOf(t, s, "a1"))
	vs, err := s.ListVersions(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Len(t, vs, 2)

	// And it is exactly what restoring that version then puts back.
	_, err = s.RestoreVersion(ctx, 1, "a1", 1, Provenance{Origin: VersionRestore}, "first")
	require.NoError(t, err)
	assert.Equal(t, view.State, stateOf(t, s, "a1"))
}

// The head has no snapshot — its data is the live rows, which the artifact's own
// page already shows — so there is no earlier state to look at. The same answer
// a restore gives, and for the same reason.
func TestTheCurrentVersionIsNotViewedAsAnEarlierOne(t *testing.T) {
	s := newTestStore(t)
	putVersioned(t, s, "a1")
	_, err := s.GetVersionView(context.Background(), 1, "a1", 1)
	assert.ErrorIs(t, err, ErrAlreadyCurrent)
}

// Another owner's artifact and a version that was never made answer alike.
func TestViewingAVersionThatIsNotThereOrNotTheOwnersIsNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	_, err := s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2")})
	require.NoError(t, err)

	_, err = s.GetVersionView(ctx, 1, "a1", 42)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.GetVersionView(ctx, 2, "a1", 1)
	assert.ErrorIs(t, err, ErrNotFound, "another owner has no view of it")
	_, err = s.GetVersionView(ctx, 1, "no-such-artifact", 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRestoringTheHeadIsRefused(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	_, err := s.RestoreVersion(ctx, 1, "a1", 1, Provenance{}, "")
	assert.ErrorIs(t, err, ErrAlreadyCurrent)

	_, err = s.RestoreVersion(ctx, 1, "a1", 42, Provenance{}, "")
	assert.ErrorIs(t, err, ErrNotFound)
}

// Deleting an artifact condemns every blob its history named, not just the
// head's — and a blob a surviving artifact still names stays.
func TestDeletingAnArtifactQueuesEveryVersionsBlobs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	_, err := s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2"), WidgetBlobID: strPtr("tile-1")})
	require.NoError(t, err)
	_, err = s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-3"), WidgetBlobID: strPtr("tile-2")})
	require.NoError(t, err)

	queued, err := s.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"a1-body-1", "a1-body-2", "a1-body-3", "tile-1", "tile-2"}, queued)

	var rows int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM artifact_versions WHERE artifact_id = 'a1'`).Scan(&rows))
	assert.Zero(t, rows, "the history goes with the artifact")
}

func TestAWidgetSharedByTwoVersionsIsQueuedOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "a1")
	_, err := s.CommitVersion(ctx, 1, "a1", VersionChange{WidgetBlobID: strPtr("tile")})
	require.NoError(t, err)
	_, err = s.CommitVersion(ctx, 1, "a1", VersionChange{BodyBlobID: strPtr("a1-body-2")}) // widget unchanged: both rows name it
	require.NoError(t, err)

	queued, err := s.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a1-body-1", "a1-body-2", "tile"}, queued)
}

// Erasing an account reaches every version's blobs: an older body is named by
// no artifact column, so nothing but the version rows could find it.
func TestDeleteAccountQueuesOlderVersionBlobs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedOwnerAccounts(t, s)
	putOwnedArtifact(t, s, bob, "bobs")
	_, err := s.CommitVersion(ctx, bob, "bobs", VersionChange{BodyBlobID: strPtr("bobs-body-2")})
	require.NoError(t, err)

	queued, err := s.DeleteAccount(ctx, bob)
	require.NoError(t, err)
	assert.Contains(t, queued, "blob-bobs", "the replaced body")
	assert.Contains(t, queued, "bobs-body-2", "and the head's")
}
