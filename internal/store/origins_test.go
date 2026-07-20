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

// exhibit-x87: migration 009 backfills the legacy network_allowlist JSON
// arrays as allow rows, so an artifact approved before the migration keeps
// exactly the CSP it had.
func TestMigration009BackfillsLegacyAllowlist(t *testing.T) {
	f, err := os.CreateTemp("", "test-migrate-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite", f.Name())
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`)
	require.NoError(t, err)

	// Stop just before 009, while network_allowlist is still a column, and
	// write the pre-migration shape directly.
	goose.SetBaseFS(migrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(db, "migrations", 7))
	_, err = db.Exec(
		`INSERT INTO artifacts (id, owner_id, title, source_blob_id, tier, network_allowlist)
		 VALUES ('legacy', 1, 'Legacy', 'blob-legacy', 1, '["https://a.example.com","https://b.example.com"]')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	allowed, err := s.AllowedOrigins(ctx, "legacy")
	require.NoError(t, err)
	assert.Equal(t, []string{"https://a.example.com", "https://b.example.com"}, allowed)

	decisions, err := s.ListOriginDecisions(ctx, "legacy")
	require.NoError(t, err)
	require.Len(t, decisions, 2)
	assert.Equal(t, "legacy", decisions[0].Source, "backfilled rows record where they came from")

	// The column itself is gone — decisions are the only source of truth now.
	got, err := s.GetArtifact(ctx, "legacy")
	require.NoError(t, err)
	assert.Equal(t, []string{"https://a.example.com", "https://b.example.com"}, got.NetworkAllowlist)
}

func putTestArtifact(t *testing.T, s *SQLiteStore, id string, allowlist []string) *Artifact {
	t.Helper()
	a := &Artifact{ID: id, OwnerID: 1, Title: id, SourceBlobID: "blob-" + id, Tier: Tier1,
		NetworkAllowlist: allowlist}
	require.NoError(t, s.PutArtifact(context.Background(), a))
	return a
}

// exhibit-x87: allow rows are the CSP's source of truth; block rows are
// "don't ask again" markers and must never widen it.
func TestAllowedOriginsIgnoresBlockDecisions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putTestArtifact(t, s, "a1", []string{"https://ok.example.com"})
	require.NoError(t, s.SetOriginDecision(ctx, "a1", "https://nope.example.com", DecisionBlock, "runtime"))

	allowed, err := s.AllowedOrigins(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, []string{"https://ok.example.com"}, allowed)

	// The hydrated artifact — what render.buildCSP reads — matches.
	got, err := s.GetArtifact(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, []string{"https://ok.example.com"}, got.NetworkAllowlist)

	decisions, err := s.ListOriginDecisions(ctx, "a1")
	require.NoError(t, err)
	assert.Len(t, decisions, 2, "both decisions are stored; only allow reaches the CSP")
}

// exhibit-x87: the (artifact, origin) primary key means one decision per
// origin — a second decision flips the first rather than duplicating it.
func TestSetOriginDecisionUpsertsAndDeletes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putTestArtifact(t, s, "a1", nil)

	require.NoError(t, s.SetOriginDecision(ctx, "a1", "https://x.example.com", DecisionAllow, "user"))
	require.NoError(t, s.SetOriginDecision(ctx, "a1", "https://x.example.com", DecisionBlock, "runtime"))

	decisions, err := s.ListOriginDecisions(ctx, "a1")
	require.NoError(t, err)
	require.Len(t, decisions, 1, "an origin can hold only one decision")
	assert.Equal(t, DecisionBlock, decisions[0].Decision)
	assert.Equal(t, "runtime", decisions[0].Source)

	allowed, err := s.AllowedOrigins(ctx, "a1")
	require.NoError(t, err)
	assert.Empty(t, allowed, "flipping allow→block must revoke the CSP grant")

	require.NoError(t, s.DeleteOriginDecision(ctx, "a1", "https://x.example.com"))
	decisions, err = s.ListOriginDecisions(ctx, "a1")
	require.NoError(t, err)
	assert.Empty(t, decisions)

	assert.Error(t, s.SetOriginDecision(ctx, "a1", "https://x.example.com", "maybe", "user"),
		"only allow/block are valid decisions")
}

// exhibit-x87: an allowlist-shaped write (the edit page's single PATCH, which
// carries only the allow set) must never clear block decisions it doesn't
// know about.
func TestReplaceAllowedOriginsPreservesBlockRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putTestArtifact(t, s, "a1", []string{"https://old.example.com"})
	require.NoError(t, s.SetOriginDecision(ctx, "a1", "https://blocked.example.com", DecisionBlock, "runtime"))

	require.NoError(t, s.ReplaceAllowedOrigins(ctx, "a1", []string{"https://new.example.com"}, "user"))

	allowed, err := s.AllowedOrigins(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, []string{"https://new.example.com"}, allowed, "replaced allow set")

	decisions, err := s.ListOriginDecisions(ctx, "a1")
	require.NoError(t, err)
	require.Len(t, decisions, 2)
	assert.Equal(t, "https://blocked.example.com", decisions[0].Origin)
	assert.Equal(t, DecisionBlock, decisions[0].Decision, "the block decision survives an allowlist-only write")
}

// exhibit-x87: explicitly approving a blocked origin overrides the block.
func TestReplaceAllowedOriginsOverridesABlockOnTheSameOrigin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putTestArtifact(t, s, "a1", nil)
	require.NoError(t, s.SetOriginDecision(ctx, "a1", "https://x.example.com", DecisionBlock, "runtime"))

	require.NoError(t, s.ReplaceAllowedOrigins(ctx, "a1", []string{"https://x.example.com"}, "user"))

	decisions, err := s.ListOriginDecisions(ctx, "a1")
	require.NoError(t, err)
	require.Len(t, decisions, 1, "the upsert flips the decision instead of adding a row")
	assert.Equal(t, DecisionAllow, decisions[0].Decision)
}

// exhibit-x87: ON DELETE CASCADE removes an artifact's decisions with it.
func TestOriginDecisionsCascadeOnArtifactDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putTestArtifact(t, s, "a1", []string{"https://ok.example.com"})
	require.NoError(t, s.SetOriginDecision(ctx, "a1", "https://nope.example.com", DecisionBlock, "runtime"))

	require.NoError(t, s.DeleteArtifact(ctx, "a1"))

	var count int
	require.NoError(t, s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM artifact_network_origins WHERE artifact_id=?", "a1").Scan(&count))
	assert.Zero(t, count, "deleting the artifact must remove its origin decisions")
}
