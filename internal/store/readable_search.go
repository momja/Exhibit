package store

import "context"

func (s *SQLiteStore) SearchArtifactsReadableBy(ctx context.Context, viewer ViewerID, query string, limit, offset int) ([]*SharedArtifact, error) {
	statement, args := readableSearchQuery(viewer, query, limit, offset)
	return s.listArtifactsWithOwner(ctx, statement, args)
}

func readableSearchQuery(viewer ViewerID, query string, limit, offset int) (string, []any) {
	// Build the accessible set from indexed ownership and recipient lookups.
	// UNION deduplicates self-grants before applying a single ordered page.
	statement := `SELECT ` + artifactColsA + `, COALESCE(u.email, ''), COALESCE(u.external_id, '')
		FROM artifacts a LEFT JOIN users u ON u.id = a.owner_id
		WHERE a.rowid IN (
			SELECT rowid FROM artifacts WHERE owner_id = ?
			UNION SELECT a.rowid` + sharedWithFrom + ` WHERE share_grants.recipient_id = ?)`
	args := []any{viewer, viewer}
	if match := ftsMatchExpression(query); match != "" {
		statement += ` AND a.rowid IN (SELECT rowid FROM artifacts_fts WHERE artifacts_fts MATCH ?)`
		args = append(args, match)
	}
	return artifactListPage(statement, args, limit, offset)
}
