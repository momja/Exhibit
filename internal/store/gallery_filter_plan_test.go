package store

import (
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGalleryFilterQueryPlans(t *testing.T) {
	s := newTestStore(t)
	for _, shared := range []bool{false, true} {
		for _, search := range []string{"", "chart"} {
			query, args := listArtifactsQuery(ListOptions{OwnerID: alice, Query: search, Tags: []string{"Work", "Read"}, CollectionIDs: []string{"one", "two"}, MatchAllFilters: true, SharedOnly: shared})
			plan := explainQueryPlan(t, s, query, args...)
			assert.NotRegexp(t, `SCAN membership(?:\s|$)`, plan)
			assert.Contains(t, plan, "artifact_tags_by_tag")
			assert.Contains(t, plan, "artifact_collections_by_collection")
		}
	}
}

func TestGalleryFilterIndexesMigration(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, goose.DownTo(s.db, "migrations", 44))
	var count int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('artifact_tags_by_tag', 'artifact_collections_by_collection')`).Scan(&count))
	assert.Zero(t, count)
	require.NoError(t, goose.Up(s.db, "migrations"))
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('artifact_tags_by_tag', 'artifact_collections_by_collection')`).Scan(&count))
	assert.Equal(t, 2, count)
}
