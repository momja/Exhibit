package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragmas: %w", err)
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	start := time.Now()
	if err := goose.Up(s.db, "migrations"); err != nil {
		return err
	}
	slog.Info("sqlite migrations applied", slog.Duration("duration", time.Since(start)))
	return nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func marshalAllowlist(list []string) string {
	if list == nil {
		list = []string{}
	}
	b, _ := json.Marshal(list)
	return string(b)
}

func unmarshalAllowlist(s string) []string {
	var list []string
	if err := json.Unmarshal([]byte(s), &list); err != nil || list == nil {
		return []string{}
	}
	return list
}

func parseTS(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func scanArtifact(rows interface{ Scan(...any) error }) (*Artifact, error) {
	var a Artifact
	var allowlistJSON string
	// Scan timestamps as any — the modernc sqlite driver may return them as time.Time or string
	var createdAt, updatedAt any
	err := rows.Scan(&a.ID, &a.OwnerID, &a.Title, &a.SourceBlobID, &a.SourceURL, &a.Tier, &createdAt, &updatedAt, &allowlistJSON, &a.DownloadsApproved)
	if err != nil {
		return nil, err
	}
	a.NetworkAllowlist = unmarshalAllowlist(allowlistJSON)
	switch v := createdAt.(type) {
	case time.Time:
		a.CreatedAt = v.UTC()
	case string:
		a.CreatedAt = parseTS(v)
	case []byte:
		a.CreatedAt = parseTS(string(v))
	}
	switch v := updatedAt.(type) {
	case time.Time:
		a.UpdatedAt = v.UTC()
	case string:
		a.UpdatedAt = parseTS(v)
	case []byte:
		a.UpdatedAt = parseTS(string(v))
	}
	return &a, nil
}

const artifactCols = "id, owner_id, title, source_blob_id, source_url, tier, created_at, updated_at, network_allowlist, downloads_approved"
const artifactColsA = "a.id, a.owner_id, a.title, a.source_blob_id, a.source_url, a.tier, a.created_at, a.updated_at, a.network_allowlist, a.downloads_approved"

func (s *SQLiteStore) PutArtifact(ctx context.Context, a *Artifact) error {
	now := a.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts (id, owner_id, title, source_blob_id, source_url, tier, network_allowlist, downloads_approved, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.OwnerID, a.Title, a.SourceBlobID, a.SourceURL, a.Tier, marshalAllowlist(a.NetworkAllowlist), a.DownloadsApproved,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	return err
}

func (s *SQLiteStore) GetArtifact(ctx context.Context, id string) (*Artifact, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+artifactCols+" FROM artifacts WHERE id = ?", id)
	a, err := scanArtifact(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.attachTags(ctx, []*Artifact{a}); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *SQLiteStore) ListArtifacts(ctx context.Context, opts ListOptions) ([]*Artifact, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	var (
		query  string
		args   []any
		wheres []string
	)

	// Always use alias 'a' for artifacts so WHERE clauses and ORDER BY are consistent.
	if opts.Query != "" {
		// Use a subquery to avoid ambiguous column names from the FTS5 JOIN.
		// The FTS5 MATCH expression works on the table name in a subquery.
		query = `SELECT ` + artifactColsA + ` FROM artifacts a
            WHERE a.rowid IN (SELECT rowid FROM artifacts_fts WHERE artifacts_fts MATCH ?)
            AND 1=1`
		args = append(args, opts.Query+"*")
	} else {
		query = `SELECT ` + artifactColsA + ` FROM artifacts a WHERE 1=1`
	}

	if len(opts.Tags) > 0 {
		placeholders := make([]string, len(opts.Tags))
		for i, t := range opts.Tags {
			placeholders[i] = "?"
			args = append(args, t)
		}
		wheres = append(wheres, `a.id IN (SELECT at.artifact_id FROM artifact_tags at
            JOIN tags t ON t.id = at.tag_id WHERE t.name IN (`+strings.Join(placeholders, ",")+`))`)
	}

	if len(opts.Collections) > 0 {
		placeholders := make([]string, len(opts.Collections))
		for i, c := range opts.Collections {
			placeholders[i] = "?"
			args = append(args, c)
		}
		wheres = append(wheres, `a.id IN (SELECT ac.artifact_id FROM artifact_collections ac
            JOIN collections c ON c.id = ac.collection_id WHERE c.name IN (`+strings.Join(placeholders, ",")+`))`)
	}

	for _, w := range wheres {
		query += " AND " + w
	}

	query += " ORDER BY a.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Artifact
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachTags(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *SQLiteStore) UpdateArtifact(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(updates)+1)
	args := make([]any, 0, len(updates)+1)
	for k, v := range updates {
		if k == "network_allowlist" {
			switch list := v.(type) {
			case []string:
				v = marshalAllowlist(list)
			case []interface{}:
				strs := make([]string, len(list))
				for i, s := range list {
					strs[i], _ = s.(string)
				}
				v = marshalAllowlist(strs)
			}
		}
		if k == "downloads_approved" {
			// The column is INTEGER 0/1; a non-bool here would store a value
			// that later fails the bool scan and bricks reads of the artifact.
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("downloads_approved must be a boolean")
			}
		}
		setClauses = append(setClauses, k+"=?")
		args = append(args, v)
	}
	setClauses = append(setClauses, "updated_at=datetime('now')")
	args = append(args, id)
	_, err := s.db.ExecContext(ctx,
		"UPDATE artifacts SET "+strings.Join(setClauses, ", ")+" WHERE id=?", args...)
	return err
}

func (s *SQLiteStore) DeleteArtifact(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM artifacts WHERE id=?", id)
	return err
}

func (s *SQLiteStore) CreateCollection(ctx context.Context, c *Collection) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO collections (id, owner_id, name) VALUES (?, ?, ?)",
		c.ID, c.OwnerID, c.Name)
	return err
}

func (s *SQLiteStore) ListCollections(ctx context.Context, ownerID int64) ([]*Collection, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, owner_id, name FROM collections WHERE owner_id=?", ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cs []*Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.Name); err != nil {
			return nil, err
		}
		cs = append(cs, &c)
	}
	return cs, rows.Err()
}

func (s *SQLiteStore) AddArtifactToCollection(ctx context.Context, artifactID, collectionID string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO artifact_collections (artifact_id, collection_id) VALUES (?, ?)",
		artifactID, collectionID)
	return err
}

func (s *SQLiteStore) RemoveArtifactFromCollection(ctx context.Context, artifactID, collectionID string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM artifact_collections WHERE artifact_id=? AND collection_id=?", artifactID, collectionID)
	return err
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint error.
// The modernc driver exposes no typed error, so match on the message.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (s *SQLiteStore) CreateTag(ctx context.Context, t *Tag) error {
	if t.Color == "" {
		t.Color = DefaultTagColor
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO tags (id, owner_id, name, color) VALUES (?, ?, ?, ?)",
		t.ID, t.OwnerID, t.Name, t.Color)
	if isUniqueViolation(err) {
		return ErrDuplicateName
	}
	return err
}

func (s *SQLiteStore) UpdateTag(ctx context.Context, ownerID int64, id string, name, color *string) (*Tag, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE tags SET name = COALESCE(?, name), color = COALESCE(?, color) WHERE id=? AND owner_id=?",
		name, color, id, ownerID)
	if isUniqueViolation(err) {
		return nil, ErrDuplicateName
	}
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	t := &Tag{ID: id, OwnerID: ownerID}
	err = s.db.QueryRowContext(ctx, "SELECT name, color FROM tags WHERE id=?", id).Scan(&t.Name, &t.Color)
	return t, err
}

func (s *SQLiteStore) DeleteTag(ctx context.Context, ownerID int64, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM tags WHERE id=? AND owner_id=?", id, ownerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) ListTags(ctx context.Context, ownerID int64) ([]*Tag, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, owner_id, name, color FROM tags WHERE owner_id=?", ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ts []*Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		ts = append(ts, &t)
	}
	return ts, rows.Err()
}

func (s *SQLiteStore) AddArtifactTag(ctx context.Context, ownerID int64, artifactID, tagID string) error {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)
		    AND EXISTS(SELECT 1 FROM tags WHERE id=? AND owner_id=?)`,
		artifactID, ownerID, tagID, ownerID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	_, err = s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO artifact_tags (artifact_id, tag_id) VALUES (?, ?)", artifactID, tagID)
	return err
}

func (s *SQLiteStore) RemoveArtifactTag(ctx context.Context, ownerID int64, artifactID, tagID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM artifact_tags WHERE artifact_id=? AND tag_id=?
		    AND artifact_id IN (SELECT id FROM artifacts WHERE owner_id=?)
		    AND tag_id IN (SELECT id FROM tags WHERE owner_id=?)`,
		artifactID, tagID, ownerID, ownerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// attachTags populates Tags on each artifact with one batched query,
// avoiding a per-artifact lookup.
func (s *SQLiteStore) attachTags(ctx context.Context, arts []*Artifact) error {
	if len(arts) == 0 {
		return nil
	}
	byID := make(map[string]*Artifact, len(arts))
	placeholders := make([]string, len(arts))
	args := make([]any, len(arts))
	for i, a := range arts {
		a.Tags = []*Tag{}
		byID[a.ID] = a
		placeholders[i] = "?"
		args[i] = a.ID
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT at.artifact_id, t.id, t.owner_id, t.name, t.color
		   FROM artifact_tags at JOIN tags t ON t.id = at.tag_id
		  WHERE at.artifact_id IN (`+strings.Join(placeholders, ",")+`)
		  ORDER BY t.name`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var artID string
		var t Tag
		if err := rows.Scan(&artID, &t.ID, &t.OwnerID, &t.Name, &t.Color); err != nil {
			return err
		}
		byID[artID].Tags = append(byID[artID].Tags, &t)
	}
	return rows.Err()
}

func (s *SQLiteStore) GetState(ctx context.Context, artifactID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT key, value FROM artifact_state WHERE artifact_id=?", artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	state := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		state[k] = v
	}
	return state, rows.Err()
}

func (s *SQLiteStore) SetState(ctx context.Context, artifactID, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifact_state (artifact_id, key, value, updated_at)
         VALUES (?, ?, ?, datetime('now'))
         ON CONFLICT(artifact_id, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		artifactID, key, value)
	return err
}

func (s *SQLiteStore) CreateShare(ctx context.Context, sh *Share) error {
	var expiresAt *string
	if sh.ExpiresAt != nil {
		str := sh.ExpiresAt.Format(time.RFC3339)
		expiresAt = &str
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO shares (id, artifact_id, public, expires_at) VALUES (?, ?, ?, ?)",
		sh.ID, sh.ArtifactID, sh.Public, expiresAt)
	return err
}

func (s *SQLiteStore) GetShare(ctx context.Context, id string) (*Share, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, artifact_id, public, expires_at FROM shares WHERE id=?", id)
	var sh Share
	var expiresAt *string
	var public int
	err := row.Scan(&sh.ID, &sh.ArtifactID, &public, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sh.Public = public == 1
	if expiresAt != nil {
		t, _ := time.Parse(time.RFC3339, *expiresAt)
		sh.ExpiresAt = &t
	}
	return &sh, nil
}

func (s *SQLiteStore) DeleteShare(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM shares WHERE id=?", id)
	return err
}
