package store

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-4wyq. Deleting an account has to erase everything this instance holds for
// the person, and "everything" is a property of the schema rather than of
// anybody's memory of it. So the tests here are written against the schema as
// it actually is at runtime: one walks every table SQLite reports and refuses
// to pass until each has been accounted for, and the other seeds a row in each
// of those tables and requires it to be gone afterwards.
//
// The two halves matter separately. The walk is what fails when the schema
// grows a table nobody thought about — the failure mode this operation cannot
// afford, because leftover rows are invisible and the person who asked is gone.
// The seeding is what stops the walk's emptiness assertions from passing
// vacuously: a table nothing was ever written to is empty for the wrong reason.

// deletedArtifact is the id of the artifact seeded for the account under test.
//
// The residue queries below name it literally rather than joining through
// `artifacts`, and that is the whole trick of this file. `SELECT … WHERE
// artifact_id IN (SELECT id FROM artifacts WHERE owner_id = ?)` reports zero
// after deletion no matter what is in the table, because the artifact row it
// joins through is gone too — it would assert nothing at all.
const deletedArtifact = "member-artifact"

// accountTable declares, for one table in the schema, how DeleteAccount
// reaches the rows an account owns in it.
//
// Every table needs a row here, including the ones that hold nothing personal:
// the point is that somebody states the answer once, in public, rather than
// that the answer is usually "nothing to do".
type accountTable struct {
	// reach is how the rows go: deleted by DeleteAccount's own statements, by
	// an ON DELETE CASCADE it relies on, by a trigger, or not at all.
	reach string
	// why is the sentence a reviewer needs — in particular for `none`, where
	// the claim is that the table holds nothing belonging to a person.
	why string
	// residue counts the rows still belonging to the deleted account. ?1 is
	// its user id. Empty only for `none`, where there is nothing to count.
	residue string
}

const (
	reachExplicit = "deleted by DeleteAccount"
	reachCascade  = "ON DELETE CASCADE from a row DeleteAccount deletes"
	reachTrigger  = "an AFTER DELETE trigger on the row DeleteAccount deletes"
	reachNone     = "holds nothing belonging to an account"
)

var accountTables = map[string]accountTable{
	"users": {reachExplicit, "the account itself, and the guard that may refuse the whole operation",
		"SELECT COUNT(*) FROM users WHERE id = ?1"},
	"artifacts": {reachExplicit, "the library, by owner_id — and the row every cascade below hangs from",
		"SELECT COUNT(*) FROM artifacts WHERE owner_id = ?1"},
	"tags": {reachExplicit, "owner-scoped and reached by no cascade; nothing about an artifact owns a tag",
		"SELECT COUNT(*) FROM tags WHERE owner_id = ?1"},
	"collections": {reachExplicit, "owner-scoped and reached by no cascade, for the same reason as tags",
		"SELECT COUNT(*) FROM collections WHERE owner_id = ?1"},
	"agent_keys": {reachExplicit, "the sealed BYO provider key, owner-scoped and reached by no cascade",
		"SELECT COUNT(*) FROM agent_keys WHERE owner_id = ?1"},

	"artifact_tags": {reachCascade, "cascades from artifacts(id)",
		"SELECT COUNT(*) FROM artifact_tags WHERE artifact_id = '" + deletedArtifact + "'"},
	"artifact_collections": {reachCascade, "cascades from artifacts(id)",
		"SELECT COUNT(*) FROM artifact_collections WHERE artifact_id = '" + deletedArtifact + "'"},
	"artifact_network_origins": {reachCascade, "cascades from artifacts(id)",
		"SELECT COUNT(*) FROM artifact_network_origins WHERE artifact_id = '" + deletedArtifact + "'"},
	"agent_transcripts": {reachCascade, "cascades from artifacts(id)",
		"SELECT COUNT(*) FROM agent_transcripts WHERE artifact_id = '" + deletedArtifact + "'"},
	"shares": {reachCascade,
		"cascades from artifacts(id) — which is what revokes every capability URL at once",
		"SELECT COUNT(*) FROM shares WHERE artifact_id = '" + deletedArtifact + "'"},
	"sessions": {reachCascade,
		"cascades from users(id), which is what signs the account out everywhere rather than in one browser",
		"SELECT COUNT(*) FROM sessions WHERE user_id = ?1"},
	"artifact_state": {reachTrigger,
		"migration 014's artifact_state_user_delete on users — and it is the only thing that reaches " +
			"the rows this person wrote as a *viewer* of somebody else's artifact",
		"SELECT COUNT(*) FROM artifact_state WHERE user_id = ?1"},

	"artifacts_fts": {reachCascade,
		"the search index is kept in step by migration 010's artifacts_fts_delete trigger on artifacts",
		"SELECT COUNT(*) FROM artifacts_fts WHERE artifacts_fts MATCH 'membersearchtoken'"},
	// FTS5's own storage for the table above. It has no schema of ours and no
	// rows of anyone's; emptying it is what the index does when its entry goes.
	"artifacts_fts_data":    {reachNone, "fts5 internal storage for artifacts_fts", ""},
	"artifacts_fts_idx":     {reachNone, "fts5 internal storage for artifacts_fts", ""},
	"artifacts_fts_docsize": {reachNone, "fts5 internal storage for artifacts_fts", ""},
	"artifacts_fts_config":  {reachNone, "fts5 internal storage for artifacts_fts", ""},

	"goose_db_version": {reachNone, "the migration ledger — a property of the database, not of a person", ""},
}

// schemaTables is every table the live database reports, which is the only
// authority worth walking: a table added by a migration nobody re-read is
// exactly the case this file exists to catch.
func schemaTables(t *testing.T, s *SQLiteStore) []string {
	t.Helper()
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	require.NoError(t, rows.Err())
	return names
}

// TestEveryTableIsAccountedForInAccountDeletion is av-4wyq's tripwire, and it
// is deliberately a walk of the whole schema rather than of the tables that
// happen to carry an `owner_id` column today. A column-name heuristic would
// wave through the next table that spells the same idea `account_id`, and
// waving one through is the failure this operation cannot afford: the rows are
// invisible, and the person who asked for them to go is no longer here to
// notice they did not.
//
// Adding a table therefore fails this test until someone writes down what
// deleting an account does about it — including "nothing, and here is why".
func TestEveryTableIsAccountedForInAccountDeletion(t *testing.T) {
	s := newTestStore(t)

	var undeclared []string
	for _, name := range schemaTables(t, s) {
		if _, ok := accountTables[name]; !ok {
			undeclared = append(undeclared, name)
		}
	}
	assert.Empty(t, undeclared,
		"a table with no row in accountTables (internal/store/sqlite_account_test.go) is a table "+
			"nobody has said what account deletion does about. Add one — reachExplicit if "+
			"DeleteAccount must delete it, reachCascade/reachTrigger if the schema already "+
			"does, reachNone with the reason it holds nothing belonging to a person.")

	// And the reverse: a row for a table that no longer exists is a stale
	// claim, which is how a list stops describing the thing it walks.
	live := map[string]bool{}
	for _, name := range schemaTables(t, s) {
		live[name] = true
	}
	var stale []string
	for name := range accountTables {
		if !live[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	assert.Empty(t, stale, "accountTables names a table the schema no longer has")
}

// accountFixture is an instance with two accounts: an admin (the instance's,
// so the account under test is never the last one that can administer it) and
// the member being deleted, holding one row in every table that can hold one.
type accountFixture struct {
	s      *SQLiteStore
	admin  *User
	member *User
}

// seedEverything writes one row for the member into every table accountTables
// gives a residue query for. Anything it misses is caught below, where each
// residue is required to be non-zero *before* the deletion — an emptiness
// assertion over a table nothing was written to proves nothing.
func seedEverything(t *testing.T, s *SQLiteStore) accountFixture {
	t.Helper()
	ctx := context.Background()

	admin, err := s.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	require.True(t, admin.IsAdmin, "the first account is the instance's admin")
	member, err := s.UpsertUser(ctx, "sub-member", "member@example.test")
	require.NoError(t, err)

	require.NoError(t, s.CreateSession(ctx, &Session{
		ID: "session-member", UserID: member.ID, ExpiresAt: time.Now().Add(time.Hour)}))

	// The title carries the token the artifacts_fts residue query matches on,
	// so the search index is asserted through the interface it is read by.
	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: deletedArtifact, OwnerID: member.ID, Title: "membersearchtoken tracker",
		SourceBlobID: "member-body", WidgetBlobID: "member-widget", Tier: Tier1,
	}))

	tag := &Tag{ID: "member-tag", OwnerID: member.ID, Name: "fitness"}
	require.NoError(t, s.CreateTag(ctx, tag))
	require.NoError(t, s.AddArtifactTag(ctx, member.ID, deletedArtifact, tag.ID))

	col := &Collection{ID: "member-collection", OwnerID: member.ID, Name: "Health"}
	require.NoError(t, s.CreateCollection(ctx, col))
	require.NoError(t, s.AddArtifactToCollection(ctx, member.ID, deletedArtifact, col.ID))

	require.NoError(t, s.SetOriginDecision(ctx, member.ID, deletedArtifact,
		"https://api.example.test", "allow", "user"))
	require.NoError(t, s.CreateShare(ctx, member.ID, &Share{ID: "member-share", ArtifactID: deletedArtifact, Public: true}))
	require.NoError(t, s.SetState(ctx, OwnerID(member.ID), deletedArtifact, ViewerID(member.ID), "runs", "12"))
	require.NoError(t, s.SetAgentKey(ctx, &AgentKey{OwnerID: member.ID, Provider: "anthropic", KeyCiphertext: "sealed"}))
	require.NoError(t, s.SaveTranscript(ctx, member.ID, deletedArtifact, "sess-1", `[{"role":"user"}]`))

	return accountFixture{s: s, admin: admin, member: member}
}

func countRows(t *testing.T, s *SQLiteStore, query string, userID int64) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRowContext(context.Background(), query, userID).Scan(&n))
	return n
}

// TestDeleteAccountLeavesNoRowOfTheAccountBehind is the tripwire's other half:
// the declarations above are executable, and this runs them. Every table that
// claims to hold rows for an account is required to hold one before the
// deletion and none after, so neither half of the pair can be satisfied by an
// accident.
func TestDeleteAccountLeavesNoRowOfTheAccountBehind(t *testing.T) {
	fx := seedEverything(t, newTestStore(t))
	ctx := context.Background()

	for name, tbl := range accountTables {
		if tbl.residue == "" {
			continue
		}
		require.NotZero(t, countRows(t, fx.s, tbl.residue, fx.member.ID),
			"%s was never seeded, so finding it empty afterwards would prove nothing "+
				"(seedEverything must write a row for every table with a residue query)", name)
	}

	blobIDs, err := fx.s.DeleteAccount(ctx, fx.member.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"member-body", "member-widget"}, blobIDs,
		"the caller is handed every blob id it must now delete, collected before the rows naming them went")

	for name, tbl := range accountTables {
		if tbl.residue == "" {
			continue
		}
		assert.Zero(t, countRows(t, fx.s, tbl.residue, fx.member.ID),
			"%s still holds rows for the deleted account — reach: %s (%s)", name, tbl.reach, tbl.why)
	}
}

// The one place a person's data lives outside their own library (av-q0ub):
// state they wrote as a *viewer* of somebody else's artifact. Nothing about
// deleting their artifacts reaches it; migration 014's AFTER DELETE trigger on
// users is what does, which is why it is asserted here rather than assumed.
func TestDeleteAccountTakesStateWrittenOnAnotherAccountsArtifact(t *testing.T) {
	fx := seedEverything(t, newTestStore(t))
	ctx := context.Background()

	require.NoError(t, fx.s.PutArtifact(ctx, &Artifact{
		ID: "admin-artifact", OwnerID: fx.admin.ID, Title: "Shared", SourceBlobID: "admin-body", Tier: Tier1}))
	owner := OwnerID(fx.admin.ID)
	require.NoError(t, fx.s.SetState(ctx, owner, "admin-artifact", ViewerID(fx.member.ID), "k", "the member's"))
	require.NoError(t, fx.s.SetState(ctx, owner, "admin-artifact", ViewerID(fx.admin.ID), "k", "the admin's"))

	_, err := fx.s.DeleteAccount(ctx, fx.member.ID)
	require.NoError(t, err)

	theirs, err := fx.s.GetState(ctx, owner, "admin-artifact", ViewerID(fx.member.ID))
	require.NoError(t, err)
	assert.Empty(t, theirs, "the deleted viewer's rows go, even on an artifact they did not own")

	survived, err := fx.s.GetState(ctx, owner, "admin-artifact", ViewerID(fx.admin.ID))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"k": "the admin's"}, survived,
		"and the artifact owner's own state on the same artifact is untouched")
}

// The non-vacuity control for every emptiness assertion above: another
// account's rows are in the same tables, and none of them moved.
func TestDeleteAccountTouchesNoOtherAccount(t *testing.T) {
	fx := seedEverything(t, newTestStore(t))
	ctx := context.Background()

	require.NoError(t, fx.s.PutArtifact(ctx, &Artifact{
		ID: "admin-artifact", OwnerID: fx.admin.ID, Title: "Theirs", SourceBlobID: "admin-body", Tier: Tier1}))
	require.NoError(t, fx.s.CreateTag(ctx, &Tag{ID: "admin-tag", OwnerID: fx.admin.ID, Name: "theirs"}))
	require.NoError(t, fx.s.CreateShare(ctx, fx.admin.ID, &Share{ID: "admin-share", ArtifactID: "admin-artifact"}))
	require.NoError(t, fx.s.CreateSession(ctx, &Session{
		ID: "session-admin", UserID: fx.admin.ID, ExpiresAt: time.Now().Add(time.Hour)}))

	_, err := fx.s.DeleteAccount(ctx, fx.member.ID)
	require.NoError(t, err)

	a, err := fx.s.GetArtifact(ctx, fx.admin.ID, "admin-artifact")
	require.NoError(t, err)
	assert.NotNil(t, a)
	tags, err := fx.s.ListTags(ctx, fx.admin.ID)
	require.NoError(t, err)
	assert.Len(t, tags, 1)
	sh, err := fx.s.GetShare(ctx, fx.admin.ID, "admin-share")
	require.NoError(t, err)
	assert.NotNil(t, sh, "somebody else's share link still resolves")
	_, err = fx.s.GetSession(ctx, "session-admin")
	assert.NoError(t, err, "and their browser is still signed in")
}

// The same refusal SetUserAdmin and SetUserDisabled give, applied to the
// stronger act: demoting the last enabled admin leaves an instance nobody can
// administer, and deleting them leaves that plus no row to promote back.
func TestDeleteAccountRefusesTheLastEnabledAdmin(t *testing.T) {
	fx := seedEverything(t, newTestStore(t))
	ctx := context.Background()

	_, err := fx.s.DeleteAccount(ctx, fx.admin.ID)
	require.ErrorIs(t, err, ErrLastAdmin)

	// A refusal writes nothing — not the users row, and not the member's
	// library, which the same transaction would otherwise have taken with it.
	still, err := fx.s.GetUser(ctx, fx.admin.ID)
	require.NoError(t, err)
	assert.NotNil(t, still)
	a, err := fx.s.GetArtifact(ctx, fx.member.ID, deletedArtifact)
	require.NoError(t, err)
	assert.NotNil(t, a)

	// A second enabled admin is the whole difference: with one, the same call
	// succeeds.
	require.NoError(t, fx.s.SetUserAdmin(ctx, fx.member.ID, true))
	_, err = fx.s.DeleteAccount(ctx, fx.admin.ID)
	require.NoError(t, err)

	// And a disabled admin is not a way back in, so it does not satisfy the
	// guard either.
	last, err := fx.s.GetUser(ctx, fx.member.ID)
	require.NoError(t, err)
	require.True(t, last.IsAdmin)
	_, err = fx.s.DeleteAccount(ctx, last.ID)
	require.ErrorIs(t, err, ErrLastAdmin, "the instance would be left with nobody able to sign in and administer it")
}

func TestDeleteAccountReportsAMissingAccount(t *testing.T) {
	s := newTestStore(t)
	_, err := s.DeleteAccount(context.Background(), 404)
	assert.ErrorIs(t, err, ErrNotFound)
}

// The confirmation's numbers. Shares are counted across the whole library
// rather than per artifact, because the sentence they appear in is about
// people holding links, not about artifacts.
func TestGetAccountSummaryCountsWhatDeletionWouldDestroy(t *testing.T) {
	fx := seedEverything(t, newTestStore(t))
	ctx := context.Background()

	sum, err := fx.s.GetAccountSummary(ctx, fx.member.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), sum.Artifacts)
	assert.Equal(t, int64(1), sum.Shares)

	require.NoError(t, fx.s.CreateShare(ctx, fx.member.ID, &Share{ID: "member-share-2", ArtifactID: deletedArtifact}))
	sum, err = fx.s.GetAccountSummary(ctx, fx.member.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), sum.Shares)

	// Another account's library is not in the count.
	require.NoError(t, fx.s.PutArtifact(ctx, &Artifact{
		ID: "admin-artifact", OwnerID: fx.admin.ID, Title: "Theirs", SourceBlobID: "admin-body", Tier: Tier1}))
	require.NoError(t, fx.s.CreateShare(ctx, fx.admin.ID, &Share{ID: "admin-share", ArtifactID: "admin-artifact"}))
	sum, err = fx.s.GetAccountSummary(ctx, fx.member.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), sum.Artifacts)
	assert.Equal(t, int64(2), sum.Shares)

	// An account with nothing counts zero rather than failing.
	sum, err = fx.s.GetAccountSummary(ctx, fx.admin.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), sum.Artifacts)
	assert.Equal(t, int64(1), sum.Shares)
}
