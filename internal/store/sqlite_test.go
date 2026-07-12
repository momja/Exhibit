package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	f, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	s, err := OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGetArtifact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := &Artifact{
		ID:               "test-id-1",
		OwnerID:          1,
		Title:            "Test Artifact",
		SourceBlobID:     "blob-1",
		Tier:             Tier1,
		NetworkAllowlist: []string{"https://cdn.example.com"},
	}

	err := s.PutArtifact(ctx, a)
	require.NoError(t, err)

	got, err := s.GetArtifact(ctx, "test-id-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, a.ID, got.ID)
	assert.Equal(t, a.Title, got.Title)
	assert.Equal(t, a.NetworkAllowlist, got.NetworkAllowlist)
}

func TestListArtifacts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i, title := range []string{"foo chart", "bar graph", "foo table"} {
		err := s.PutArtifact(ctx, &Artifact{
			ID:           fmt.Sprintf("id-%d", i),
			OwnerID:      1,
			Title:        title,
			SourceBlobID: fmt.Sprintf("blob-%d", i),
			Tier:         Tier1,
		})
		require.NoError(t, err)
	}

	all, err := s.ListArtifacts(ctx, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.PutArtifact(ctx, &Artifact{
		ID: "state-test", OwnerID: 1, Title: "T", SourceBlobID: "b", Tier: Tier1,
	})
	require.NoError(t, err)

	state, err := s.GetState(ctx, "state-test")
	require.NoError(t, err)
	assert.Empty(t, state)

	err = s.SetState(ctx, "state-test", "key1", "value1")
	require.NoError(t, err)

	state, err = s.GetState(ctx, "state-test")
	require.NoError(t, err)
	assert.Equal(t, "value1", state["key1"])

	// Upsert
	err = s.SetState(ctx, "state-test", "key1", "updated")
	require.NoError(t, err)
	state, _ = s.GetState(ctx, "state-test")
	assert.Equal(t, "updated", state["key1"])
}

func TestCollectionsAndTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.PutArtifact(ctx, &Artifact{
		ID: "a1", OwnerID: 1, Title: "Artifact 1", SourceBlobID: "b1", Tier: Tier1,
	})
	require.NoError(t, err)

	col := &Collection{ID: "col1", OwnerID: 1, Name: "My Collection"}
	err = s.CreateCollection(ctx, col)
	require.NoError(t, err)

	err = s.AddArtifactToCollection(ctx, "a1", "col1")
	require.NoError(t, err)

	tag := &Tag{ID: "tag1", OwnerID: 1, Name: "charts"}
	err = s.CreateTag(ctx, tag)
	require.NoError(t, err)

	err = s.AddArtifactTag(ctx, 1, "a1", "tag1")
	require.NoError(t, err)

	cols, err := s.ListCollections(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, cols, 1)

	tags, err := s.ListTags(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, tags, 1)
}

func TestCreateTagColor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Explicit color is persisted verbatim.
	err := s.CreateTag(ctx, &Tag{ID: "tag-red", OwnerID: 1, Name: "urgent", Color: "#FF0000"})
	require.NoError(t, err)

	// Omitted color falls back to the default.
	defaulted := &Tag{ID: "tag-default", OwnerID: 1, Name: "misc"}
	err = s.CreateTag(ctx, defaulted)
	require.NoError(t, err)
	assert.Equal(t, DefaultTagColor, defaulted.Color, "CreateTag should backfill the default color on the passed tag")

	tags, err := s.ListTags(ctx, 1)
	require.NoError(t, err)
	require.Len(t, tags, 2)

	colors := map[string]string{}
	for _, tag := range tags {
		colors[tag.ID] = tag.Color
	}
	assert.Equal(t, "#FF0000", colors["tag-red"])
	assert.Equal(t, DefaultTagColor, colors["tag-default"])
}

func strPtr(s string) *string { return &s }

func TestCreateTagDuplicateName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t1", OwnerID: 1, Name: "charts"}))

	err := s.CreateTag(ctx, &Tag{ID: "t2", OwnerID: 1, Name: "charts"})
	assert.ErrorIs(t, err, ErrDuplicateName)

	// Same name under a different owner is fine.
	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t3", OwnerID: 2, Name: "charts"}))

	tags, err := s.ListTags(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, tags, 1, "duplicate must not create a second row")
}

func TestUpdateTag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t1", OwnerID: 1, Name: "charts", Color: "#FF0000"}))
	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t2", OwnerID: 1, Name: "maps"}))

	// Rename + recolor together.
	got, err := s.UpdateTag(ctx, 1, "t1", strPtr("graphs"), strPtr("#00FF00"))
	require.NoError(t, err)
	assert.Equal(t, "graphs", got.Name)
	assert.Equal(t, "#00FF00", got.Color)

	// Partial update: only color; name untouched.
	got, err = s.UpdateTag(ctx, 1, "t1", nil, strPtr("#0000FF"))
	require.NoError(t, err)
	assert.Equal(t, "graphs", got.Name)
	assert.Equal(t, "#0000FF", got.Color)

	// Renaming onto an existing name collides.
	_, err = s.UpdateTag(ctx, 1, "t1", strPtr("maps"), nil)
	assert.ErrorIs(t, err, ErrDuplicateName)

	// Renaming to its own current name is a no-op, not a collision.
	_, err = s.UpdateTag(ctx, 1, "t1", strPtr("graphs"), nil)
	assert.NoError(t, err)

	// Unknown id and foreign owner both read as not found.
	_, err = s.UpdateTag(ctx, 1, "missing", strPtr("x"), nil)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.UpdateTag(ctx, 2, "t1", strPtr("x"), nil)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteTagCascades(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "a1", OwnerID: 1, Title: "A", SourceBlobID: "b1", Tier: Tier1}))
	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t1", OwnerID: 1, Name: "charts"}))
	require.NoError(t, s.AddArtifactTag(ctx, 1, "a1", "t1"))

	// Foreign owner cannot delete it.
	assert.ErrorIs(t, s.DeleteTag(ctx, 2, "t1"), ErrNotFound)

	require.NoError(t, s.DeleteTag(ctx, 1, "t1"))

	tags, err := s.ListTags(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, tags)

	// The artifact association is gone too.
	a, err := s.GetArtifact(ctx, "a1")
	require.NoError(t, err)
	assert.Empty(t, a.Tags)

	assert.ErrorIs(t, s.DeleteTag(ctx, 1, "t1"), ErrNotFound)
}

func TestArtifactTagValidation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "a1", OwnerID: 1, Title: "A", SourceBlobID: "b1", Tier: Tier1}))
	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t1", OwnerID: 1, Name: "charts"}))
	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t-other", OwnerID: 2, Name: "foreign"}))

	// Attach: nonexistent artifact, nonexistent tag, or another owner's tag all fail.
	assert.ErrorIs(t, s.AddArtifactTag(ctx, 1, "missing", "t1"), ErrNotFound)
	assert.ErrorIs(t, s.AddArtifactTag(ctx, 1, "a1", "missing"), ErrNotFound)
	assert.ErrorIs(t, s.AddArtifactTag(ctx, 1, "a1", "t-other"), ErrNotFound)

	// Detach before any attach: no pairing.
	assert.ErrorIs(t, s.RemoveArtifactTag(ctx, 1, "a1", "t1"), ErrNotFound)

	require.NoError(t, s.AddArtifactTag(ctx, 1, "a1", "t1"))
	// Attaching again is idempotent.
	require.NoError(t, s.AddArtifactTag(ctx, 1, "a1", "t1"))

	// Foreign owner cannot detach.
	assert.ErrorIs(t, s.RemoveArtifactTag(ctx, 2, "a1", "t1"), ErrNotFound)

	require.NoError(t, s.RemoveArtifactTag(ctx, 1, "a1", "t1"))
	assert.ErrorIs(t, s.RemoveArtifactTag(ctx, 1, "a1", "t1"), ErrNotFound)
}

func TestArtifactTagsHydrated(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, id := range []string{"a1", "a2", "a3"} {
		require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: id, OwnerID: 1, Title: id, SourceBlobID: "b-" + id, Tier: Tier1}))
	}
	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t1", OwnerID: 1, Name: "charts", Color: "#FF0000"}))
	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "t2", OwnerID: 1, Name: "maps", Color: "#00FF00"}))
	require.NoError(t, s.AddArtifactTag(ctx, 1, "a1", "t1"))
	require.NoError(t, s.AddArtifactTag(ctx, 1, "a1", "t2"))
	require.NoError(t, s.AddArtifactTag(ctx, 1, "a2", "t2"))

	arts, err := s.ListArtifacts(ctx, ListOptions{})
	require.NoError(t, err)
	require.Len(t, arts, 3)

	tagNames := map[string][]string{}
	for _, a := range arts {
		require.NotNil(t, a.Tags, "Tags must be hydrated, not nil")
		names := []string{}
		for _, tag := range a.Tags {
			assert.NotEmpty(t, tag.ID)
			assert.NotEmpty(t, tag.Color)
			names = append(names, tag.Name)
		}
		tagNames[a.ID] = names
	}
	assert.Equal(t, []string{"charts", "maps"}, tagNames["a1"])
	assert.Equal(t, []string{"maps"}, tagNames["a2"])
	assert.Empty(t, tagNames["a3"])

	// GetArtifact hydrates too.
	a, err := s.GetArtifact(ctx, "a1")
	require.NoError(t, err)
	require.Len(t, a.Tags, 2)
	assert.Equal(t, "charts", a.Tags[0].Name)
	assert.Equal(t, "#FF0000", a.Tags[0].Color)
}

// TestMigration004DedupesTags applies migrations only up to 003, seeds
// duplicate tag names (legal before the unique index), then applies 004 and
// verifies duplicates are folded into the first-created tag with attachments
// re-pointed.
func TestMigration004DedupesTags(t *testing.T) {
	f, err := os.CreateTemp("", "test-mig-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite", f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`)
	require.NoError(t, err)

	goose.SetBaseFS(migrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(db, "migrations", 3))

	mustExec := func(q string) {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
	mustExec(`INSERT INTO artifacts (id, source_blob_id) VALUES ('a1','b1'), ('a2','b2')`)
	mustExec(`INSERT INTO tags (id, owner_id, name) VALUES ('t1',1,'charts'), ('t2',1,'charts'), ('t3',2,'charts')`)
	mustExec(`INSERT INTO artifact_tags (artifact_id, tag_id) VALUES ('a1','t1'), ('a1','t2'), ('a2','t2')`)

	require.NoError(t, goose.UpTo(db, "migrations", 4))

	// Owner 1 keeps a single 'charts' (t1); owner 2's identically named tag survives.
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM tags`).Scan(&n))
	assert.Equal(t, 2, n)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM tags WHERE id='t1'`).Scan(&n))
	assert.Equal(t, 1, n)

	// Both artifacts now point at the surviving tag, with no leftovers.
	rows, err := db.Query(`SELECT artifact_id, tag_id FROM artifact_tags ORDER BY artifact_id`)
	require.NoError(t, err)
	defer rows.Close()
	pairs := map[string]string{}
	for rows.Next() {
		var art, tag string
		require.NoError(t, rows.Scan(&art, &tag))
		pairs[art] = tag
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, map[string]string{"a1": "t1", "a2": "t1"}, pairs)

	// The unique index is enforced from here on.
	_, err = db.Exec(`INSERT INTO tags (id, owner_id, name) VALUES ('t4',1,'charts')`)
	assert.True(t, isUniqueViolation(err))
}

// downloads_approved is the download bridge's first-use approval flag
// (av-ryby): it must default to false, round-trip through Put/Get, and flip
// via UpdateArtifact — that persistence is what makes the approval survive
// reloads and devices.
func TestDownloadsApproved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "dl-1", OwnerID: 1, SourceBlobID: "b1"}))
	got, err := s.GetArtifact(ctx, "dl-1")
	require.NoError(t, err)
	assert.False(t, got.DownloadsApproved, "new artifacts must not be pre-approved for downloads")

	require.NoError(t, s.UpdateArtifact(ctx, "dl-1", map[string]any{"downloads_approved": true}))
	got, err = s.GetArtifact(ctx, "dl-1")
	require.NoError(t, err)
	assert.True(t, got.DownloadsApproved)

	// Revoke.
	require.NoError(t, s.UpdateArtifact(ctx, "dl-1", map[string]any{"downloads_approved": false}))
	got, err = s.GetArtifact(ctx, "dl-1")
	require.NoError(t, err)
	assert.False(t, got.DownloadsApproved)

	// A non-bool would be stored as an unscannable value and brick reads of
	// the artifact, so the store rejects it outright.
	err = s.UpdateArtifact(ctx, "dl-1", map[string]any{"downloads_approved": "yes"})
	assert.Error(t, err)
}

// clipboard_approved is the sibling capability-bridge approval (av-hll6): same
// default-false, round-trip, flip, and non-bool rejection as downloads_approved,
// and the two are independent columns.
func TestClipboardApproved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "cl-1", OwnerID: 1, SourceBlobID: "b1"}))
	got, err := s.GetArtifact(ctx, "cl-1")
	require.NoError(t, err)
	assert.False(t, got.ClipboardApproved, "new artifacts must not be pre-approved for clipboard")

	require.NoError(t, s.UpdateArtifact(ctx, "cl-1", map[string]any{"clipboard_approved": true}))
	got, err = s.GetArtifact(ctx, "cl-1")
	require.NoError(t, err)
	assert.True(t, got.ClipboardApproved)
	assert.False(t, got.DownloadsApproved, "clipboard approval must not leak into downloads")

	require.NoError(t, s.UpdateArtifact(ctx, "cl-1", map[string]any{"clipboard_approved": false}))
	got, err = s.GetArtifact(ctx, "cl-1")
	require.NoError(t, err)
	assert.False(t, got.ClipboardApproved)

	err = s.UpdateArtifact(ctx, "cl-1", map[string]any{"clipboard_approved": "yes"})
	assert.Error(t, err)
}
