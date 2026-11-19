package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadableSearchScopeAndPaging(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	seedOwnerAccounts(t, s)
	for i := range 53 {
		owner := alice
		if i%2 == 0 {
			owner = bob
		}
		a := &Artifact{ID: fmt.Sprintf("tool-%02d", i), OwnerID: owner, Title: "Chart tool", SourceBlobID: "fixture", SourceText: "visible dashboard", Tier: Tier1, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		require.NoError(t, s.PutArtifact(ctx, a))
		// Self-grants and several recipients must never duplicate a hit.
		_, err := s.CreateGrant(ctx, owner, a.ID, bob)
		require.NoError(t, err)
		_, err = s.CreateGrant(ctx, owner, a.ID, alice)
		require.NoError(t, err)
	}
	for _, id := range []string{"private", "public-only", "wrong-recipient"} {
		require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: id, OwnerID: alice, Title: "Chart tool", SourceText: "visible dashboard", SourceBlobID: "fixture", Tier: Tier1}))
	}
	_, err := s.CreateAnonymousLink(ctx, alice, "public-only")
	require.NoError(t, err)
	_, err = s.CreateGrant(ctx, alice, "wrong-recipient", alice)
	require.NoError(t, err)
	var all []string
	for offset := 0; offset < 53; offset += 24 {
		page, err := s.SearchArtifactsReadableBy(ctx, ViewerID(bob), "cha", 24, offset)
		require.NoError(t, err)
		for _, hit := range page {
			readable, err := s.GetArtifactReadableBy(ctx, ViewerID(bob), hit.ID)
			require.NoError(t, err)
			require.NotNil(t, readable)
			assert.Equal(t, readable, hit.Artifact)
			require.NotEmpty(t, hit.OwnerName)
			all = append(all, hit.ID)
		}
	}
	require.Len(t, all, 53)
	for i, id := range all {
		assert.Equal(t, fmt.Sprintf("tool-%02d", 52-i), id)
	}
	// Visible source, tags and literal FTS punctuation use the same index as
	// the library; the viewer scope must survive each query shape.
	tag := &Tag{ID: "search-tag", OwnerID: alice, Name: "UniqueTag"}
	require.NoError(t, s.CreateTag(ctx, tag))
	require.NoError(t, s.AddArtifactTag(ctx, alice, "tool-01", tag.ID))
	for query, count := range map[string]int{"dashboard": 53, "UniqueTag": 1, "absent": 0, "a:b": 0, "\"cha\"": 53} {
		got, err := s.SearchArtifactsReadableBy(ctx, ViewerID(bob), query, 100, 0)
		require.NoError(t, err, query)
		assert.Len(t, got, count, query)
	}
	require.NoError(t, s.DeleteGrant(ctx, alice, "tool-01", bob))
	got, err := s.SearchArtifactsReadableBy(ctx, ViewerID(bob), "UniqueTag", 24, 0)
	require.NoError(t, err)
	assert.Empty(t, got)
	got, err = s.SearchArtifactsReadableBy(ctx, ViewerID(0), "chart", 24, 0)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestReadableSearchQueryPlans(t *testing.T) {
	s := newTestStore(t)
	for _, search := range []string{"", "chart"} {
		query, args := readableSearchQuery(ViewerID(bob), search, 24, 0)
		plan := explainQueryPlan(t, s, query, args...)
		assert.Contains(t, plan, "artifacts_owner")
		assert.Contains(t, plan, "share_grants_by_recipient")
		assert.NotRegexp(t, `SCAN (a|artifacts|share_grants)(?:\s|$)`, plan)
	}
}
