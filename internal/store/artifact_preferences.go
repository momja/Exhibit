package store

import (
	"context"
	"strings"
	"time"
)

// ViewerGalleryOptions selects one page from the viewer's owned and granted
// artifacts. Stars and opening history belong to the viewer, never the file.
type ViewerGalleryOptions struct {
	Query                   string
	Tags, CollectionIDs     []string
	SharedOnly, StarredOnly bool
	Limit, Offset           int
}

type ArtifactPreference struct {
	Starred      bool
	LastOpenedAt time.Time
}

func (s *SQLiteStore) SetArtifactStarred(ctx context.Context, viewer ViewerID, id string, starred bool) error {
	return s.writeArtifactPreference(ctx, viewer, id, `starred`, starred)
}

func (s *SQLiteStore) RecordArtifactOpened(ctx context.Context, viewer ViewerID, id string) error {
	return s.writeArtifactPreference(ctx, viewer, id, `last_opened_at`, time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"))
}

// The access check and upsert share a statement, including for recipients.
// A revoked grant cannot update preferences or make a file discoverable.
func (s *SQLiteStore) writeArtifactPreference(ctx context.Context, viewer ViewerID, id, column string, value any) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO artifact_preferences(viewer_id, artifact_id, `+column+`)
        SELECT ?, a.id, ? FROM artifacts a WHERE a.id = ? AND
        (a.owner_id = ? OR EXISTS (SELECT 1 FROM share_grants g WHERE g.artifact_id = a.id AND g.recipient_id = ?))
        ON CONFLICT(viewer_id,artifact_id) DO UPDATE SET `+column+`=excluded.`+column,
		viewer, value, id, viewer, viewer)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrNotFound
	}
	return err
}

func (s *SQLiteStore) ArtifactPreferences(ctx context.Context, viewer ViewerID, ids []string) (map[string]ArtifactPreference, error) {
	preferences := map[string]ArtifactPreference{}
	if len(ids) == 0 {
		return preferences, nil
	}
	args := []any{viewer}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id, starred, COALESCE(last_opened_at,'') FROM artifact_preferences
        WHERE viewer_id = ? AND artifact_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, opened string
		var preference ArtifactPreference
		if err := rows.Scan(&id, &preference.Starred, &opened); err != nil {
			return nil, err
		}
		preference.LastOpenedAt, _ = time.Parse(time.RFC3339Nano, opened)
		preferences[id] = preference
	}
	return preferences, rows.Err()
}

func (s *SQLiteStore) ListViewerGallery(ctx context.Context, viewer ViewerID, opts ViewerGalleryOptions) ([]*SharedArtifact, error) {
	query, args := readableSearchBase(viewer, opts.Query)
	if opts.SharedOnly {
		query += ` AND (a.owner_id != ? OR ` + artifactHasShares + `)`
		args = append(args, viewer)
	}
	for _, tag := range opts.Tags {
		query += ` AND a.id IN (SELECT at.artifact_id FROM artifact_tags at JOIN tags t ON t.id = at.tag_id WHERE t.owner_id = ? AND t.name = ?)`
		args = append(args, viewer, tag)
	}
	for _, collection := range opts.CollectionIDs {
		query += ` AND a.id IN (SELECT ac.artifact_id FROM artifact_collections ac JOIN collections c ON c.id = ac.collection_id WHERE c.owner_id = ? AND c.id = ?)`
		args = append(args, viewer, collection)
	}
	if opts.StarredOnly {
		query += ` AND a.id IN (SELECT artifact_id FROM artifact_preferences WHERE viewer_id = ? AND starred = 1)
            ORDER BY (SELECT last_opened_at FROM artifact_preferences WHERE viewer_id = ? AND artifact_id = a.id) DESC,
            julianday(a.created_at) DESC, a.id DESC LIMIT ? OFFSET ?`
		args = append(args, viewer, viewer, galleryLimit(opts.Limit), opts.Offset)
	} else {
		query, args = artifactListPage(query, args, opts.Limit, opts.Offset)
	}
	return s.listArtifactsWithOwner(ctx, query, args)
}

func galleryLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	return limit
}
