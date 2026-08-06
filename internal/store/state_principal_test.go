package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-q0ub: artifact_state gained a principal. These tests fix the three things
// that follow from that — the migration keeps every existing row (and files it
// under the right viewer), two viewers on one artifact are separate stores, and
// a viewer's rows die with the viewer.

// stateRow is one row read straight out of the table, bypassing the Store so
// the migration's own output is what is being asserted.
type stateRow struct {
	artifactID string
	userID     int64
	key, value string
}

func readStateRows(t *testing.T, db *sql.DB) []stateRow {
	t.Helper()
	rows, err := db.Query(
		`SELECT artifact_id, user_id, key, value FROM artifact_state
		  ORDER BY artifact_id, user_id, key`)
	require.NoError(t, err)
	defer rows.Close()
	var out []stateRow
	for rows.Next() {
		var r stateRow
		require.NoError(t, rows.Scan(&r.artifactID, &r.userID, &r.key, &r.value))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// AC#1. The migration is a table rebuild, which is the shape of change that
// loses data when it goes wrong — so this runs it against a database that
// already has state in it, seeded through the *pre-migration* schema, rather
// than against a fresh one where there is nothing to lose.
//
// The artifacts belong to different owners on purpose. A backfill that hardcodes
// owner 1 (the single-user default, and the value every row in a real upgraded
// database happens to have) passes a one-owner test and silently misfiles
// everything else.
func TestMigration014BackfillsStateToTheArtifactOwner(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test-mig-state-*.db")
	require.NoError(t, err)
	f.Close()

	db, err := sql.Open("sqlite", f.Name())
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`)
	require.NoError(t, err)

	// Stop at 013 — the last schema before state had a principal. The repairs
	// are registered explicitly so the walk to 013 does not depend on whether
	// some earlier test in this process happened to call OpenSQLite first.
	registerRepairMigrations()
	goose.SetBaseFS(migrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(db, "migrations", 13))

	mustExec := func(q string, args ...any) {
		_, err := db.Exec(q, args...)
		require.NoError(t, err)
	}
	mustExec(`INSERT INTO artifacts (id, owner_id, source_blob_id) VALUES
		('mine', 1, 'b1'), ('theirs', 7, 'b2')`)
	// The four-column shape: no user_id exists yet to write.
	mustExec(`INSERT INTO artifact_state (artifact_id, key, value) VALUES
		('mine',   'todo',   '["milk"]'),
		('mine',   '',       'the empty key is legal Web Storage'),
		('theirs', 'todo',   '["theirs"]'),
		('theirs', 'config', '{"dark":true}')`)
	require.NoError(t, db.Close())

	// The production upgrade path: reopening the service runs the rest.
	st, err := OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })

	db, err = sql.Open("sqlite", f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	assert.Equal(t, []stateRow{
		{"mine", 1, "", "the empty key is legal Web Storage"},
		{"mine", 1, "todo", `["milk"]`},
		{"theirs", 7, "config", `{"dark":true}`},
		{"theirs", 7, "todo", `["theirs"]`},
	}, readStateRows(t, db),
		"every pre-migration row must survive, filed under its artifact's owner")

	// And the rows are still reachable the way the application reads them,
	// which is the thing a user would notice: state that migrated into a table
	// nobody queries is state that is gone.
	ctx := context.Background()
	mine, err := st.GetState(ctx, 1, "mine", 1)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"todo": `["milk"]`,
		"":     "the empty key is legal Web Storage",
	}, mine)

	theirs, err := st.GetState(ctx, 7, "theirs", 7)
	require.NoError(t, err)
	assert.Len(t, theirs, 2)

	// The new primary key admits the same key twice under different viewers,
	// and still rejects it twice under one.
	require.NoError(t, st.SetState(ctx, 1, "mine", 9, "todo", `["not mine"]`))
	require.NoError(t, st.SetState(ctx, 1, "mine", 9, "todo", `["overwritten"]`))
	rows := readStateRows(t, db)
	assert.Len(t, rows, 5, "a second viewer adds a row; a repeat write updates one")
}

// AC#2, at the store. Two viewers' rows on one artifact are two stores: same
// keys, no collision, and neither read reaches the other.
func TestStateOfTwoViewersOnOneArtifactStaysSeparate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const owner, guest int64 = 1, 2

	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: "shared", OwnerID: owner, SourceBlobID: "b1", Tier: Tier1}))

	// Both write the same key. Authorization is the owner's either way — the
	// artifact is the owner's — but selection differs, which is the whole point
	// of the two parameters.
	require.NoError(t, s.SetState(ctx, owner, "shared", owner, "draft", "the owner's"))
	require.NoError(t, s.SetState(ctx, owner, "shared", guest, "draft", "the guest's"))

	ownerState, err := s.GetState(ctx, owner, "shared", owner)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"draft": "the owner's"}, ownerState,
		"a read must return one viewer's rows, never the union")

	guestState, err := s.GetState(ctx, owner, "shared", guest)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"draft": "the guest's"}, guestState)

	// A delete is scoped the same way: it removes the caller's row and leaves
	// the other viewer's identically-keyed row alone.
	require.NoError(t, s.DeleteState(ctx, owner, "shared", owner, "draft"))
	guestState, err = s.GetState(ctx, owner, "shared", guest)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"draft": "the guest's"}, guestState,
		"deleting one viewer's key must not delete another's")

	// So is erase-all. "Erase my state" is the operation the state inspector
	// offers; erasing someone else's is a different act no route grants.
	require.NoError(t, s.SetState(ctx, owner, "shared", owner, "draft", "again"))
	require.NoError(t, s.ClearState(ctx, owner, "shared", owner))
	guestState, err = s.GetState(ctx, owner, "shared", guest)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"draft": "the guest's"}, guestState,
		"ClearState must erase the caller's rows, not the artifact's")
}

// AC#5, at the store: the property the whole state design exists for. "Two
// devices" is two independent calls by the same principal — there is nothing
// device-shaped in the schema, and that is exactly what must stay true. The
// principal split must not have quietly turned one shared row into two.
func TestOneUserOnTwoDevicesSharesOneSetOfRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const me int64 = 1

	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: "tracker", OwnerID: me, SourceBlobID: "b1", Tier: Tier1}))

	// iPhone writes.
	require.NoError(t, s.SetState(ctx, me, "tracker", me, "runs", `[{"km":5}]`))

	// Mac reads it back — same principal, separate call, no device identifier
	// anywhere in the signature.
	fromMac, err := s.GetState(ctx, me, "tracker", me)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"runs": `[{"km":5}]`}, fromMac,
		"a second device must read the first device's write")

	// Mac writes back; iPhone sees the update, not a second row.
	require.NoError(t, s.SetState(ctx, me, "tracker", me, "runs", `[{"km":5},{"km":8}]`))
	fromPhone, err := s.GetState(ctx, me, "tracker", me)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"runs": `[{"km":5},{"km":8}]`}, fromPhone,
		"last write wins over one row, not per-device forks")

	var n int
	require.NoError(t, s.db.QueryRow(
		`SELECT COUNT(*) FROM artifact_state WHERE artifact_id='tracker'`).Scan(&n))
	assert.Equal(t, 1, n, "one user, one key, one row — however many devices wrote it")
}

// AC#4. State already dies with its artifact; a user-scoped table needs the
// second cascade too. Deleting the user must take their rows *everywhere*,
// including from an artifact somebody else owns — which is precisely the case
// the artifact cascade cannot reach.
func TestDeletingAUserRemovesTheirStateRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	host, err := s.UpsertUser(ctx, "host@idp", "host@example.com")
	require.NoError(t, err)
	guest, err := s.UpsertUser(ctx, "guest@idp", "guest@example.com")
	require.NoError(t, err)
	require.NotEqual(t, host.ID, guest.ID)

	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: "hosted", OwnerID: host.ID, SourceBlobID: "b1", Tier: Tier1}))
	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: "guests-own", OwnerID: guest.ID, SourceBlobID: "b2", Tier: Tier1}))

	require.NoError(t, s.SetState(ctx, host.ID, "hosted", host.ID, "k", "host's own"))
	require.NoError(t, s.SetState(ctx, host.ID, "hosted", guest.ID, "k", "guest, visiting"))
	require.NoError(t, s.SetState(ctx, guest.ID, "guests-own", guest.ID, "k", "guest at home"))

	// There is no DeleteUser on the Store yet; account deletion will add one.
	// The cascade is a property of the schema, so it is asserted against the
	// row removal itself rather than against a method that does not exist.
	_, err = s.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, guest.ID)
	require.NoError(t, err)

	var remaining int
	require.NoError(t, s.db.QueryRow(
		`SELECT COUNT(*) FROM artifact_state WHERE user_id=?`, guest.ID).Scan(&remaining))
	assert.Zero(t, remaining,
		"deleting a user must drop their state rows, including those on another owner's artifact")

	survived, err := s.GetState(ctx, host.ID, "hosted", host.ID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"k": "host's own"}, survived,
		"no other viewer's state may be collateral damage")

	// And the artifact the departed user owned is untouched by *this* cascade —
	// artifact lifetime is a separate question from viewer lifetime.
	a, err := s.GetArtifactUnscoped(ctx, "guests-own")
	require.NoError(t, err)
	assert.NotNil(t, a, "deleting a user erases their state, not their library")
}
