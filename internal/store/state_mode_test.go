package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-v991: what share_state_mode MEANS, and the write path a granted recipient
// reaches state through.
//
// Two claims are under test here and they pull in opposite directions, which is
// why they are in one file. A grantee must be able to write state, or a shared
// tool forgets everything the moment its frame reloads and the epic's central
// promise is untrue. And a grantee must still reach nothing else — that half is
// TestAGrantDoesNotWidenAnyOwnerScopedMethod's, and this file is the exception
// it points at.

// sharedFixture is Alice's artifact granted to Bob, in whichever state mode the
// test needs. The mode is set with a direct UPDATE on purpose: it is not
// caller-writable through the Store yet (av-6xjd owns the PATCH surface), and
// these tests are about what the value MEANS, not about who may set it.
type sharedFixture struct {
	s          *SQLiteStore
	artifactID string
}

func newSharedFixture(t *testing.T, mode string) sharedFixture {
	t.Helper()
	s := newTestStore(t)
	ctx := context.Background()
	putOwnedArtifact(t, s, alice, "alices")
	recipient := bob
	require.NoError(t, s.CreateShare(ctx, alice,
		&Share{ID: "to-bob", ArtifactID: "alices", RecipientID: &recipient}))
	if mode != ShareStateOwn {
		_, err := s.db.ExecContext(ctx,
			"UPDATE artifacts SET share_state_mode = ? WHERE id = ?", mode, "alices")
		require.NoError(t, err)
	}
	return sharedFixture{s: s, artifactID: "alices"}
}

// The default, and the case av-awr4 found broken: Bob writes, and what he wrote
// is his. Before this the write simply failed — SetState gated on ownsArtifact —
// so 'own' mode, the mode every artifact is in, did not work at all.
func TestAGranteeWritesTheirOwnRowsUnderTheDefaultMode(t *testing.T) {
	f := newSharedFixture(t, ShareStateOwn)
	ctx := context.Background()

	require.NoError(t, f.s.SetStateAsViewer(ctx, ViewerID(bob), f.artifactID, "note", "bob's"))

	his, err := f.s.GetStateAsViewer(ctx, ViewerID(bob), f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"note": "bob's"}, his,
		"a recipient reads back what they wrote, which is the whole of using a shared tool")

	// And it landed nowhere near Alice. Her seed row is untouched and her view
	// of the artifact never learns Bob exists.
	hers, err := f.s.GetStateAsViewer(ctx, ViewerID(alice), f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"seed": "value"}, hers,
		"'own' is av-q0ub's per-viewer isolation, unchanged: two people, two sets of rows")
}

// 'shared' is the whole point of the mode existing: one board. Both of them
// write the same rows and each sees what the other left, which is what a
// two-player artifact means.
func TestSharedModePutsEveryViewerOnTheOwnersBoard(t *testing.T) {
	f := newSharedFixture(t, ShareStateShared)
	ctx := context.Background()

	require.NoError(t, f.s.SetStateAsViewer(ctx, ViewerID(bob), f.artifactID, "position", "e4"))

	hers, err := f.s.GetStateAsViewer(ctx, ViewerID(alice), f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, "e4", hers["position"],
		"the owner must see the move the recipient made; a board only one of them can see is not a board")

	// The rows are literally the owner's, not a copy kept in step: Bob holds
	// none of his own on this artifact.
	assert.Equal(t, 0, countStateRowsFor(t, f.s, bob),
		"'shared' means one row set, so a viewer writing it must acquire no rows of their own")

	require.NoError(t, f.s.SetStateAsViewer(ctx, ViewerID(alice), f.artifactID, "position", "e5"))
	his, err := f.s.GetStateAsViewer(ctx, ViewerID(bob), f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, "e5", his["position"], "and the traffic runs both ways")
}

// The resolution is the artifact's answer, not the caller's, and this is that
// stated as the unit it is. Nothing outside StatePrincipal reads
// share_state_mode, so a mode nobody recognises — an empty string on a
// zero-valued Artifact, a value written before this code existed — has to fail
// towards isolation rather than towards the owner's rows.
func TestStatePrincipalResolvesTheModeAndFailsTowardsIsolation(t *testing.T) {
	owned := &Artifact{OwnerID: alice, ShareStateMode: ShareStateOwn}
	assert.Equal(t, ViewerID(bob), owned.StatePrincipal(ViewerID(bob)))

	shared := &Artifact{OwnerID: alice, ShareStateMode: ShareStateShared}
	assert.Equal(t, ViewerID(alice), shared.StatePrincipal(ViewerID(bob)))

	unset := &Artifact{OwnerID: alice}
	assert.Equal(t, ViewerID(bob), unset.StatePrincipal(ViewerID(bob)),
		"an unrecognised mode must resolve to the viewer's own rows; the alternative "+
			"is silently putting a stranger on the owner's board")
}

// The owner reaches their own state through the same four methods, so there is
// one path rather than an owner's and a grantee's — which is what stops the two
// resolving the mode differently.
func TestTheOwnerUsesTheSameViewerPathAsAGrantee(t *testing.T) {
	f := newSharedFixture(t, ShareStateOwn)
	ctx := context.Background()

	require.NoError(t, f.s.SetStateAsViewer(ctx, ViewerID(alice), f.artifactID, "note", "hers"))
	hers, err := f.s.GetStateAsViewer(ctx, ViewerID(alice), f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, "hers", hers["note"])

	require.NoError(t, f.s.DeleteStateAsViewer(ctx, ViewerID(alice), f.artifactID, "note"))
	require.NoError(t, f.s.ClearStateAsViewer(ctx, ViewerID(alice), f.artifactID))
	hers, err = f.s.GetStateAsViewer(ctx, ViewerID(alice), f.artifactID)
	require.NoError(t, err)
	assert.Empty(t, hers)
}

// "Erase all" means MINE (av-q0ub), and under 'own' that is exactly the
// grantee's rows. A recipient tidying up after themselves must not be able to
// wipe the library owner's saved data as a side effect.
func TestAGranteesEraseAllTakesOnlyTheirOwnRows(t *testing.T) {
	f := newSharedFixture(t, ShareStateOwn)
	ctx := context.Background()

	require.NoError(t, f.s.SetStateAsViewer(ctx, ViewerID(bob), f.artifactID, "note", "bob's"))
	require.NoError(t, f.s.ClearStateAsViewer(ctx, ViewerID(bob), f.artifactID))

	assert.Equal(t, 0, countStateRowsFor(t, f.s, bob))
	hers, err := f.s.GetStateAsViewer(ctx, ViewerID(alice), f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"seed": "value"}, hers,
		"erasing your own state must leave the owner's alone")
}

// The counterweight to the whole file: the parallel path is authorized by the
// grant and by nothing else. A stranger gets the shapes a stranger has always
// got — absence on a read, ErrNotFound on a write, silence on a delete — so
// widening state widened it for the people named on a grant and for no one else.
func TestTheViewerStatePathAdmitsNobodyWithoutAGrant(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putOwnedArtifact(t, s, alice, "alices")
	carol := seedThirdAccount(t, s)

	empty, err := s.GetStateAsViewer(ctx, ViewerID(carol), "alices")
	require.NoError(t, err, "an ungranted read is absence, not an error")
	assert.Empty(t, empty)

	assert.ErrorIs(t, s.SetStateAsViewer(ctx, ViewerID(carol), "alices", "planted", "by a stranger"),
		ErrNotFound, "and a write is 404, never a permission error that would confirm the row exists")

	require.NoError(t, s.DeleteStateAsViewer(ctx, ViewerID(carol), "alices", "seed"),
		"the deletes stay idempotent: rows a caller cannot reach are not rows they asked to remove")
	require.NoError(t, s.ClearStateAsViewer(ctx, ViewerID(carol), "alices"))

	hers, err := s.GetStateAsViewer(ctx, ViewerID(alice), "alices")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"seed": "value"}, hers,
		"nothing a stranger attempted touched the owner's rows")

	// A caller that resolved no principal at all fails closed too, on the same
	// predicate — the zero value matches no owner and no grant.
	assert.ErrorIs(t, s.SetStateAsViewer(ctx, ViewerID(0), "alices", "planted", "by nobody"), ErrNotFound)
}

// The anonymous link is authorized by holding its id at /s/:shareID, never by
// being logged in as somebody — so a link on an artifact must not hand every
// account on the instance a pen. This is TestAnAnonymousLinkGrantsNobodyInParticular
// carried onto the write path, where getting it wrong costs more than a read.
func TestAnAnonymousLinkGivesNobodyTheWritePath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// putOwnedArtifact already seeds the artifact's anonymous link.
	putOwnedArtifact(t, s, alice, "alices")

	assert.ErrorIs(t, s.SetStateAsViewer(ctx, ViewerID(bob), "alices", "planted", "by a link holder"),
		ErrNotFound, "a public link is not a grant to every account on the instance")
}

// Deleting a grant revokes the write in the same motion — no second lifecycle,
// which is the argument that put state_mode on the artifact and left the grant
// as the only thing to revoke.
func TestRevokingAGrantRevokesTheWrite(t *testing.T) {
	f := newSharedFixture(t, ShareStateOwn)
	ctx := context.Background()

	require.NoError(t, f.s.SetStateAsViewer(ctx, ViewerID(bob), f.artifactID, "note", "bob's"))
	require.NoError(t, f.s.DeleteShare(ctx, alice, "to-bob"))

	assert.ErrorIs(t, f.s.SetStateAsViewer(ctx, ViewerID(bob), f.artifactID, "note", "still bob's?"),
		ErrNotFound)
	gone, err := f.s.GetStateAsViewer(ctx, ViewerID(bob), f.artifactID)
	require.NoError(t, err)
	assert.Empty(t, gone, "and the rows they wrote stop being readable by them too")
}
