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
	// Exercise 039's Down and the upgrade from layer 2 as well as the fresh
	// migration path newTestStore already took.
	require.NoError(t, goose.DownTo(s.db, "migrations", 38))
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
