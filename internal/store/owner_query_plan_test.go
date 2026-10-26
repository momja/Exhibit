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
	// Exercise 044's Down and the upgrade from layer 2 as well as the fresh
	// migration path newTestStore already took.
	require.NoError(t, goose.Down(s.db, "migrations"))
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
