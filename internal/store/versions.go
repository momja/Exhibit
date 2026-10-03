package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Artifact version history (migration 031).
//
// A version is a point in an artifact's life that can be returned to. It is
// the body and the widget as they were, plus the state the code had written by
// the time the next version replaced it — so a restore puts back a coherent
// pair, not just text.
//
// Everything here follows from three decisions.
//
// The head is a version. artifacts.source_blob_id and widget_blob_id stay as
// the head's pointers because the render path reads them, but they are written
// only by commitVersionTx, in the same transaction as the version row they
// mirror. There is no second way to change an artifact's content.
//
// A change snapshots the state of the version it replaces, in that same
// transaction. "State right before a new version" is therefore exact rather
// than approximate, and no caller has to remember to take one. Each snapshot is
// one JSON object on the outgoing version's row, not rows of its own: a
// restore must return to a single coherent moment, which is the opposite of
// what the live per-key table is shaped for.
//
// Blobs are never overwritten. A change names a new blob, so an older
// version's bytes are exactly what they were, and a blob two versions share
// stays alive while either does (blobqueue.go).

// Version origins. They say what produced a version, for display; nothing
// branches on them.
const (
	VersionInitial = "initial"
	VersionEdit    = "edit"
	VersionAgent   = "agent"
	VersionRefetch = "refetch"
	VersionRestore = "restore"
)

// ErrAlreadyCurrent means a restore named the version that is already the
// head: there is nothing to return to, and recording a duplicate would only
// add noise to the history.
var ErrAlreadyCurrent = errors.New("that version is already the current one")

// Version is one row of an artifact's history.
type Version struct {
	ArtifactID string `json:"artifact_id"`
	Seq        int    `json:"seq"`
	Origin     string `json:"origin"`
	// Message labels the version for a person: the prompt that caused an agent
	// version, the source URL of a refetch, "Restored v3".
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
	// The blobs the version names. Internal: the API never exposes them.
	BodyBlobID   string `json:"-"`
	WidgetBlobID string `json:"-"`
	// HasState reports whether a snapshot of the state this version left behind
	// exists — true once it has been superseded, false for the head.
	HasState  bool      `json:"has_state"`
	Current   bool      `json:"current"`
	CreatedAt time.Time `json:"created_at"`
}

// Provenance records what produced a version. The zero value is a manual edit.
type Provenance struct {
	Origin    string
	Message   string
	SessionID string
}

// VersionChange is one new version. A nil BodyBlobID or WidgetBlobID leaves
// that part as the head has it, read inside the transaction — so a widget save
// and a body save racing each other cannot undo one another. A non-nil
// WidgetBlobID that points at "" removes the widget.
type VersionChange struct {
	Provenance
	BodyBlobID   *string
	WidgetBlobID *string
	// SourceText is the body's search shadow (ExtractSearchText). Set it
	// whenever BodyBlobID is, because the blob store is not reachable from SQL.
	SourceText *string
}

const versionColumns = `v.seq, v.origin, v.message, v.session_id, v.body_blob_id, v.widget_blob_id,
       v.state_json IS NOT NULL,
       v.seq = (SELECT MAX(seq) FROM artifact_versions WHERE artifact_id = v.artifact_id),
       v.created_at`

func scanVersion(artifactID string, row interface{ Scan(...any) error }) (*Version, error) {
	v := &Version{ArtifactID: artifactID}
	var createdAt any
	if err := row.Scan(&v.Seq, &v.Origin, &v.Message, &v.SessionID, &v.BodyBlobID, &v.WidgetBlobID,
		&v.HasState, &v.Current, &createdAt); err != nil {
		return nil, err
	}
	v.CreatedAt = anyToTime(createdAt)
	return v, nil
}

// ListVersions returns an artifact's history, newest first. Another owner's
// artifact reads as having none, like every other artifact-scoped read.
func (s *SQLiteStore) ListVersions(ctx context.Context, ownerID int64, artifactID string) ([]Version, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+versionColumns+` FROM artifact_versions v
		  WHERE v.artifact_id = ? AND v.`+ownedArtifact+`
		  ORDER BY v.seq DESC`, artifactID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		v, err := scanVersion(artifactID, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// HeadVersionSeq returns the sequence number of an artifact's current version.
// It is 0 for an artifact that is not the owner's, like every other read here
// that has nothing to say about somebody else's.
func (s *SQLiteStore) HeadVersionSeq(ctx context.Context, ownerID int64, artifactID string) (int, error) {
	var seq sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(seq) FROM artifact_versions WHERE artifact_id = ? AND `+ownedArtifact,
		artifactID, ownerID).Scan(&seq)
	return int(seq.Int64), err
}

// GetVersion returns one version, or (nil, nil) when it does not exist or the
// artifact is another owner's.
func (s *SQLiteStore) GetVersion(ctx context.Context, ownerID int64, artifactID string, seq int) (*Version, error) {
	v, err := scanVersion(artifactID, s.db.QueryRowContext(ctx,
		`SELECT `+versionColumns+` FROM artifact_versions v
		  WHERE v.artifact_id = ? AND v.seq = ? AND v.`+ownedArtifact,
		artifactID, seq, ownerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

// VersionView is an earlier version as it is looked at without being restored:
// the code as it was, and the data that version left behind. It is exactly the
// pair RestoreVersion would put back, read from the same snapshot, so what a
// person is shown is what a restore gives them.
type VersionView struct {
	BodyBlobID string
	State      map[string]string
}

// GetVersionView returns what looking at an earlier version shows, and changes
// nothing.
//
// The head is ErrAlreadyCurrent, as it is for a restore: its data is the live
// rows rather than a snapshot, and the artifact's own page already shows it, so
// there is no earlier state to look at. Another owner's artifact and a version
// that was never made are both ErrNotFound.
func (s *SQLiteStore) GetVersionView(ctx context.Context, ownerID int64, artifactID string, seq int) (*VersionView, error) {
	var body string
	var snapshot sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT body_blob_id, state_json FROM artifact_versions
		  WHERE artifact_id = ? AND seq = ? AND `+ownedArtifact,
		artifactID, seq, ownerID).Scan(&body, &snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !snapshot.Valid {
		return nil, ErrAlreadyCurrent
	}
	var state map[string]string
	if err := json.Unmarshal([]byte(snapshot.String), &state); err != nil {
		return nil, fmt.Errorf("decode state snapshot: %w", err)
	}
	return &VersionView{BodyBlobID: body, State: state}, nil
}

// CommitVersion makes a change to an artifact's body or widget the new head.
// It returns the new version.
func (s *SQLiteStore) CommitVersion(ctx context.Context, ownerID int64, artifactID string, c VersionChange) (*Version, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	v, err := commitVersionTx(ctx, tx, ownerID, artifactID, c)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return v, nil
}

// RestoreVersion returns an artifact to an earlier version: its body and
// widget become the head again, as a new version rather than a rewind (nothing
// in the history is discarded), and the state that version left behind
// replaces the live state.
//
// The state the artifact has right now is snapshotted onto the version being
// left, exactly as for any other change, so a restore can itself be undone.
// A version with no snapshot is the head, which is ErrAlreadyCurrent.
func (s *SQLiteStore) RestoreVersion(ctx context.Context, ownerID int64, artifactID string, seq int, p Provenance, sourceText string) (*Version, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	var body, widget string
	var state sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT body_blob_id, widget_blob_id, state_json FROM artifact_versions
		  WHERE artifact_id = ? AND seq = ? AND `+ownedArtifact,
		artifactID, seq, ownerID).Scan(&body, &widget, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !state.Valid {
		return nil, ErrAlreadyCurrent
	}

	v, err := commitVersionTx(ctx, tx, ownerID, artifactID, VersionChange{
		Provenance: p, BodyBlobID: &body, WidgetBlobID: &widget, SourceText: &sourceText,
	})
	if err != nil {
		return nil, err
	}
	if err := replaceStateTx(ctx, tx, ownerID, artifactID, state.String); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return v, nil
}

func commitVersionTx(ctx context.Context, tx *sql.Tx, ownerID int64, artifactID string, c VersionChange) (*Version, error) {
	var headSeq int
	var body, widget string
	err := tx.QueryRowContext(ctx,
		`SELECT v.seq, v.body_blob_id, v.widget_blob_id FROM artifact_versions v
		  WHERE v.artifact_id = ? AND v.`+ownedArtifact+`
		  ORDER BY v.seq DESC LIMIT 1`, artifactID, ownerID).Scan(&headSeq, &body, &widget)
	if errors.Is(err, sql.ErrNoRows) {
		// Another owner's artifact and one that never existed are the same
		// answer here, as everywhere else on this interface.
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// The state as it stands right before this change, recorded on the version
	// it leaves.
	snapshot, err := snapshotStateTx(ctx, tx, ownerID, artifactID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE artifact_versions SET state_json = ? WHERE artifact_id = ? AND seq = ?`,
		snapshot, artifactID, headSeq); err != nil {
		return nil, err
	}

	if c.BodyBlobID != nil {
		body = *c.BodyBlobID
	}
	if c.WidgetBlobID != nil {
		widget = *c.WidgetBlobID
	}
	origin := c.Origin
	if origin == "" {
		origin = VersionEdit
	}
	seq := headSeq + 1
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO artifact_versions (artifact_id, seq, origin, message, session_id, body_blob_id, widget_blob_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		artifactID, seq, origin, c.Message, c.SessionID, body, widget); err != nil {
		return nil, err
	}

	// The head's pointers, in step with the row just written.
	if c.SourceText != nil {
		_, err = tx.ExecContext(ctx,
			`UPDATE artifacts SET source_blob_id = ?, widget_blob_id = ?, source_text = ?, updated_at = datetime('now')
			  WHERE id = ? AND owner_id = ?`, body, widget, *c.SourceText, artifactID, ownerID)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE artifacts SET source_blob_id = ?, widget_blob_id = ?, updated_at = datetime('now')
			  WHERE id = ? AND owner_id = ?`, body, widget, artifactID, ownerID)
	}
	if err != nil {
		return nil, err
	}

	return scanVersion(artifactID, tx.QueryRowContext(ctx,
		`SELECT `+versionColumns+` FROM artifact_versions v WHERE v.artifact_id = ? AND v.seq = ?`,
		artifactID, seq))
}

// snapshotStateTx serializes the owner's state rows as one JSON object. The
// owner's rows are the artifact's state: on a shared-state artifact they are
// the board everyone plays on, and on any other a recipient's rows are theirs
// alone and never enter the owner's history.
func snapshotStateTx(ctx context.Context, tx *sql.Tx, ownerID int64, artifactID string) (string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT key, value FROM artifact_state WHERE artifact_id = ? AND user_id = ?`, artifactID, ownerID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	state := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return "", err
		}
		state[k] = v
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// replaceStateTx makes the owner's state rows exactly the snapshot.
func replaceStateTx(ctx context.Context, tx *sql.Tx, ownerID int64, artifactID, snapshot string) error {
	var state map[string]string
	if err := json.Unmarshal([]byte(snapshot), &state); err != nil {
		return fmt.Errorf("decode state snapshot: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM artifact_state WHERE artifact_id = ? AND user_id = ?`, artifactID, ownerID); err != nil {
		return err
	}
	for k, v := range state {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO artifact_state (artifact_id, user_id, key, value) VALUES (?, ?, ?, ?)`,
			artifactID, ownerID, k, v); err != nil {
			return err
		}
	}
	return nil
}

// versionBlobIDs is every blob any version of one artifact names. The rows are
// about to be cascaded away with the artifact, so this is read first, like the
// asset blobs.
func versionBlobIDs(ctx context.Context, tx *sql.Tx, artifactID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT body_blob_id, widget_blob_id FROM artifact_versions WHERE artifact_id = ?`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var body, widget string
		if err := rows.Scan(&body, &widget); err != nil {
			return nil, err
		}
		for _, id := range []string{body, widget} {
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids, rows.Err()
}
