package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

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
        INSERT INTO users(id, external_id) VALUES (1, 'owner'), (2, 'recipient'), (9, 'nine'), (12, 'twelve'), (3, 'three');
        INSERT INTO artifacts(id, owner_id, source_blob_id) VALUES ('a', 1, 'body'), ('b', 1, 'body-b');
        INSERT INTO shares(id, artifact_id, recipient_id) VALUES
            ('nine-grant', 'a', 9), ('grant-only-id', 'b', 2),
            ('twelve-grant', 'a', 12), ('existing-url', 'a', NULL),
            ('three-grant', 'a', 3), ('old-grant-id', 'a', 2);`)
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
		if id == "a" {
			assert.Equal(t, 4, a.ShareGrantCount)
		} else {
			assert.Equal(t, 1, a.ShareGrantCount)
		}
		assert.Equal(t, id == "a", a.SharePublicLink)
	}
	listed, err := s.ListArtifactShares(ctx, 1, "a")
	require.NoError(t, err)
	require.Len(t, listed, 5)
	assert.Equal(t, "existing-url", listed[0].ID, "the link comes first even when created between grants")
	for i, recipient := range []int64{9, 12, 3, 2} {
		require.NotNil(t, listed[i+1].RecipientID)
		assert.Equal(t, recipient, *listed[i+1].RecipientID, "legacy rowid order survives upgrade")
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
	assert.Equal(t, 4, grants)
	assert.True(t, publicLink)
	var restoredOrder string
	require.NoError(t, s.db.QueryRow(`SELECT group_concat(recipient_id) FROM (
        SELECT recipient_id FROM shares WHERE artifact_id='a' AND recipient_id IS NOT NULL ORDER BY rowid
    )`).Scan(&restoredOrder))
	assert.Equal(t, "9,12,3,2", restoredOrder, "Down preserves relative grant order for the old application")
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

// Separate additions retain creation order, independent of the composite PK.
// A submission completed within one millisecond has the documented id tie-break.
func TestGrantCreationOrderSurvivesLinkRotationAndDuplication(t *testing.T) {
	f := newGrantFixture(t)
	ctx := context.Background()
	_, err := f.s.db.Exec(`INSERT INTO users(id, external_id) VALUES (9, 'nine'), (12, 'twelve'), (3, 'three')`)
	require.NoError(t, err)
	for _, recipient := range []int64{9, 12, 3} {
		_, err := f.s.CreateGrant(ctx, alice, f.artifactID, recipient)
		require.NoError(t, err)
		// Model separate additions, keeping their millisecond timestamps distinct.
		time.Sleep(2 * time.Millisecond)
	}
	_, err = f.s.CreateAnonymousLink(ctx, alice, f.artifactID)
	require.NoError(t, err)
	_, err = f.s.ReplaceAnonymousLink(ctx, alice, f.artifactID, "rotated")
	require.NoError(t, err)
	created, err := f.s.DuplicateArtifact(ctx, alice, f.artifactID, ArtifactDuplicate{
		ID: "copy", Title: "Copy", BodyBlobID: "copy-body", SourceVersion: 1, CopyShareSettings: true,
	})
	require.NoError(t, err)
	require.True(t, created)
	for _, artifactID := range []string{f.artifactID, "copy"} {
		listed, err := f.s.ListArtifactShares(ctx, alice, artifactID)
		require.NoError(t, err)
		require.Len(t, listed, 4)
		assert.NotEmpty(t, listed[0].ID)
		assert.Nil(t, listed[0].RecipientID)
		for i, recipient := range []int64{9, 12, 3} {
			assert.Empty(t, listed[i+1].ID)
			require.NotNil(t, listed[i+1].Recipient)
			assert.Equal(t, recipient, listed[i+1].Recipient.ID)
		}
	}
	var copiedTimestamps int
	require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM share_grants original
        JOIN share_grants copy ON copy.recipient_id=original.recipient_id AND copy.created_at=original.created_at
        WHERE original.artifact_id=? AND copy.artifact_id='copy'`, f.artifactID).Scan(&copiedTimestamps))
	assert.Equal(t, 3, copiedTimestamps)

	_, err = f.s.db.Exec(`UPDATE share_grants SET created_at='2026-01-01T00:00:00.123Z' WHERE artifact_id=?`, f.artifactID)
	require.NoError(t, err)
	listed, err := f.s.ListArtifactShares(ctx, alice, f.artifactID)
	require.NoError(t, err)
	require.Len(t, listed, 4)
	assert.Equal(t, "rotated", listed[0].ID)
	for i, recipient := range []int64{3, 9, 12} {
		assert.Equal(t, recipient, *listed[i+1].RecipientID, "millisecond ties use recipient id")
	}
}
