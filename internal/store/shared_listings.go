package store

import (
	"context"
	"sort"
	"strings"
)

// Both subqueries probe a single artifact's primary/unique key. Starting at
// an owner's indexed artifacts keeps private libraries bounded by that owner.
const artifactHasShares = `(EXISTS (SELECT 1 FROM share_grants WHERE share_grants.artifact_id = a.id)
	OR EXISTS (SELECT 1 FROM share_links WHERE share_links.artifact_id = a.id))`

// CROSS JOIN fixes the recipient-first traversal even with an OwnerIDs/FTS
// filter: search share_grants_by_recipient, then look up each artifact by ID.
// An unbounded artifact query with a correlated grant EXISTS would instead
// visit the whole library. The grant PK also makes each incoming artifact unique.
const sharedWithFrom = ` FROM share_grants
	CROSS JOIN artifacts a ON a.id = share_grants.artifact_id`
const sharedWithWhere = ` WHERE share_grants.recipient_id = ? AND a.owner_id != ?`

func listArtifactsSharedWithQuery(viewer ViewerID, opts SharedListOptions) (string, []any) {
	query := `SELECT ` + artifactColsA + `, COALESCE(u.email, ''), COALESCE(u.external_id, '')` +
		sharedWithFrom + ` LEFT JOIN users u ON u.id = a.owner_id` + sharedWithWhere
	args := []any{viewer, viewer}
	if len(opts.OwnerIDs) > 0 {
		placeholders := make([]string, len(opts.OwnerIDs))
		for i, id := range opts.OwnerIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += ` AND a.owner_id IN (` + strings.Join(placeholders, ",") + `)`
	}
	if match := ftsMatchExpression(opts.Query); match != "" {
		query += ` AND a.rowid IN (SELECT rowid FROM artifacts_fts WHERE artifacts_fts MATCH ?)`
		args = append(args, match)
	}
	return artifactListPage(query, args, opts.Limit, opts.Offset)
}

func (s *SQLiteStore) ListArtifactsSharedWith(ctx context.Context, viewer ViewerID, opts SharedListOptions) ([]*SharedArtifact, error) {
	query, args := listArtifactsSharedWithQuery(viewer, opts)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*SharedArtifact
	var artifacts []*Artifact
	for rows.Next() {
		var owner User
		a, err := scanArtifact(rows, &owner.Email, &owner.ExternalID)
		if err != nil {
			return nil, err
		}
		name, _ := owner.DisplayName()
		results = append(results, &SharedArtifact{Artifact: a, OwnerName: name})
		artifacts = append(artifacts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachTags(ctx, artifacts); err != nil {
		return nil, err
	}
	if err := s.attachAllowlists(ctx, artifacts); err != nil {
		return nil, err
	}
	return results, nil
}

func sharedWithOwnersQuery() string {
	return `SELECT a.owner_id, COALESCE(u.email, ''), COALESCE(u.external_id, ''), COUNT(*)` +
		sharedWithFrom + ` LEFT JOIN users u ON u.id = a.owner_id` + sharedWithWhere + ` GROUP BY a.owner_id`
}

func (s *SQLiteStore) SharedWithOwners(ctx context.Context, viewer ViewerID) ([]SharedOwner, error) {
	rows, err := s.db.QueryContext(ctx, sharedWithOwnersQuery(), viewer, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SharedOwner
	for rows.Next() {
		var owner User
		var result SharedOwner
		if err := rows.Scan(&result.ID, &owner.Email, &owner.ExternalID, &result.Count); err != nil {
			return nil, err
		}
		result.Name, _ = owner.DisplayName()
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Sort after applying DisplayName so local names and subject fallbacks
	// follow the same ordering as email names, with a stable tie-breaker.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Name == results[j].Name {
			return results[i].ID < results[j].ID
		}
		return results[i].Name < results[j].Name
	})
	return results, nil
}

func spaceCountsQuery() string {
	return `SELECT (SELECT COUNT(*)` + sharedWithFrom + sharedWithWhere + `),
		(SELECT COUNT(*) FROM artifacts a WHERE a.owner_id = ? AND ` + artifactHasShares + `)`
}

func (s *SQLiteStore) SpaceCounts(ctx context.Context, viewer ViewerID) (sharedWith, sharedBy int, err error) {
	err = s.db.QueryRowContext(ctx, spaceCountsQuery(), viewer, viewer, viewer).Scan(&sharedWith, &sharedBy)
	return
}
