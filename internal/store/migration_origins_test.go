package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertRawOrigin writes an artifact_network_origins row the way pre-av-i7hd
// code did: verbatim, with no normalization. Nothing behind the Store
// interface can do this any more, which is the point — the repair has to be
// exercised against raw SQL.
func insertRawOrigin(t *testing.T, s *SQLiteStore, artifactID, origin, decision string) {
	t.Helper()
	_, err := s.db.Exec(
		`INSERT INTO artifact_network_origins (artifact_id, origin, decision, source)
		 VALUES (?, ?, ?, 'legacy')`, artifactID, origin, decision)
	require.NoError(t, err)
}

func seedArtifact(t *testing.T, s *SQLiteStore, id string) {
	t.Helper()
	require.NoError(t, s.PutArtifact(context.Background(), &Artifact{
		ID: id, OwnerID: 1, Title: id, SourceBlobID: "blob-" + id,
		Tier: Tier1, NetworkAllowlist: []string{}, CreatedAt: time.Now().UTC(),
	}))
}

// The v12 repair (av-i7hd) is what makes the "a row is an origin" invariant
// true for artifacts nobody has edited since the validation landed.
func TestNormalizeStoredOriginsRepairsLegacyRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedArtifact(t, s, "art-1")

	// The live artifact from the bug report: a bare origin, the same origin
	// with a path, and again with a sentence-terminating dot.
	insertRawOrigin(t, s, "art-1", "https://unpkg.com", DecisionAllow)
	insertRawOrigin(t, s, "art-1", "https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/esm/worker.js", DecisionAllow)
	insertRawOrigin(t, s, "art-1", "https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/esm/worker.js.", DecisionAllow)
	insertRawOrigin(t, s, "art-1", "https://CDN.example.com/", DecisionAllow)
	insertRawOrigin(t, s, "art-1", "'self'", DecisionAllow)

	runOriginRepair(t, s)

	allowed, err := s.AllowedOrigins(ctx, "art-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"https://cdn.example.com", "https://unpkg.com"}, allowed,
		"near-duplicates collapse onto the origin they always effectively named; a keyword is dropped")
}

// A collapse must never upgrade a "don't ask again" into network access.
func TestNormalizeStoredOriginsKeepsBlockOverAllow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedArtifact(t, s, "art-2")

	insertRawOrigin(t, s, "art-2", "https://tracker.example.com/pixel.gif", DecisionAllow)
	insertRawOrigin(t, s, "art-2", "https://tracker.example.com", DecisionBlock)

	runOriginRepair(t, s)

	decisions, err := s.ListOriginDecisions(ctx, "art-2")
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "https://tracker.example.com", decisions[0].Origin)
	assert.Equal(t, DecisionBlock, decisions[0].Decision)

	allowed, err := s.AllowedOrigins(ctx, "art-2")
	require.NoError(t, err)
	assert.Empty(t, allowed, "the repair may narrow a policy, never widen one")
}

// Already-clean rows are left exactly as they are (the repair is a no-op on a
// database that never held a dirty row).
func TestNormalizeStoredOriginsLeavesCleanRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedArtifact(t, s, "art-3")
	require.NoError(t, s.ReplaceAllowedOrigins(ctx, "art-3", []string{"https://a.example.com", "https://b.example.com"}, "user"))

	runOriginRepair(t, s)

	allowed, err := s.AllowedOrigins(ctx, "art-3")
	require.NoError(t, err)
	assert.Equal(t, []string{"https://a.example.com", "https://b.example.com"}, allowed)
}

func runOriginRepair(t *testing.T, s *SQLiteStore) {
	t.Helper()
	tx, err := s.db.Begin()
	require.NoError(t, err)
	require.NoError(t, normalizeStoredOrigins(context.Background(), tx))
	require.NoError(t, tx.Commit())
}
