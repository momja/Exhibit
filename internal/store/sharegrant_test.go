package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-lrae: a share becomes a grant, and reaching a granted artifact becomes a
// second, deny-by-default accessor rather than a wider owner predicate.
//
// The tests here divide the same way the ticket does. The first group asserts
// things the *database* refuses — written as raw INSERTs, deliberately going
// around the creation methods, because an invariant a handler upholds is a convention
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

// insertShare writes a share row directly, bypassing the store creation methods. The
// acceptance criteria say the *database* refuses a duplicate, so the test has
// to ask the database rather than the method that knows better.
func (f grantFixture) insertShare(id string, recipient *int64) error {
	if recipient == nil {
		_, err := f.s.db.ExecContext(context.Background(),
			"INSERT INTO share_links (id, artifact_id) VALUES (?, ?)", id, f.artifactID)
		return err
	}
	_, err := f.s.db.ExecContext(context.Background(),
		"INSERT INTO share_grants (artifact_id, recipient_id) VALUES (?, ?)", f.artifactID, *recipient)
	return err
}

// The artifact UNIQUE constraint makes anonymous links singular.
func TestTheDatabaseRefusesASecondAnonymousLink(t *testing.T) {
	f := newGrantFixture(t)

	require.NoError(t, f.insertShare("link-1", nil))
	err := f.insertShare("link-2", nil)
	require.Error(t, err, "a second anonymous link must be refused by the schema, not by a handler")
	assert.True(t, isUniqueViolation(err), "expected a UNIQUE constraint failure, got %v", err)

	// And through the store, where the refusal is typed so the API can answer
	// 409 rather than reporting its own invariant as a fault.
	_, err = f.s.CreateAnonymousLink(context.Background(), alice, f.artifactID)
	assert.ErrorIs(t, err, ErrDuplicateShare)
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

	_, err = f.s.CreateGrant(context.Background(), alice, f.artifactID, recipient)
	assert.ErrorIs(t, err, ErrDuplicateShare)
}

// An omitted or invalid recipient must fail before INSERT, never take the
// anonymous-link creation path. Check the error and both kinds of stored row.
func TestCreateGrantRefusesNonPositiveRecipientsWithoutPublishing(t *testing.T) {
	for _, recipientID := range []int64{0, -1} {
		t.Run(fmt.Sprint(recipientID), func(t *testing.T) {
			f := newGrantFixture(t)
			ctx := context.Background()
			grant, err := f.s.CreateGrant(ctx, alice, f.artifactID, recipientID)
			require.ErrorIs(t, err, ErrInvalidRecipient)
			assert.Nil(t, grant)

			var rows, links int
			require.NoError(t, f.s.db.QueryRowContext(ctx,
				"SELECT (SELECT COUNT(*) FROM share_grants WHERE artifact_id=?1), (SELECT COUNT(*) FROM share_links WHERE artifact_id=?1)",
				f.artifactID).Scan(&rows, &links))
			assert.Zero(t, rows, "an invalid grant creates no row")
			assert.Zero(t, links, "an invalid grant must never create an anonymous link")
		})
	}
}

// The control the two tests above need: the indexes must refuse *only* the
// duplicates. An index that refused everything would satisfy both of them.
func TestOneArtifactCarriesALinkAndAGrantPerPerson(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	toBob, toCarol := bob, seedThirdAccount(t, f.s)

	createdLink, createdLinkErr := f.s.CreateAnonymousLink(ctx, alice, f.artifactID)
	require.NoError(t, createdLinkErr)
	bobsGrant, bobsGrantErr := f.s.CreateGrant(ctx, alice, f.artifactID, toBob)
	require.NoError(t, bobsGrantErr)
	_, createdGrantErr := f.s.CreateGrant(ctx, alice, f.artifactID, toCarol)
	require.NoError(t, createdGrantErr)

	// Links resolve by id; grants name a recipient and have no link identity.
	link, err := f.s.GetShare(ctx, alice, createdLink.ID)
	require.NoError(t, err)
	require.NotNil(t, link)
	assert.Equal(t, createdLink.ID, link.ID)

	assert.Equal(t, bob, bobsGrant.RecipientID)
	shares, err := f.s.ListArtifactShares(ctx, alice, f.artifactID)
	require.NoError(t, err)
	require.Len(t, shares, 3)
	assert.Empty(t, shares[1].ID, "grants have no link id")
	assert.Empty(t, shares[2].ID)
	assert.Equal(t, bob, *shares[1].RecipientID)
	assert.Equal(t, toCarol, *shares[2].RecipientID)

	// Grants have no link id; neither an empty id nor their artifact/recipient
	// identity may resolve or revoke them through the anonymous-link accessor.
	for _, id := range []string{shares[1].ID, f.artifactID, fmt.Sprint(bobsGrant.RecipientID), "never-issued"} {
		grant, err := f.s.GetShare(ctx, alice, id)
		require.NoError(t, err)
		assert.Nil(t, grant)
		require.ErrorIs(t, f.s.DeleteShare(ctx, alice, id), ErrNotFound)
	}
	unchanged, err := f.s.ListArtifactShares(ctx, alice, f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, shares, unchanged, "id-based revocation leaves links and grants intact")

	require.NoError(t, f.s.DeleteShare(ctx, alice, createdLink.ID))
	remaining, err := f.s.ListArtifactShares(ctx, alice, f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, shares[1:], remaining, "removing the link leaves every grant intact")
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

	_, createdGrantErr := f.s.CreateGrant(ctx, alice, f.artifactID, recipient)
	require.NoError(t, createdGrantErr)

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

	_, createdLinkErr := f.s.CreateAnonymousLink(ctx, alice, f.artifactID)
	require.NoError(t, createdLinkErr)

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
			_, createdGrantErr := s.CreateGrant(ctx, alice, "alices", recipient)
			require.NoError(t, createdGrantErr)
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
	_, createdGrantErr := s.CreateGrant(ctx, alice, "alices", recipient)
	require.NoError(t, createdGrantErr)
	// The state Bob wrote as a *viewer* of Alice's artifact: her artifact
	// authorizes the reach, his id selects the rows (av-q0ub).
	require.NoError(t, s.SetState(ctx, OwnerID(alice), "alices", ViewerID(bob), "bobs-key", "bobs-value"))

	require.Equal(t, 1, countShareRowsFor(t, s, bob), "the grant must exist before it can be shown to go")
	require.Equal(t, 1, countStateRowsFor(t, s, bob))

	_, err := s.DeleteAccount(ctx, bob)
	require.NoError(t, err)

	assert.Equal(t, 0, countShareRowsFor(t, s, bob),
		"a deleted account's grants go with it (share_grants.recipient_id ON DELETE CASCADE)")
	assert.Equal(t, 0, countStateRowsFor(t, s, bob),
		"and so does the state they wrote on somebody else's artifact (migration 014's trigger)")

	// Alice's artifact and her own state are untouched — deleting a recipient
	// revokes their access, it does not reach into the library that granted it.
	a, err := s.GetArtifact(ctx, alice, "alices")
	require.NoError(t, err)
	assert.NotNil(t, a)
	assert.Equal(t, 1, countStateRowsFor(t, s, alice))
}

// AC#8. The grant dies with the artifact too, through the cascade share_grants has
// carried since 001 — so revoking everybody is a consequence of deleting the
// thing, not a second step somebody has to remember.
func TestDeletingTheArtifactTakesItsGrants(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putOwnedArtifact(t, s, alice, "alices")

	recipient := bob
	_, createdGrantErr := s.CreateGrant(ctx, alice, "alices", recipient)
	require.NoError(t, createdGrantErr)
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
		"SELECT COUNT(*) FROM share_grants WHERE recipient_id = ?", recipient).Scan(&n))
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
// A new artifact is always private-per-person: the mode is something somebody
// chose, never something an ingest inherited.
func TestAnArtifactDefaultsToItsViewersOwnState(t *testing.T) {
	f := newGrantFixture(t)

	a, err := f.s.GetArtifact(context.Background(), alice, f.artifactID)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, ShareStateOwn, a.ShareStateMode)
}

// av-6xjd gave the owner a control for the mode, so the column became
// writable — and validated on the way in. This supersedes av-lrae's "not
// caller writable", which was the right contract while nothing could set the
// value and no surface could read it back.
//
// The value check is the half worth an executable claim. share_state_mode is
// an enum the render path branches on, so an unrecognized value is not a bad
// label but a branch nobody wrote, silently deciding whose state rows a
// recipient writes.
func TestShareStateModeIsWritableOnlyAsOneOfItsTwoValues(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()

	require.NoError(t, f.s.UpdateArtifact(ctx, alice, f.artifactID,
		map[string]any{"share_state_mode": ShareStateShared}))
	a, err := f.s.GetArtifact(ctx, alice, f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, ShareStateShared, a.ShareStateMode)

	for _, bad := range []any{"everyone", "", 1, true} {
		err := f.s.UpdateArtifact(ctx, alice, f.artifactID,
			map[string]any{"share_state_mode": bad})
		assert.Error(t, err, "%v is not a mode anything honours", bad)
	}

	// And a refusal changed nothing: the stored mode is still the last value
	// that actually was one.
	a, err = f.s.GetArtifact(ctx, alice, f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, ShareStateShared, a.ShareStateMode)
}

// Exh-grj9: both detail and list reads compute badge indicators directly,
// with no share triggers or persisted rollup columns involved.
func TestSharingAnArtifactUpdatesItsCardCounts(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	recipient := bob
	var triggers int
	require.NoError(t, f.s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND tbl_name IN ('shares', 'share_links', 'share_grants')").Scan(&triggers))
	require.Zero(t, triggers, "badge updates need no share triggers")
	assert.False(t, artifactHasColumn(t, f.s.db, "share_link"))
	assert.False(t, artifactHasColumn(t, f.s.db, "share_grant_count"))

	read := func() (int, bool) {
		a, err := f.s.GetArtifact(ctx, alice, f.artifactID)
		require.NoError(t, err)
		listed, err := f.s.ListArtifacts(ctx, ListOptions{OwnerID: alice})
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, a.ShareGrantCount, listed[0].ShareGrantCount)
		assert.Equal(t, a.SharePublicLink, listed[0].SharePublicLink)
		return a.ShareGrantCount, a.SharePublicLink
	}

	grants, link := read()
	assert.Equal(t, 0, grants)
	assert.False(t, link, "an artifact nobody has shared carries no marker at all")

	createdLink, createdLinkErr := f.s.CreateAnonymousLink(ctx, alice, f.artifactID)
	require.NoError(t, createdLinkErr)
	grants, link = read()
	assert.Equal(t, 0, grants)
	assert.True(t, link)

	_, createdGrantErr := f.s.CreateGrant(ctx, alice, f.artifactID, recipient)
	require.NoError(t, createdGrantErr)
	grants, link = read()
	assert.Equal(t, 1, grants)
	assert.True(t, link, "a grant does not disturb the link's count, nor the reverse")

	require.NoError(t, f.s.DeleteShare(ctx, alice, createdLink.ID))
	grants, link = read()
	assert.Equal(t, 1, grants)
	assert.False(t, link)

	require.NoError(t, f.s.DeleteGrant(ctx, alice, f.artifactID, recipient))
	grants, link = read()
	assert.Equal(t, 0, grants)
	assert.False(t, link, "revoking the last share returns the card to unmarked")
}

// Account deletion cascades grants away; the next read must show that fact.
func TestDeletingARecipientLeavesTheCardCountCorrect(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	recipient := bob

	_, createdGrantErr := f.s.CreateGrant(ctx, alice, f.artifactID, recipient)
	require.NoError(t, createdGrantErr)
	_, err := f.s.DeleteAccount(ctx, recipient)
	require.NoError(t, err)

	a, err := f.s.GetArtifact(ctx, alice, f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, 0, a.ShareGrantCount)
}

// The enumeration half (av-6xjd): the owner's panel lists both kinds in one
// read, each grant carrying the account it names, because a grant is only
// legible as the person it was given to.
func TestListArtifactSharesNamesTheLinkAndEveryGrant(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	recipient := bob

	createdLink, createdLinkErr := f.s.CreateAnonymousLink(ctx, alice, f.artifactID)
	require.NoError(t, createdLinkErr)
	_, createdGrantErr := f.s.CreateGrant(ctx, alice, f.artifactID, recipient)
	require.NoError(t, createdGrantErr)

	shares, err := f.s.ListArtifactShares(ctx, alice, f.artifactID)
	require.NoError(t, err)
	require.Len(t, shares, 2)

	assert.Equal(t, createdLink.ID, shares[0].ID)
	assert.Nil(t, shares[0].RecipientID, "the link names nobody; that is what makes it the link")
	assert.Nil(t, shares[0].Recipient)

	assert.Empty(t, shares[1].ID, "grants have no id")
	require.NotNil(t, shares[1].Recipient)
	assert.Equal(t, bob, shares[1].Recipient.ID)

	// And the guest list is the owner's. Another owner asking gets what they
	// get for an artifact that does not exist: nothing, never a refusal.
	other, err := f.s.ListArtifactShares(ctx, bob, f.artifactID)
	require.NoError(t, err)
	assert.Empty(t, other)
}

// Rotating the link is one operation, not a delete a caller follows with a
// create: the gap between those two is an artifact with no link at all, which
// is worse than either the old link or the new one.
func TestReplaceAnonymousLinkRotatesInOneStep(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	recipient := bob

	createdLink, createdLinkErr := f.s.CreateAnonymousLink(ctx, alice, f.artifactID)
	require.NoError(t, createdLinkErr)
	fresh, err := f.s.ReplaceAnonymousLink(ctx, alice, f.artifactID, "new")
	require.NoError(t, err)
	assert.Equal(t, "new", fresh.ID)

	// The old id is gone, which is the whole point: a leaked link stops
	// working the moment it is replaced.
	gone, err := f.s.GetAnonymousShareUnscoped(ctx, createdLink.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	live, err := f.s.GetAnonymousShareUnscoped(ctx, "new")
	require.NoError(t, err)
	require.NotNil(t, live)

	// Grants are untouched: replacing the public link is not a revocation of
	// the people who were named.
	_, createdGrantErr := f.s.CreateGrant(ctx, alice, f.artifactID, recipient)
	require.NoError(t, createdGrantErr)
	_, err = f.s.ReplaceAnonymousLink(ctx, alice, f.artifactID, "newer")
	require.NoError(t, err)
	shares, err := f.s.ListArtifactShares(ctx, alice, f.artifactID)
	require.NoError(t, err)
	assert.Len(t, shares, 2)

	// Replacing on an artifact with no link yet is a mint, so a caller need
	// not know which of the two cases it is in.
	require.NoError(t, f.s.DeleteShare(ctx, alice, "newer"))
	minted, err := f.s.ReplaceAnonymousLink(ctx, alice, f.artifactID, "first-one")
	require.NoError(t, err)
	assert.Equal(t, "first-one", minted.ID)

	// And it is owner-scoped like every other write that names an artifact.
	_, err = f.s.ReplaceAnonymousLink(ctx, bob, f.artifactID, "stolen")
	assert.ErrorIs(t, err, ErrNotFound)
}

// Resolving a typed handle to an account (av-6xjd), and the one case that has
// to refuse rather than choose. external_id is UNIQUE so a login name is
// exact; email is not, and picking between two accounts that share an address
// would hand somebody's artifact to whichever of them was created first.
func TestGetUserByEmailRefusesAnAmbiguousAddress(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	one, err := s.UpsertUser(ctx, "sub-one", "shared@example.test")
	require.NoError(t, err)
	found, err := s.GetUserByEmail(ctx, "shared@example.test")
	require.NoError(t, err)
	assert.Equal(t, one.ID, found.ID)

	// An address is not case-sensitive to the person typing it.
	found, err = s.GetUserByEmail(ctx, "Shared@Example.test")
	require.NoError(t, err)
	assert.Equal(t, one.ID, found.ID)

	_, err = s.UpsertUser(ctx, "sub-two", "shared@example.test")
	require.NoError(t, err)
	_, err = s.GetUserByEmail(ctx, "shared@example.test")
	assert.ErrorIs(t, err, ErrAmbiguousUser)

	// The blank address matches nothing. users.email is NOT NULL DEFAULT '',
	// so without that guard every account a provider named no address for
	// would answer to an empty field.
	_, err = s.UpsertUser(ctx, "sub-nameless", "")
	require.NoError(t, err)
	_, err = s.GetUserByEmail(ctx, "")
	assert.ErrorIs(t, err, ErrNotFound)
}
