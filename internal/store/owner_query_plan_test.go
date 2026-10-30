package store

import (
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnerScopedQueryPlans(t *testing.T) {
	s := newTestStore(t)
	// Exercise 040's Down (via DownTo 39, so later migrations do not shift
	// which one is undone) and the upgrade from layer 2 as well as the fresh
	// migration path newTestStore already took.
	require.NoError(t, goose.DownTo(s.db, "migrations", 39))
	var indexes int
	require.NoError(t, s.db.QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='artifacts_owner'").Scan(&indexes))
	require.Zero(t, indexes)
	require.NoError(t, goose.Up(s.db, "migrations"))

	for _, tc := range []struct {
		name, query string
		args        []any
	}{
		{"ListOriginDecisions", listOriginDecisionsQuery(), []any{"artifact", alice}},
		{"GetState", getStateQuery(), []any{"artifact", alice, alice}},
		{"ListVersions", listVersionsQuery(), []any{"artifact", alice}},
		{"VersionAssets", versionAssetsQuery(), []any{"artifact", 1, alice}},
		{"ListTranscripts", listTranscriptsQuery(), []any{"artifact", alice}},
		{"ListArtifactShares", listArtifactSharesQuery(), []any{"artifact", alice}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := explainQueryPlan(t, s, tc.query, tc.args...)
			assert.Regexp(t, `SEARCH artifacts(?: EXISTS)? USING INDEX sqlite_autoindex_artifacts_1 \(id=\?\)`, plan)
			assert.NotContains(t, plan, "SCAN artifacts")
			assert.NotContains(t, plan, "LIST SUBQUERY")
		})
	}

	t.Run("GetAccountSummary", func(t *testing.T) {
		plan := explainQueryPlan(t, s, accountSummaryQuery(), alice, alice, alice)
		assert.Regexp(t, `SEARCH artifacts USING (?:COVERING )?INDEX artifacts_owner \(owner_id=\?\)`, plan)
		assert.Regexp(t, `SEARCH share_links USING (?:COVERING )?INDEX \S+ \(artifact_id=\?\)`, plan)
		assert.Regexp(t, `SEARCH share_grants USING (?:COVERING INDEX \S+|PRIMARY KEY) \(artifact_id=\?\)`, plan)
		assert.NotContains(t, plan, "SCAN share_links")
		assert.NotContains(t, plan, "SCAN share_grants")
		assert.NotContains(t, plan, "SCAN artifacts")
	})

	// These callers pin a share's primary key rather than artifact_id. They
	// still check at most one artifact instead of enumerating the share table.
	for _, tc := range []struct {
		name, query string
	}{
		{"GetShare", getShareQuery()},
		{"DeleteShare", deleteShareQuery()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := explainQueryPlan(t, s, tc.query, "share", alice)
			assert.Contains(t, plan, "SEARCH share_links USING INDEX sqlite_autoindex_share_links_1 (id=?)")
			assert.Regexp(t, `SEARCH artifacts(?: EXISTS)? USING INDEX sqlite_autoindex_artifacts_1 \(id=\?\)`, plan)
			assert.NotContains(t, plan, "SCAN share_links")
			assert.NotContains(t, plan, "SCAN artifacts")
			assert.NotContains(t, plan, "LIST SUBQUERY")
		})
	}

	t.Run("ListArtifacts", func(t *testing.T) {
		query, args := listArtifactsQuery(ListOptions{OwnerID: alice, Limit: 24})
		plan := explainQueryPlan(t, s, query, args...)
		assert.Contains(t, plan, "SEARCH a USING INDEX artifacts_owner (owner_id=?)")
		assert.NotContains(t, plan, "SCAN a ")
		// created_at is ordered by julianday(), so sorting still needs a
		// temporary B-tree; artifacts_owner only narrows the owner's rows.
		assert.Contains(t, plan, "USE TEMP B-TREE FOR ORDER BY")
	})

	for _, tc := range []struct {
		name string
		opts SharedListOptions
	}{
		{"all", SharedListOptions{}},
		{"owners", SharedListOptions{OwnerIDs: []int64{alice, bob}}},
		{"search", SharedListOptions{Query: "chart"}},
		{"owners and search", SharedListOptions{OwnerIDs: []int64{alice}, Query: "chart"}},
	} {
		t.Run("ListArtifactsSharedWith/"+tc.name, func(t *testing.T) {
			query, args := listArtifactsSharedWithQuery(ViewerID(bob), tc.opts)
			plan := explainQueryPlan(t, s, query, args...)
			assertRecipientQueryPlan(t, plan)
		})
	}
	t.Run("SharedWithOwners", func(t *testing.T) {
		plan := explainQueryPlan(t, s, sharedWithOwnersQuery(), bob, bob)
		assertRecipientQueryPlan(t, plan)
	})
	t.Run("SpaceCounts", func(t *testing.T) {
		plan := explainQueryPlan(t, s, spaceCountsQuery(), bob, bob, bob)
		assertRecipientQueryPlan(t, plan)
		assert.Contains(t, plan, "SEARCH a USING INDEX artifacts_owner (owner_id=?)")
	})
	t.Run("ListArtifacts/SharedOnly", func(t *testing.T) {
		query, args := listArtifactsQuery(ListOptions{OwnerID: alice, SharedOnly: true})
		plan := explainQueryPlan(t, s, query, args...)
		assert.Contains(t, plan, "SEARCH a USING INDEX artifacts_owner (owner_id=?)")
		assertNoSharingTableScans(t, plan)
	})
}

func assertRecipientQueryPlan(t *testing.T, plan string) {
	t.Helper()
	assert.Regexp(t, `SEARCH share_grants USING (?:COVERING )?INDEX share_grants_by_recipient \(recipient_id=\?\)`, plan)
	assert.Contains(t, plan, "SEARCH a USING INDEX sqlite_autoindex_artifacts_1 (id=?)")
	assertNoSharingTableScans(t, plan)
}

func assertNoSharingTableScans(t *testing.T, plan string) {
	t.Helper()
	// FTS5 describes its indexed MATCH as a virtual-table SCAN, and scalar
	// counts may SCAN CONSTANT ROW. Neither is a scan of a library table.
	assert.NotRegexp(t, `SCAN (?:artifacts|a|share_grants|g|share_links|l|users|u)(?:\s|$)`, plan)
}

func TestOwnedArtifactRejectsUnknownIdentifiers(t *testing.T) {
	for _, identifier := range []string{"", "artifacts", "unknown_alias", "share_links; DROP TABLE artifacts", "share_links --"} {
		t.Run(identifier, func(t *testing.T) {
			require.Panics(t, func() { ownedArtifact(identifier) })
		})
	}
}

func explainQueryPlan(t *testing.T, s *SQLiteStore, query string, args ...any) string {
	t.Helper()
	rows, err := s.db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query, args...)
	require.NoError(t, err)
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		details = append(details, detail)
	}
	require.NoError(t, rows.Err())
	plan := strings.Join(details, "\n")
	t.Log(plan)
	return plan
}
