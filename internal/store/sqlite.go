package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
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
	registerRepairMigration() // guarded, idempotent v8 repair for the 005 renumber collision
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

// toStringSlice normalizes an allowlist value arriving from a JSON PATCH
// body ([]interface{}) or from Go code ([]string).
func toStringSlice(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		return list, true
	case []interface{}:
		strs := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			strs = append(strs, s)
		}
		return strs, true
	case nil:
		return []string{}, true
	}
	return nil, false
}

// anyToTime normalizes a scanned timestamp — the modernc driver may hand back
// a time.Time, a string, or []byte depending on the column's storage.
func anyToTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t.UTC()
	case string:
		return parseTS(t)
	case []byte:
		return parseTS(string(t))
	}
	return time.Time{}
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
	// Scan timestamps as any — the modernc sqlite driver may return them as time.Time or string
	var createdAt, updatedAt any
	err := rows.Scan(&a.ID, &a.OwnerID, &a.Title, &a.SourceBlobID, &a.SourceURL, &a.Tier, &createdAt, &updatedAt, &a.DownloadsApproved, &a.ClipboardApproved)
	if err != nil {
		return nil, err
	}
	// NetworkAllowlist is hydrated separately from artifact_network_origins
	// (attachAllowlists); an artifact with no allow rows carries an empty list,
	// never nil, so JSON callers always see [].
	a.NetworkAllowlist = []string{}
	a.CreatedAt, a.UpdatedAt = anyToTime(createdAt), anyToTime(updatedAt)
	return &a, nil
}

const artifactCols = "id, owner_id, title, source_blob_id, source_url, tier, created_at, updated_at, downloads_approved, clipboard_approved"
const artifactColsA = "a.id, a.owner_id, a.title, a.source_blob_id, a.source_url, a.tier, a.created_at, a.updated_at, a.downloads_approved, a.clipboard_approved"

func (s *SQLiteStore) PutArtifact(ctx context.Context, a *Artifact) error {
	now := a.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts (id, owner_id, title, source_blob_id, source_url, tier, downloads_approved, clipboard_approved, source_text, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.OwnerID, a.Title, a.SourceBlobID, a.SourceURL, a.Tier, a.DownloadsApproved, a.ClipboardApproved, a.SourceText,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	// The allowlist passed in is the set of origins the caller has approved;
	// it lands as allow rows in the child table (exhibit-x87).
	return s.ReplaceAllowedOrigins(ctx, a.ID, a.NetworkAllowlist, "user")
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
	if err := s.attachAllowlists(ctx, []*Artifact{a}); err != nil {
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
	if err := s.attachAllowlists(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *SQLiteStore) UpdateArtifact(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	// "network_allowlist" is no longer a column: it is the artifact's allow
	// rows. Apply it through ReplaceAllowedOrigins (which leaves block rows
	// alone) and drop it from the column update below.
	if v, ok := updates["network_allowlist"]; ok {
		origins, ok := toStringSlice(v)
		if !ok {
			return fmt.Errorf("network_allowlist must be an array of strings")
		}
		if err := s.ReplaceAllowedOrigins(ctx, id, origins, "user"); err != nil {
			return err
		}
		updates = withoutKey(updates, "network_allowlist")
		if len(updates) == 0 {
			// Still bump updated_at so an allowlist-only PATCH is visible.
			_, err := s.db.ExecContext(ctx, "UPDATE artifacts SET updated_at=datetime('now') WHERE id=?", id)
			return err
		}
	}
	setClauses := make([]string, 0, len(updates)+1)
	args := make([]any, 0, len(updates)+1)
	for k, v := range updates {
		if k == "downloads_approved" || k == "clipboard_approved" {
			// These columns are INTEGER 0/1; a non-bool here would store a value
			// that later fails the bool scan and bricks reads of the artifact.
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", k)
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

// withoutKey returns a copy of m with key removed, leaving the caller's map
// untouched (handlers reuse the decoded PATCH body after the store call).
func withoutKey(m map[string]any, key string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k != key {
			out[k] = v
		}
	}
	return out
}

func (s *SQLiteStore) ListOriginDecisions(ctx context.Context, artifactID string) ([]OriginDecision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT origin, decision, source, created_at, updated_at
		   FROM artifact_network_origins WHERE artifact_id=? ORDER BY origin`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OriginDecision{}
	for rows.Next() {
		var d OriginDecision
		var createdAt, updatedAt any
		if err := rows.Scan(&d.Origin, &d.Decision, &d.Source, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		d.CreatedAt, d.UpdatedAt = anyToTime(createdAt), anyToTime(updatedAt)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) AllowedOrigins(ctx context.Context, artifactID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT origin FROM artifact_network_origins
		  WHERE artifact_id=? AND decision=? ORDER BY origin`, artifactID, DecisionAllow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SetOriginDecision(ctx context.Context, artifactID, origin, decision, source string) error {
	if decision != DecisionAllow && decision != DecisionBlock {
		return fmt.Errorf("invalid origin decision %q", decision)
	}
	// The (artifact_id, origin) primary key is what makes one decision per
	// origin an invariant; the upsert flips an existing decision in place
	// rather than creating a second, contradictory row.
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifact_network_origins (artifact_id, origin, decision, source)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(artifact_id, origin) DO UPDATE SET
		   decision=excluded.decision, source=excluded.source, updated_at=datetime('now')`,
		artifactID, origin, decision, source)
	return err
}

func (s *SQLiteStore) DeleteOriginDecision(ctx context.Context, artifactID, origin string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM artifact_network_origins WHERE artifact_id=? AND origin=?", artifactID, origin)
	return err
}

func (s *SQLiteStore) ReplaceAllowedOrigins(ctx context.Context, artifactID string, origins []string, source string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	// Only allow rows are in scope. Block rows are decisions this caller
	// doesn't know about ("don't ask again", exhibit-fr7) and survive.
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM artifact_network_origins WHERE artifact_id=? AND decision=?",
		artifactID, DecisionAllow); err != nil {
		return err
	}
	for _, o := range origins {
		if o == "" {
			continue
		}
		// An origin listed here was explicitly approved, so an allow
		// decision overrides any block row it previously carried.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO artifact_network_origins (artifact_id, origin, decision, source)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT(artifact_id, origin) DO UPDATE SET
			   decision=excluded.decision, source=excluded.source, updated_at=datetime('now')`,
			artifactID, o, DecisionAllow, source); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// attachAllowlists hydrates NetworkAllowlist on a batch of artifacts from
// their decision='allow' rows, in one query.
func (s *SQLiteStore) attachAllowlists(ctx context.Context, arts []*Artifact) error {
	if len(arts) == 0 {
		return nil
	}
	byID := make(map[string]*Artifact, len(arts))
	placeholders := make([]string, len(arts))
	args := make([]any, 0, len(arts)+1)
	for i, a := range arts {
		a.NetworkAllowlist = []string{}
		byID[a.ID] = a
		placeholders[i] = "?"
		args = append(args, a.ID)
	}
	args = append(args, DecisionAllow)
	rows, err := s.db.QueryContext(ctx,
		`SELECT artifact_id, origin FROM artifact_network_origins
		  WHERE artifact_id IN (`+strings.Join(placeholders, ",")+`) AND decision=?
		  ORDER BY origin`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var artID, origin string
		if err := rows.Scan(&artID, &origin); err != nil {
			return err
		}
		a := byID[artID]
		a.NetworkAllowlist = append(a.NetworkAllowlist, origin)
	}
	return rows.Err()
}

func (s *SQLiteStore) DeleteArtifact(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM artifacts WHERE id=?", id)
	return err
}

// blobGetter is the read side of blob.Store — the minimal seam
// BackfillSourceText needs, so the store package doesn't import blob.
type blobGetter interface {
	Get(ctx context.Context, id string) (io.ReadCloser, error)
}

// BackfillSourceText populates source_text for artifacts left over from
// before migration 010 (source_text = ''), so their pre-existing bodies
// become searchable without requiring a re-edit. It's a startup pass, not a
// migration, because the blob store isn't reachable from SQL. Safe to call
// repeatedly: rows already backfilled are excluded by the WHERE clause, and a
// row whose blob can't be read is logged and skipped rather than aborting
// the rest.
func (s *SQLiteStore) BackfillSourceText(ctx context.Context, blobs blobGetter) error {
	rows, err := s.db.QueryContext(ctx, "SELECT id, source_blob_id FROM artifacts WHERE source_text = ''")
	if err != nil {
		return err
	}
	type pending struct{ id, blobID string }
	var toFill []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.blobID); err != nil {
			rows.Close()
			return err
		}
		toFill = append(toFill, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, p := range toFill {
		rc, err := blobs.Get(ctx, p.blobID)
		if err != nil {
			slog.WarnContext(ctx, "backfill source_text: read blob failed",
				slog.String("artifact_id", p.id), slog.String("blob_id", p.blobID), slog.String("err", err.Error()))
			continue
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			slog.WarnContext(ctx, "backfill source_text: blob read error",
				slog.String("artifact_id", p.id), slog.String("err", err.Error()))
			continue
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE artifacts SET source_text = ? WHERE id = ?", string(body), p.id); err != nil {
			return fmt.Errorf("backfill source_text for %s: %w", p.id, err)
		}
	}
	if len(toFill) > 0 {
		slog.InfoContext(ctx, "backfilled artifact source_text", slog.Int("count", len(toFill)))
	}
	return nil
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

func (s *SQLiteStore) SetAgentKey(ctx context.Context, k *AgentKey) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_keys (owner_id, provider, model, key_ciphertext, updated_at)
         VALUES (?, ?, ?, ?, datetime('now'))
         ON CONFLICT(owner_id) DO UPDATE SET provider=excluded.provider,
             model=excluded.model, key_ciphertext=excluded.key_ciphertext,
             updated_at=excluded.updated_at`,
		k.OwnerID, k.Provider, k.Model, k.KeyCiphertext)
	return err
}

func (s *SQLiteStore) GetAgentKey(ctx context.Context, ownerID int64) (*AgentKey, error) {
	k := &AgentKey{OwnerID: ownerID}
	var updated string
	err := s.db.QueryRowContext(ctx,
		"SELECT provider, model, key_ciphertext, updated_at FROM agent_keys WHERE owner_id=?",
		ownerID).Scan(&k.Provider, &k.Model, &k.KeyCiphertext, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	k.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)
	return k, nil
}

func (s *SQLiteStore) DeleteAgentKey(ctx context.Context, ownerID int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM agent_keys WHERE owner_id=?", ownerID)
	return err
}

func (s *SQLiteStore) SaveTranscript(ctx context.Context, artifactID, sessionID, messagesJSON string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_transcripts (artifact_id, session_id, messages, updated_at)
         VALUES (?, ?, ?, datetime('now'))
         ON CONFLICT(artifact_id, session_id) DO UPDATE SET
             messages=excluded.messages, updated_at=excluded.updated_at`,
		artifactID, sessionID, messagesJSON)
	return err
}

func (s *SQLiteStore) ListTranscripts(ctx context.Context, artifactID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT session_id, messages FROM agent_transcripts WHERE artifact_id=? ORDER BY updated_at",
		artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var sid, msgs string
		if err := rows.Scan(&sid, &msgs); err != nil {
			return nil, err
		}
		out[sid] = msgs
	}
	return out, rows.Err()
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
