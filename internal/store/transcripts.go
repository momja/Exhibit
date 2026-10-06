package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Agent conversations kept with the artifact they worked on (migration 033).
//
// A conversation is stored as Pi writes it: its session file, which is also
// the one format `pi --session` starts from, so keeping it is what makes a
// conversation resumable and nothing else is needed for that. It is stored in
// the row that already existed for the conversation, beside two facts about it
// — which version of the artifact it was last working against, and what it was
// about — because those are what a person needs to pick one from a list.

// Transcript is one agent conversation.
type Transcript struct {
	ArtifactID string `json:"-"`
	SessionID  string `json:"session_id"`
	// Title is the conversation's first prompt, shortened.
	Title string `json:"title"`
	// VersionSeq is the artifact's head version when the conversation last
	// settled: the code and saved data it was last working against. 0 means
	// unknown, which only conversations kept before migration 033 are.
	VersionSeq int `json:"version_seq"`
	// Resumable reports whether a session can be started from it. Only a
	// conversation kept with a session file can; an older one holds a dump of
	// Pi's messages, which can be read and nothing more.
	Resumable bool      `json:"resumable"`
	UpdatedAt time.Time `json:"updated_at"`

	// What a conversation is made of, loaded by GetTranscript alone. They are
	// the bulk of a row, and a list has no use for them.
	SessionFile string `json:"-"` // Pi's session file; empty for an older conversation
	Messages    string `json:"-"` // that older conversation's message dump
}

// SaveTranscript stores a conversation, replacing what was kept of it before.
// It is stamped with the artifact's head version in the same statement that
// stores the file, so the two cannot disagree about the moment they describe.
func (s *SQLiteStore) SaveTranscript(ctx context.Context, ownerID int64, t Transcript) error {
	if err := s.ownsArtifact(ctx, ownerID, t.ArtifactID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_transcripts (artifact_id, session_id, title, session_file, version_seq, updated_at)
		 VALUES (?, ?, ?, ?, (SELECT MAX(seq) FROM artifact_versions WHERE artifact_id = ?), datetime('now'))
		 ON CONFLICT(artifact_id, session_id) DO UPDATE SET
		     title=excluded.title, session_file=excluded.session_file,
		     version_seq=excluded.version_seq, updated_at=excluded.updated_at`,
		t.ArtifactID, t.SessionID, t.Title, t.SessionFile, t.ArtifactID)
	return err
}

// ListTranscripts returns an artifact's conversations, most recently active
// first. Another owner's artifact reads as having none.
//
// "Has a session file" is asked with typeof() rather than a comparison: SQLite
// answers it from the record header, where a comparison would read every
// stored file just to say whether it is there.
func (s *SQLiteStore) ListTranscripts(ctx context.Context, ownerID int64, artifactID string) ([]Transcript, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, title, version_seq, typeof(session_file) = 'text', updated_at
		   FROM agent_transcripts
		  WHERE artifact_id = ? AND `+ownedArtifact+`
		  ORDER BY updated_at DESC, session_id`,
		artifactID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Transcript{}
	for rows.Next() {
		t := Transcript{ArtifactID: artifactID}
		var updatedAt any
		if err := rows.Scan(&t.SessionID, &t.Title, &t.VersionSeq, &t.Resumable, &updatedAt); err != nil {
			return nil, err
		}
		t.UpdatedAt = anyToTime(updatedAt)
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTranscript returns one conversation with its contents, or (nil, nil) when
// it does not exist or the artifact is another owner's.
func (s *SQLiteStore) GetTranscript(ctx context.Context, ownerID int64, artifactID, sessionID string) (*Transcript, error) {
	t := &Transcript{ArtifactID: artifactID, SessionID: sessionID}
	var file sql.NullString
	var updatedAt any
	err := s.db.QueryRowContext(ctx,
		`SELECT title, version_seq, session_file, messages, updated_at
		   FROM agent_transcripts
		  WHERE artifact_id = ? AND session_id = ? AND `+ownedArtifact,
		artifactID, sessionID, ownerID).Scan(&t.Title, &t.VersionSeq, &file, &t.Messages, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.SessionFile, t.Resumable = file.String, file.Valid
	t.UpdatedAt = anyToTime(updatedAt)
	return t, nil
}
