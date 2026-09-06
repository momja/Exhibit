package store

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-lrae: a share becomes a grant, and reaching a granted artifact becomes a
// second, deny-by-default accessor rather than a wider owner predicate.
//
// The tests here divide the same way the ticket does. The first group asserts
// things the *database* refuses — written as raw INSERTs, deliberately going
// around CreateShare, because an invariant a handler upholds is a convention
// and an invariant an index upholds is a fact. The second group asserts what a
// recipient can and cannot reach, and the second half of that is the whole
// argument for a separate accessor: every mutating method must still deny them
// exactly as it denies a stranger.

// grantFixture is Alice's artifact, Bob's account, and no relationship between
// them yet. Each test establishes the one it needs.
type grantFixture struct {
	s          *SQLiteStore
	artifactID string
}

// Deliberately not putOwnedArtifact: that helper seeds an anonymous link of
// its own, and these tests are about which share rows may exist, so they need
// an artifact carrying none.
func newGrantFixture(t *testing.T) grantFixture {
	t.Helper()
	s := newTestStore(t)
	seedOwnerAccounts(t, s)
	require.NoError(t, s.PutArtifact(context.Background(), &Artifact{
		ID: "alices", OwnerID: alice, Title: "alices", SourceBlobID: "blob-alices", Tier: Tier1,
	}))
	return grantFixture{s: s, artifactID: "alices"}
}

// insertShare writes a share row directly, bypassing CreateShare entirely. The
// acceptance criteria say the *database* refuses a duplicate, so the test has
// to ask the database rather than the method that knows better.
func (f grantFixture) insertShare(id string, recipient *int64) error {
	_, err := f.s.db.ExecContext(context.Background(),
		"INSERT INTO shares (id, artifact_id, recipient_id) VALUES (?, ?, ?)",
		id, f.artifactID, recipient)
	return err
}

// AC#2. The composite index does not deliver this and cannot: SQLite treats
// every NULL in a unique index as distinct from every other NULL, so
// UNIQUE(artifact_id, recipient_id) accepts any number of recipient-less rows.
// The partial index is what makes the anonymous link singular, and this is the
// test that fails if it is ever dropped as redundant.
func TestTheDatabaseRefusesASecondAnonymousLink(t *testing.T) {
	f := newGrantFixture(t)

	require.NoError(t, f.insertShare("link-1", nil))
	err := f.insertShare("link-2", nil)
	require.Error(t, err, "a second anonymous link must be refused by the schema, not by a handler")
	assert.True(t, isUniqueViolation(err), "expected a UNIQUE constraint failure, got %v", err)

	// And through the store, where the refusal is typed so the API can answer
	// 409 rather than reporting its own invariant as a fault.
	assert.ErrorIs(t,
		f.s.CreateShare(context.Background(), alice, &Share{ID: "link-3", ArtifactID: f.artifactID}),
		ErrDuplicateShare)
}

// AC#3. Two grants of one artifact to one account would be two answers to a
// question that has one.
func TestTheDatabaseRefusesTwoGrantsToOneUser(t *testing.T) {
	f := newGrantFixture(t)
	recipient := bob

	require.NoError(t, f.insertShare("grant-1", &recipient))
	err := f.insertShare("grant-2", &recipient)
	require.Error(t, err, "a repeated grant must be refused by the schema, not by a handler")
	assert.True(t, isUniqueViolation(err), "expected a UNIQUE constraint failure, got %v", err)

	assert.ErrorIs(t,
		f.s.CreateShare(context.Background(), alice,
			&Share{ID: "grant-3", ArtifactID: f.artifactID, RecipientID: &recipient}),
		ErrDuplicateShare)
}

// The control the two tests above need: the indexes must refuse *only* the
// duplicates. An index that refused everything would satisfy both of them.
func TestOneArtifactCarriesALinkAndAGrantPerPerson(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	toBob, toCarol := bob, seedThirdAccount(t, f.s)

	require.NoError(t, f.s.CreateShare(ctx, alice, &Share{ID: "the-link", ArtifactID: f.artifactID}))
	require.NoError(t, f.s.CreateShare(ctx, alice,
		&Share{ID: "to-bob", ArtifactID: f.artifactID, RecipientID: &toBob}))
	require.NoError(t, f.s.CreateShare(ctx, alice,
		&Share{ID: "to-carol", ArtifactID: f.artifactID, RecipientID: &toCarol}))

	// Read back: the link and the grants stay told apart by recipient_id
	// alone, which is the only thing that distinguishes them.
	link, err := f.s.GetShare(ctx, alice, "the-link")
	require.NoError(t, err)
	require.NotNil(t, link)
	assert.Nil(t, link.RecipientID, "no recipient is what makes it the anonymous link")

	grant, err := f.s.GetShare(ctx, alice, "to-bob")
	require.NoError(t, err)
	require.NotNil(t, grant)
	require.NotNil(t, grant.RecipientID)
	assert.Equal(t, bob, *grant.RecipientID)
}

// seedThirdAccount adds an account beyond alice and bob and returns its id, for
// the cases that need two distinct recipients.
func seedThirdAccount(t *testing.T, s *SQLiteStore) int64 {
	t.Helper()
	u, err := s.UpsertUser(context.Background(), "sub-carol", "carol@example.test")
	require.NoError(t, err)
	return u.ID
}

// A grant names a real account, and the foreign key says so. Without it a
// grant could be written to a user id that never existed — a row nobody can
// ever hold and nobody can ever revoke by deleting their account.
func TestAGrantMustNameAnAccountThatExists(t *testing.T) {
	f := newGrantFixture(t)
	ghost := int64(9999)
	assert.Error(t, f.insertShare("to-nobody", &ghost),
		"a grant to a nonexistent account must be refused by the foreign key")
}

// AC#5 and AC#6, the read half: the recipient reaches the artifact through the
// new accessor and everyone else reads it as absent.
func TestGetArtifactReadableByAdmitsTheOwnerAndTheGrantee(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	recipient := bob
	carol := seedThirdAccount(t, f.s)

	// Before the grant, Bob is indistinguishable from a stranger, and both are
	// indistinguishable from an artifact that does not exist (AC#6).
	ungranted, err := f.s.GetArtifactReadableBy(ctx, ViewerID(bob), f.artifactID)
	require.NoError(t, err, "an ungranted read is absence, not an error")
	assert.Nil(t, ungranted)

	missing, err := f.s.GetArtifactReadableBy(ctx, ViewerID(bob), "no-such-artifact")
	require.NoError(t, err)
	assert.Nil(t, missing)

	require.NoError(t, f.s.CreateShare(ctx, alice,
		&Share{ID: "to-bob", ArtifactID: f.artifactID, RecipientID: &recipient}))

	granted, err := f.s.GetArtifactReadableBy(ctx, ViewerID(bob), f.artifactID)
	require.NoError(t, err)
	require.NotNil(t, granted, "a grant is what makes the artifact readable by somebody else")
	assert.Equal(t, alice, granted.OwnerID, "and it is still the owner's artifact")

	// The owner reads their own through the same accessor, so one read path
	// serves both and the caller need not know which it is holding.
	own, err := f.s.GetArtifactReadableBy(ctx, ViewerID(alice), f.artifactID)
	require.NoError(t, err)
	assert.NotNil(t, own)

	// Somebody else's grant is not theirs, and a caller that resolved no
	// principal at all fails closed.
	third, err := f.s.GetArtifactReadableBy(ctx, ViewerID(carol), f.artifactID)
	require.NoError(t, err)
	assert.Nil(t, third, "a grant admits exactly the account it names")

	nobody, err := f.s.GetArtifactReadableBy(ctx, ViewerID(0), f.artifactID)
	require.NoError(t, err)
	assert.Nil(t, nobody, "an unresolved viewer must match no owner and no grant")
}

// The anonymous link is authorized by holding its id at /s/:shareID, never by
// being logged in as somebody. So a link on an artifact must not make it
// readable to every account on the instance — which is what a predicate
// matching NULL recipients would quietly have done.
func TestAnAnonymousLinkGrantsNobodyInParticular(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()

	require.NoError(t, f.s.CreateShare(ctx, alice, &Share{ID: "the-link", ArtifactID: f.artifactID}))

	got, err := f.s.GetArtifactReadableBy(ctx, ViewerID(bob), f.artifactID)
	require.NoError(t, err)
	assert.Nil(t, got, "a public link is not a grant to every account on the instance")
}

// AC#4 and AC#5, the half that matters most: the read accessor admits a
// recipient and *nothing else does*. Every method the owner-scoped predicate
// guards must deny them exactly as it denies a stranger — same shape, same
// silence, no 403 anywhere — because widening that predicate instead of adding
// an accessor beside it is precisely what would have handed a recipient the
// DELETE and the body rewrite along with the read.
//
// It reuses ownerCases() deliberately: a method added there is covered here on
// the day it is added, which is the only way this stays true.
//
// **State is the one deliberate exception, and it was decided rather than
// leaked** (av-v991). A recipient may write the state of an artifact they were
// granted — a tool whose saved data evaporates on reload is not a tool they can
// use, which is the epic's central promise. That exception does not live here:
// the four methods this test walks are still owner-scoped and still deny a
// grantee, exactly as asserted below. It lives in the four ...AsViewer methods
// beside them, which take one principal and resolve whose rows they touch from
// the artifact's share_state_mode — see TestAGranteeWritesStateAndOnlyState
// and TestSharedModePutsEveryViewerOnTheOwnersBoard, and read them together
// with this one. Whatever else moves, the closing assertions here must keep
// holding: a grantee may not reach the artifact itself, and under the default
// mode may not touch the owner's state rows either.
func TestAGrantDoesNotWidenAnyOwnerScopedMethod(t *testing.T) {
	for _, tc := range ownerCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			putOwnedArtifact(t, s, alice, "alices")
			putOwnedArtifact(t, s, bob, "bobs")

			// Alice grants Bob her artifact. He can read it; that is all.
			recipient := bob
			require.NoError(t, s.CreateShare(ctx, alice,
				&Share{ID: "to-bob", ArtifactID: "alices", RecipientID: &recipient}))
			readable, err := s.GetArtifactReadableBy(ctx, ViewerID(bob), "alices")
			require.NoError(t, err)
			require.NotNil(t, readable, "the grant must actually be in force")

			empty, err := tc.run(ctx, s, bob, "alices")

			switch tc.deny {
			case denyEmptyRead:
				require.NoError(t, err, "a grant is not a reason to error")
				assert.True(t, empty,
					"%s must read as absent for a grantee — a grant is read access to the "+
						"artifact, not to the owner's rows beneath it", tc.name)
			case denyErrNotFound:
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrNotFound),
					"%s must refuse a grantee with ErrNotFound (404), never a permission error; got %v",
					tc.name, err)
			case denySilentNoop:
				require.NoError(t, err, "these deletes are idempotent by contract")
			}

			// Alice's artifact survived whatever Bob attempted, and it is
			// still hers.
			a, err := s.GetArtifact(ctx, alice, "alices")
			require.NoError(t, err)
			require.NotNil(t, a, "a grantee must not be able to delete what they were shown")
			assert.Equal(t, "alices", a.Title, "nor rewrite it")
			assert.Equal(t, []string{"https://seed.example.com"}, a.NetworkAllowlist,
				"nor widen the allowlist the CSP is built from")
			assert.False(t, a.DownloadsApproved, "nor spend a capability approval on her behalf")

			state, err := s.GetState(ctx, OwnerID(alice), "alices", ViewerID(alice))
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"seed": "value"}, state,
				"nor read, plant, or erase the owner's own state rows")

			sh, err := s.GetShare(ctx, alice, "share-alices")
			require.NoError(t, err)
			assert.NotNil(t, sh, "nor revoke a share they do not own")
		})
	}
}

// AC#7. Two cascades retire what a recipient leaves behind on somebody else's
// artifact, and they are separate mechanisms for a reason worth keeping
// straight: the grant goes by a real foreign key (a grant names a *named*
// account by definition), and the state rows by migration 014's trigger on
// users (state must hold rows for owner 1 on a static-token instance with an
// empty users table, so a foreign key there would reject every write on the
// commonest deployment).
func TestDeletingARecipientTakesTheirGrantsAndTheirState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putOwnedArtifact(t, s, alice, "alices")

	// Alice keeps her admin rights, so deleting Bob is not refused as the last
	// enabled admin.
	recipient := bob
	require.NoError(t, s.CreateShare(ctx, alice,
		&Share{ID: "to-bob", ArtifactID: "alices", RecipientID: &recipient}))
	// The state Bob wrote as a *viewer* of Alice's artifact: her artifact
	// authorizes the reach, his id selects the rows (av-q0ub).
	require.NoError(t, s.SetState(ctx, OwnerID(alice), "alices", ViewerID(bob), "bobs-key", "bobs-value"))

	require.Equal(t, 1, countShareRowsFor(t, s, bob), "the grant must exist before it can be shown to go")
	require.Equal(t, 1, countStateRowsFor(t, s, bob))

	_, err := s.DeleteAccount(ctx, bob)
	require.NoError(t, err)

	assert.Equal(t, 0, countShareRowsFor(t, s, bob),
		"a deleted account's grants go with it (shares.recipient_id ON DELETE CASCADE)")
	assert.Equal(t, 0, countStateRowsFor(t, s, bob),
		"and so does the state they wrote on somebody else's artifact (migration 014's trigger)")

	// Alice's artifact and her own state are untouched — deleting a recipient
	// revokes their access, it does not reach into the library that granted it.
	a, err := s.GetArtifact(ctx, alice, "alices")
	require.NoError(t, err)
	assert.NotNil(t, a)
	assert.Equal(t, 1, countStateRowsFor(t, s, alice))
}

// AC#8. The grant dies with the artifact too, through the cascade shares has
// carried since 001 — so revoking everybody is a consequence of deleting the
// thing, not a second step somebody has to remember.
func TestDeletingTheArtifactTakesItsGrants(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putOwnedArtifact(t, s, alice, "alices")

	recipient := bob
	require.NoError(t, s.CreateShare(ctx, alice,
		&Share{ID: "to-bob", ArtifactID: "alices", RecipientID: &recipient}))
	require.Equal(t, 1, countShareRowsFor(t, s, bob))

	_, err := s.DeleteArtifact(ctx, alice, "alices")
	require.NoError(t, err)

	assert.Equal(t, 0, countShareRowsFor(t, s, bob),
		"deleting an artifact revokes every grant over it (ON DELETE CASCADE from artifacts)")

	// And the grantee's read goes with it, rather than surviving as a row
	// pointing at nothing.
	got, err := s.GetArtifactReadableBy(ctx, ViewerID(bob), "alices")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func countShareRowsFor(t *testing.T, s *SQLiteStore, recipient int64) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM shares WHERE recipient_id = ?", recipient).Scan(&n))
	return n
}

func countStateRowsFor(t *testing.T, s *SQLiteStore, viewer int64) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM artifact_state WHERE user_id = ?", viewer).Scan(&n))
	return n
}

// share_state_mode's default, asserted through the interface it is read by.
// One answer per artifact, and until av-v991 ships the modes beyond it, that
// answer is always this one — a recipient writes their own rows, never the
// owner's.
func TestAnArtifactDefaultsToItsViewersOwnState(t *testing.T) {
	f := newGrantFixture(t)

	a, err := f.s.GetArtifact(context.Background(), alice, f.artifactID)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, ShareStateOwn, a.ShareStateMode)
}

// The column is read and never written, which is the claim the comment on
// Artifact.ShareStateMode makes and this is the executable half of it: a caller
// cannot store a mode nothing honours yet.
func TestShareStateModeIsNotCallerWritable(t *testing.T) {
	f := newGrantFixture(t)

	err := f.s.UpdateArtifact(context.Background(), alice, f.artifactID,
		map[string]any{"share_state_mode": "shared"})
	assert.ErrorIs(t, err, ErrNotUpdatable)
}
