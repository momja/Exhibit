package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exh-grj9: upgrade an actual previous-release database, then reopen it with
// the application's startup path. Existing capability URLs must retain ids.
func TestShareSplitUpgradeAndDown(t *testing.T) {
	path := newMigratedTo(t, 37)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`PRAGMA foreign_keys=ON;
        INSERT INTO users(id, external_id) VALUES (1, 'owner'), (2, 'recipient');
        INSERT INTO artifacts(id, owner_id, source_blob_id) VALUES ('a', 1, 'body'), ('b', 1, 'body-b');
        INSERT INTO shares(id, artifact_id, recipient_id) VALUES
            ('existing-url', 'a', NULL), ('old-grant-id', 'a', 2), ('grant-only-id', 'b', 2);`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := OpenSQLite(path)
	require.NoError(t, err, "the previous release must still start")
	defer s.Close()
	ctx := context.Background()
	link, err := s.GetAnonymousShareUnscoped(ctx, "existing-url")
	require.NoError(t, err)
	require.NotNil(t, link)
	assert.Equal(t, "a", link.ArtifactID)
	for _, id := range []string{"old-grant-id", "grant-only-id"} {
		sh, err := s.GetAnonymousShareUnscoped(ctx, id)
		require.NoError(t, err)
		assert.Nil(t, sh, "a legacy grant id must never become a public URL")
	}
	for _, id := range []string{"a", "b"} {
		a, err := s.GetArtifactReadableBy(ctx, ViewerID(2), id)
		require.NoError(t, err)
		require.NotNil(t, a)
		assert.Equal(t, 1, a.ShareGrantCount)
		assert.Equal(t, id == "a", a.SharePublicLink)
	}
	var oldObjects int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
        WHERE name IN ('shares', 'shares_artifact_recipient', 'shares_one_anonymous_link',
            'shares_counts_sync_insert', 'shares_counts_sync_delete', 'shares_counts_sync_update')`).Scan(&oldObjects))
	assert.Zero(t, oldObjects)
	assert.False(t, artifactHasColumn(t, s.db, "share_link"))
	assert.False(t, artifactHasColumn(t, s.db, "share_grant_count"))

	require.NoError(t, goose.DownTo(s.db, "migrations", 37))
	var id string
	require.NoError(t, s.db.QueryRow(`SELECT id FROM shares WHERE artifact_id='a' AND recipient_id IS NULL`).Scan(&id))
	assert.Equal(t, "existing-url", id)
	require.NoError(t, s.db.QueryRow(`SELECT id FROM shares WHERE artifact_id='a' AND recipient_id=2`).Scan(&id))
	assert.NotEmpty(t, id)
	assert.NotEqual(t, "old-grant-id", id, "Down regenerates discarded grant ids")
	var grants int
	var publicLink bool
	require.NoError(t, s.db.QueryRow(`SELECT share_grant_count, share_link FROM artifacts WHERE id='a'`).Scan(&grants, &publicLink))
	assert.Equal(t, 1, grants)
	assert.True(t, publicLink)
	// Restored 029 triggers must still track writes for the old application.
	_, err = s.db.Exec(`DELETE FROM shares WHERE artifact_id='a'`)
	require.NoError(t, err)
	require.NoError(t, s.db.QueryRow(`SELECT share_grant_count, share_link FROM artifacts WHERE id='a'`).Scan(&grants, &publicLink))
	assert.Zero(t, grants)
	assert.False(t, publicLink)
	require.NoError(t, goose.Up(s.db, "migrations"))
	readable, err := s.GetArtifactReadableBy(ctx, ViewerID(2), "b")
	require.NoError(t, err)
	assert.NotNil(t, readable, "Down/Up preserves the other grant")
	rows, err := s.db.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	defer rows.Close()
	assert.False(t, rows.Next(), "no FK violations after upgrade, Down and upgrade")
	require.NoError(t, rows.Err())
}

func TestGrantLookupByRecipientUsesCoveringIndex(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	_, err := f.s.CreateGrant(ctx, alice, f.artifactID, bob)
	require.NoError(t, err)
	rows, err := f.s.db.QueryContext(ctx,
		`EXPLAIN QUERY PLAN SELECT artifact_id FROM share_grants WHERE recipient_id=?`, bob)
	require.NoError(t, err)
	defer rows.Close()
	var plans []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plans = append(plans, detail)
		t.Log(detail)
		assert.NotContains(t, detail, "SCAN")
	}
	require.NoError(t, rows.Err())
	require.Len(t, plans, 1)
	assert.Contains(t, plans[0], "SEARCH share_grants USING COVERING INDEX share_grants_by_recipient (recipient_id=?)")
}
