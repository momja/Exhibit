package store

import "context"

// ArtifactActivity carries metadata only: listing never reads version bodies,
// saved data snapshots, or full conversation files.
type ArtifactActivity struct {
	Version
	Title           string
	HasConversation bool
}

func (s *SQLiteStore) ListArtifactActivity(ctx context.Context, ownerID int64, limit, offset int) ([]ArtifactActivity, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.title, v.seq, v.origin, v.message, v.session_id, v.created_at,
        EXISTS (SELECT 1 FROM agent_transcripts t WHERE t.artifact_id = a.id AND t.session_id = v.session_id)
        FROM artifacts a JOIN artifact_versions v ON v.artifact_id = a.id
        WHERE a.owner_id = ? ORDER BY julianday(v.created_at) DESC, a.id DESC, v.seq DESC LIMIT ? OFFSET ?`, ownerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ArtifactActivity{}
	for rows.Next() {
		var item ArtifactActivity
		var created any
		if err := rows.Scan(&item.ArtifactID, &item.Title, &item.Seq, &item.Origin, &item.Message, &item.SessionID, &created, &item.HasConversation); err != nil {
			return nil, err
		}
		item.CreatedAt = anyToTime(created)
		items = append(items, item)
	}
	return items, rows.Err()
}
