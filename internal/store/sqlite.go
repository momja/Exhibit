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
	"unicode"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/momja/Exhibit/internal/origin"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type SQLiteStore struct {
	db *sql.DB

	// Held across a blob write and the transaction that references it, and
	// across the deletion queue's recheck-and-unlink, so the two cannot
	// interleave into a referenced blob with no bytes (bloblock.go).
	blobLocks    blobLocks
	storageLocks blobLocks
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
	// Three databases exist in the wild whose ledger disagrees with this
	// repo's numbering; migration_repair.go says which and why. All of it is a
	// no-op on a database with no such history — nothing below defines schema.
	ctx := context.Background()
	registerRepairMigrations() // guarded, idempotent repairs for the version collisions at 5 and 11
	if err := rewindReusedVersion13(ctx, s.db); err != nil {
		return fmt.Errorf("rewind version 13: %w", err)
	}
	registerOriginNormalizationMigration() // v23: normalize/collapse pre-av-i7hd origin rows
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	start := time.Now()
	if err := goose.Up(s.db, "migrations"); err != nil {
		return err
	}
	if err := restoreRewoundApprovals(ctx, s.db); err != nil {
		return fmt.Errorf("restore rewound approvals: %w", err)
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

// extra receives columns appended to artifactCols by a listing's joins.
func scanArtifact(rows interface{ Scan(...any) error }, extra ...any) (*Artifact, error) {
	var a Artifact
	// Scan timestamps as any — the modernc sqlite driver may return them as time.Time or string
	var createdAt, updatedAt any
	columns := []any{&a.ID, &a.OwnerID, &a.Title, &a.SourceBlobID, &a.SourceURL, &a.Tier, &createdAt, &updatedAt, &a.DownloadsApproved, &a.ClipboardApproved, &a.LinksApproved, &a.CameraApproved, &a.MicrophoneApproved, &a.GeolocationApproved, &a.WidgetBlobID, &a.ShareStateMode, &a.ShareGrantCount, &a.SharePublicLink, &a.VersionSeq}
	err := rows.Scan(append(columns, extra...)...)
	if err != nil {
		return nil, err
	}
	// NetworkAllowlist is hydrated separately from artifact_network_origins
	// (attachAllowlists); an artifact with no allow rows carries an empty list,
	// never nil, so JSON callers always see [].
	a.SourceRevision = ContentRevision(a.SourceBlobID)
	a.WidgetRevision = ContentRevision(a.WidgetBlobID)
	a.NetworkAllowlist = []string{}
	a.CreatedAt, a.UpdatedAt = anyToTime(createdAt), anyToTime(updatedAt)
	return &a, nil
}

// share_state_mode is read here and written only through UpdateArtifact's
// validated path (av-6xjd): PutArtifact leaves it to the column's DEFAULT, so
// a new artifact is always 'own' and the mode is a decision somebody made
// rather than one an ingest inherited.
//
// Sharing indicators are derived from indexed tables on every read (Exh-grj9),
// so badges follow link/grant writes and cascades without rollup triggers.
const artifactCols = "id, owner_id, title, source_blob_id, source_url, tier, created_at, updated_at, downloads_approved, clipboard_approved, links_approved, camera_approved, microphone_approved, geolocation_approved, widget_blob_id, share_state_mode, (SELECT COUNT(*) FROM share_grants g WHERE g.artifact_id = artifacts.id), EXISTS (SELECT 1 FROM share_links l WHERE l.artifact_id = artifacts.id), (SELECT MAX(seq) FROM artifact_versions WHERE artifact_id = artifacts.id)"
const artifactColsA = "a.id, a.owner_id, a.title, a.source_blob_id, a.source_url, a.tier, a.created_at, a.updated_at, a.downloads_approved, a.clipboard_approved, a.links_approved, a.camera_approved, a.microphone_approved, a.geolocation_approved, a.widget_blob_id, a.share_state_mode, (SELECT COUNT(*) FROM share_grants g WHERE g.artifact_id = a.id), EXISTS (SELECT 1 FROM share_links l WHERE l.artifact_id = a.id), (SELECT MAX(seq) FROM artifact_versions WHERE artifact_id = a.id)"

func (s *SQLiteStore) PutArtifact(ctx context.Context, a *Artifact) error {
	now := a.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO artifacts (id, owner_id, title, source_blob_id, source_url, tier, downloads_approved, clipboard_approved, links_approved, camera_approved, microphone_approved, geolocation_approved, widget_blob_id, source_text, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.OwnerID, a.Title, a.SourceBlobID, a.SourceURL, a.Tier, a.DownloadsApproved, a.ClipboardApproved, a.LinksApproved, a.CameraApproved, a.MicrophoneApproved, a.GeolocationApproved, a.WidgetBlobID, a.SourceText,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	// Version 1 already exists: the artifacts_initial_version trigger wrote it
	// from the row just inserted. An ingest that was not an ordinary one says so
	// here (an agent that created the artifact), the single time that row is
	// touched after creation.
	if p := a.Provenance; p.Origin != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE artifact_versions SET origin = ?, message = ?, session_id = ? WHERE artifact_id = ? AND seq = 1`,
			p.Origin, p.Message, p.SessionID, a.ID); err != nil {
			return err
		}
	}
	// The allowlist passed in is the set of origins the caller has approved;
	// it lands as allow rows in the child table (exhibit-x87).
	if err := replaceAllowedOriginsTx(ctx, tx, a.ID, a.NetworkAllowlist, "user"); err != nil {
		return err
	}
	return tx.Commit()
}

// ownsArtifact reports whether ownerID owns artifactID. It is the one place
// the EXISTS predicate is written for callers whose statement can't carry it
// (multi-statement transactions, upserts), so those still fail with
// ErrNotFound rather than silently touching another owner's rows.
func (s *SQLiteStore) ownsArtifact(ctx context.Context, ownerID int64, artifactID string) error {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)",
		artifactID, ownerID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// GetArtifact resolves an artifact within one owner's library. A row that
// belongs to someone else is reported exactly like a row that never existed —
// (nil, nil), which handlers render as 404 — so the API answers no questions
// about ids outside the caller's library (av-ep8k).
func (s *SQLiteStore) GetArtifact(ctx context.Context, ownerID int64, id string) (*Artifact, error) {
	return s.getArtifactWhere(ctx,
		"SELECT "+artifactCols+" FROM artifacts WHERE id=? AND owner_id=?", id, ownerID)
}

// GetArtifactUnscoped is the render/share read path — see the Store interface
// comment for why it carries no owner and why the name is this loud.
func (s *SQLiteStore) GetArtifactUnscoped(ctx context.Context, id string) (*Artifact, error) {
	return s.getArtifactWhere(ctx,
		"SELECT "+artifactCols+" FROM artifacts WHERE id=?", id)
}

// readableByViewer is the deny-by-default counterpart to ownedArtifact
// (av-lrae): the artifact's owner, or an account a grant row names on it.
//
// It is written here, once, and used by exactly one method. That is the point
// — it is deliberately *not* folded into ownedArtifact, which every
// artifact-child query interpolates: widening that predicate would hand a
// recipient the DELETE, the body rewrite and the origin decisions along with
// the read, and it would do so silently, because no call site mentions it.
//
// `recipient_id = ?` never matches the anonymous link's NULL, which is correct
// and not incidental: a link is authorized by holding its id at /s/:shareID,
// never by being logged in as somebody. And the two parameters are the same
// viewer id twice, so a zero value — a caller that forgot to resolve a
// principal — matches no owner and no grant and fails closed.
const readableByViewer = `(artifacts.owner_id = ?
      OR EXISTS (SELECT 1 FROM share_grants
                  WHERE share_grants.artifact_id = artifacts.id AND share_grants.recipient_id = ?))`

// GetArtifactReadableBy resolves an artifact this viewer may read — see the
// Store interface comment for why reaching a shared artifact is a second
// accessor rather than a wider predicate. A viewer with no grant reads exactly
// like an artifact that does not exist.
func (s *SQLiteStore) GetArtifactReadableBy(ctx context.Context, viewerID ViewerID, id string) (*Artifact, error) {
	return s.getArtifactWhere(ctx,
		"SELECT "+artifactCols+" FROM artifacts WHERE id=? AND "+readableByViewer,
		id, int64(viewerID), int64(viewerID))
}

func (s *SQLiteStore) getArtifactWhere(ctx context.Context, query string, args ...any) (*Artifact, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
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

// ftsMatchExpression turns a user's search box input into an FTS5 MATCH
// expression that treats every character as literal text. Users type things
// like `<script>` or `a:b`, all of which are FTS5 query *syntax* — passed
// through raw they make SQLite return a syntax error instead of results.
//
// Each whitespace-separated token becomes a quoted phrase (inner quotes
// doubled, per FTS5's string literal rules) with a trailing `*`, so
// prefix matching still works exactly as it did for plain words. Tokens that
// carry no indexable characters at all (`<`, `>`, `-` on their own) are
// dropped: an empty phrase is itself a syntax error, and such a token could
// never match anything anyway. An input that reduces to nothing yields "",
// which callers read as "no search filter".
func ftsMatchExpression(userQuery string) string {
	var phrases []string
	for _, token := range strings.Fields(userQuery) {
		if !hasIndexableChar(token) {
			continue
		}
		phrases = append(phrases, `"`+strings.ReplaceAll(token, `"`, `""`)+`"*`)
	}
	return strings.Join(phrases, " ")
}

// hasIndexableChar reports whether a token contains anything SQLite's default
// unicode61 tokenizer would index — letters, digits, or other non-punctuation
// symbols. Everything else is a separator that tokenizes away to nothing.
func hasIndexableChar(token string) bool {
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func (s *SQLiteStore) ListArtifacts(ctx context.Context, opts ListOptions) ([]*Artifact, error) {
	query, args := listArtifactsQuery(opts)

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

func listArtifactsQuery(opts ListOptions) (string, []any) {
	var (
		query  string
		args   []any
		wheres []string
	)

	// Always use alias 'a' for artifacts so WHERE clauses and ORDER BY are consistent.
	// owner_id leads every variant: the listing is one owner's library, and
	// the search index is deliberately filtered *after* the MATCH so an FTS5
	// hit on someone else's artifact is dropped rather than counted.
	matchExpr := ftsMatchExpression(opts.Query)
	if matchExpr != "" {
		// Use a subquery to avoid ambiguous column names from the FTS5 JOIN.
		// The FTS5 MATCH expression works on the table name in a subquery.
		query = `SELECT ` + artifactColsA + ` FROM artifacts a
            WHERE a.owner_id = ?
            AND a.rowid IN (SELECT rowid FROM artifacts_fts WHERE artifacts_fts MATCH ?)`
		args = append(args, opts.OwnerID, matchExpr)
	} else {
		query = `SELECT ` + artifactColsA + ` FROM artifacts a WHERE a.owner_id = ?`
		args = append(args, opts.OwnerID)
	}

	// Each axis normally matches any value for compatibility with API callers.
	// Gallery selections require every value, including stable collection IDs.
	addMembership := func(values []string, table, join, column string) {
		groups := [][]string{values}
		if opts.MatchAllFilters {
			groups = nil
			for _, value := range values {
				groups = append(groups, []string{value})
			}
		}
		for _, group := range groups {
			if len(group) == 0 {
				continue
			}
			placeholders := make([]string, len(group))
			for i, value := range group {
				placeholders[i] = "?"
				args = append(args, value)
			}
			args = append(args, opts.OwnerID)
			wheres = append(wheres, `a.id IN (SELECT membership.artifact_id FROM `+table+` membership
				JOIN `+join+` choice ON choice.id = membership.`+column+` WHERE choice.name IN (`+strings.Join(placeholders, ",")+`) AND choice.owner_id = ?)`)
		}
	}
	addMembership(opts.Tags, "artifact_tags", "tags", "tag_id")
	addMembership(opts.Collections, "artifact_collections", "collections", "collection_id")
	for _, id := range opts.CollectionIDs {
		wheres = append(wheres, `a.id IN (SELECT membership.artifact_id FROM artifact_collections membership
			JOIN collections choice ON choice.id = membership.collection_id WHERE choice.id = ? AND choice.owner_id = ?)`)
		args = append(args, id, opts.OwnerID)
	}

	for _, w := range wheres {
		query += " AND " + w
	}
	if opts.SharedOnly {
		query += " AND " + artifactHasShares
	}

	return artifactListPage(query, args, opts.Limit, opts.Offset)
}

func artifactListPage(query string, args []any, limit, offset int) (string, []any) {
	if limit <= 0 {
		limit = 50
	}

	// Paging needs a total order even when several artifacts share a timestamp.
	// SQLite's default timestamps and RFC3339 (including subsecond clone
	// timestamps) sort differently as text. Compare their actual instants.
	query += " ORDER BY julianday(a.created_at) DESC, a.id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	return query, args
}

// ApprovalColumns are the first-use capability approval columns: the
// per-artifact "yes, this tool may do that" decisions the host frame persists
// (architecture.md §6). They are named once, here, because two layers have to
// agree about them — the API rejects a non-bool in a PATCH body with a 400
// (internal/api/artifacts.go) and the UPDATE below refuses to store one — and a
// capability added later that reached only one of those two lists would be
// accepted by the handler and then stored as a value no later read can scan.
//
// The order is the order the UI shows them in: downloads, clipboard, links,
// camera, microphone, location.
var ApprovalColumns = []string{
	"downloads_approved",
	"clipboard_approved",
	"links_approved",
	"camera_approved",
	"microphone_approved",
	"geolocation_approved",
}

// approvalArtifactColumns is ApprovalColumns as a set, for the per-key check in
// UpdateArtifact.
var approvalArtifactColumns = func() map[string]bool {
	m := make(map[string]bool, len(ApprovalColumns))
	for _, c := range ApprovalColumns {
		m[c] = true
	}
	return m
}()

// updatableArtifactColumns is the allowlist UpdateArtifact checks its caller's
// keys against. "network_allowlist" is deliberately absent: it never reaches
// the generic loop below (handled and stripped first), so listing it here
// would just be a second place that claim could be made and forgotten.
// Everything else naming an artifacts column — id, owner_id, source_blob_id,
// widget_blob_id, created_at, updated_at and tags_text — is
// deliberately excluded: an update map here is a handler-decoded PATCH body
// (updateArtifact decodes into map[string]any and passes the keys through), so
// an unvalidated key is a column the *caller* chose. widget_blob_id matters
// most of those: a caller who could set it would repoint their card at any
// blob id they can name, including one belonging to another owner's artifact,
// since blob ids are global and nothing downstream re-checks whose widget the
// id was.
//
// The server's own writer therefore does not come through here at all — see
// CommitVersion, the only writer of source_blob_id and widget_blob_id. This map
// answers "what may a PATCH body write", and that is the only question it
// answers.
var updatableArtifactColumns = map[string]bool{
	"title":                true,
	"tier":                 true,
	"source_url":           true,
	"source_text":          true,
	"downloads_approved":   true,
	"clipboard_approved":   true,
	"links_approved":       true,
	"camera_approved":      true,
	"microphone_approved":  true,
	"geolocation_approved": true,
	// Whose state a recipient writes (av-6xjd). It became writable here when
	// the owner got a control for it; the value is checked below rather than
	// merely allowed, because unlike a title this column is read as an enum by
	// the render path, and an unrecognized value there is a branch nobody
	// wrote.
	"share_state_mode": true,
}

func (s *SQLiteStore) UpdateArtifact(ctx context.Context, ownerID int64, id string, updates map[string]any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := updateArtifactTx(ctx, tx, ownerID, id, updates); err != nil {
		return err
	}
	return tx.Commit()
}

func updateArtifactTx(ctx context.Context, tx *sql.Tx, ownerID int64, id string, updates map[string]any) error {
	if len(updates) > 0 {
		var found string
		err := tx.QueryRowContext(ctx, "SELECT id FROM artifacts WHERE id = ? AND owner_id = ?", id, ownerID).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
	}
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
		if err := replaceAllowedOriginsTx(ctx, tx, id, origins, "user"); err != nil {
			return err
		}
		updates = withoutKey(updates, "network_allowlist")
		if len(updates) == 0 {
			// Still bump updated_at so an allowlist-only PATCH is visible.
			return execOwnedTx(ctx, tx,
				"UPDATE artifacts SET updated_at=datetime('now') WHERE id=? AND owner_id=?", id, ownerID)
		}
	}
	setClauses := make([]string, 0, len(updates)+1)
	args := make([]any, 0, len(updates)+2)
	for k, v := range updates {
		if !updatableArtifactColumns[k] {
			// k becomes a bare SQL identifier below (k+"=?"), never a bound
			// parameter, so an unvalidated key is a column the caller chose —
			// up to and including owner_id or id. This map is a handler-decoded
			// PATCH body (internal/api/artifacts.go), so that caller is
			// whoever sent the request.
			return fmt.Errorf("update artifact: %q: %w", k, ErrNotUpdatable)
		}
		if approvalArtifactColumns[k] {
			// These columns are INTEGER 0/1; a non-bool here would store a value
			// that later fails the bool scan and bricks reads of the artifact.
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", k)
			}
		}
		if k == "share_state_mode" {
			// The API rejects a bad mode with a 400 (internal/api/artifacts.go)
			// and this rejects the write, for the reason the approval columns
			// are checked twice: the handler's list is a user-facing rule and
			// this is the same rule as a store invariant, closed to any future
			// caller. A mode nothing recognizes would decide whose state rows a
			// recipient writes by falling off the end of a switch.
			mode, ok := v.(string)
			if !ok || !ValidShareStateMode(mode) {
				return fmt.Errorf("share_state_mode must be %q or %q", ShareStateOwn, ShareStateShared)
			}
		}
		setClauses = append(setClauses, k+"=?")
		args = append(args, v)
	}
	setClauses = append(setClauses, "updated_at=datetime('now')")
	args = append(args, id, ownerID)
	return execOwnedTx(ctx, tx,
		"UPDATE artifacts SET "+strings.Join(setClauses, ", ")+" WHERE id=? AND owner_id=?", args...)
}

// execOwned runs a statement whose WHERE clause already carries the owner
// predicate and translates "matched nothing" into ErrNotFound — which is what
// a cross-tenant id and an unknown id both look like from here.
func (s *SQLiteStore) execOwned(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
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

func execOwnedTx(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	res, err := tx.ExecContext(ctx, query, args...)
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

// ownedArtifact checks an artifact-child row's owner, separately from
// readableByViewer: a grant must never widen a child mutation.
//
// This correlated check is efficient only when the outer query pins
// <outerTable>.artifact_id = ? (or a single child row by primary key, as share
// link lookups do). Enumerating an owner's library must instead drive from
// artifacts(owner_id), then look up child rows by artifact_id.
//
// outerTable must be a known table or alias from our SQL; unknown identifiers
// panic before they can be spliced into SQL. The predicate binds one owner ID.
func ownedArtifact(outerTable string) string {
	switch outerTable {
	case "artifact_network_origins", "artifact_collections", "artifact_tags",
		"artifact_state", "share_links", "share_grants", "artifact_shares",
		"v", "artifact_versions", "agent_transcripts", "artifact_version_assets":
	default:
		panic("ownedArtifact: unknown outer table or alias: " + outerTable)
	}
	return `EXISTS (SELECT 1 FROM artifacts WHERE artifacts.id = ` + outerTable + `.artifact_id AND artifacts.owner_id = ?)`
}

func listOriginDecisionsQuery() string {
	return `SELECT origin, decision, source, created_at, updated_at
		   FROM artifact_network_origins WHERE artifact_id=? AND ` + ownedArtifact("artifact_network_origins") + `
		  ORDER BY origin`
}

func (s *SQLiteStore) ListOriginDecisions(ctx context.Context, ownerID int64, artifactID string) ([]OriginDecision, error) {
	rows, err := s.db.QueryContext(ctx,
		listOriginDecisionsQuery(), artifactID, ownerID)
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

func (s *SQLiteStore) AllowedOrigins(ctx context.Context, ownerID int64, artifactID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT origin FROM artifact_network_origins
		  WHERE artifact_id=? AND decision=? AND `+ownedArtifact("artifact_network_origins")+`
		  ORDER BY origin`, artifactID, DecisionAllow, ownerID)
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

// SetOriginDecision widens or narrows what an artifact may reach, so it is
// the sharpest thing in this file to leave unscoped: the owner check runs
// before the upsert, and a foreign artifact gets ErrNotFound rather than a
// new allow row. One decision per *origin* (docs/architecture.md §3.3) only
// holds if what lands in the row is an origin; a near-duplicate spelling
// would otherwise split one decision into several (av-i7hd).
func (s *SQLiteStore) SetOriginDecision(ctx context.Context, ownerID int64, artifactID, o, decision, source string) error {
	if decision != DecisionAllow && decision != DecisionBlock {
		return fmt.Errorf("invalid origin decision %q", decision)
	}
	if err := s.ownsArtifact(ctx, ownerID, artifactID); err != nil {
		return err
	}
	normalized, err := origin.NormalizeOrigin(o)
	if err != nil {
		return fmt.Errorf("invalid origin %q: %w", o, err)
	}
	o = normalized
	// The (artifact_id, origin) primary key is what makes one decision per
	// origin an invariant; the upsert flips an existing decision in place
	// rather than creating a second, contradictory row.
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO artifact_network_origins (artifact_id, origin, decision, source)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(artifact_id, origin) DO UPDATE SET
		   decision=excluded.decision, source=excluded.source, updated_at=datetime('now')`,
		artifactID, o, decision, source)
	return err
}

// DeleteOriginDecision is idempotent like the state deletes: removing a
// decision that isn't there is what the caller asked for either way. Owner
// scoping only guarantees it can never remove someone else's. The origin is
// normalized when it can be, so a caller may spell it the way the user typed
// it, but an unnormalizable value is still passed through verbatim — that is
// exactly how a pre-normalization legacy row is deleted.
func (s *SQLiteStore) DeleteOriginDecision(ctx context.Context, ownerID int64, artifactID, o string) error {
	if normalized, _ := origin.NormalizeOrigin(o); normalized != "" {
		o = normalized
	}
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM artifact_network_origins WHERE artifact_id=? AND origin=? AND "+ownedArtifact("artifact_network_origins"),
		artifactID, o, ownerID)
	return err
}

func (s *SQLiteStore) ReplaceAllowedOrigins(ctx context.Context, ownerID int64, artifactID string, origins []string, source string) error {
	if err := s.ownsArtifact(ctx, ownerID, artifactID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	if err := replaceAllowedOriginsTx(ctx, tx, artifactID, origins, source); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceAllowedOriginsTx(ctx context.Context, tx *sql.Tx, artifactID string, origins []string, source string) error {
	// Only allow rows are in scope. Block rows are decisions this caller
	// doesn't know about ("don't ask again", exhibit-fr7) and survive.
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM artifact_network_origins WHERE artifact_id=? AND decision=?",
		artifactID, DecisionAllow); err != nil {
		return err
	}
	for _, o := range origins {
		// Defense in depth behind the API's validation (av-i7hd): the store's
		// own invariant is that a row is an origin, so a caller that skipped
		// normalization can't write a path-bearing or duplicate spelling here.
		// Unlike the API this can't 400, so anything that doesn't validate
		// cleanly is dropped rather than widening the CSP with something nobody
		// approved — a path-bearing value is refused, not truncated to its host
		// (salvaging those is migration 23's job, via origin.SalvageOrigin). The
		// rejected value is untrusted data, so the log carries fixed context and
		// a fixed reason only.
		normalized, err := origin.NormalizeOrigin(o)
		if err != nil {
			slog.Warn("dropping unusable allowlist origin",
				slog.String("artifact_id", artifactID), slog.String("reason", "invalid_origin"))
			continue
		}
		o = normalized
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
	return nil
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

// ReplaceArtifactSnapshot commits a refetch as one unit. A body naming freshly
// minted markup asset URLs must never become visible without those asset rows.
// Superseded bytes are queued only after every reference has been switched.
func (s *SQLiteStore) ReplaceArtifactSnapshot(ctx context.Context, ownerID int64, id string, snapshot ArtifactSnapshot) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var old string
	err = tx.QueryRowContext(ctx,
		"SELECT source_blob_id FROM artifacts WHERE id=? AND owner_id=?",
		id, ownerID).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	change := VersionChange{Provenance: snapshot.Provenance, ExpectedSourceRevision: snapshot.ExpectedSourceRevision}
	if snapshot.SourceBlobID != "" {
		change.BodyBlobID = &snapshot.SourceBlobID
		change.SourceText = &snapshot.SourceText
	}
	if _, err := commitVersionTx(ctx, tx, ownerID, id, change); err != nil {
		return nil, err
	}
	var superseded []string
	if snapshot.Assets != nil {
		oldAssets, err := replaceArtifactAssets(ctx, tx, id, snapshot.GenerationID, snapshot.Assets)
		if err != nil {
			return nil, err
		}
		superseded = append(superseded, oldAssets...)
	}
	queued, err := enqueueUnreferencedBlobs(ctx, tx, superseded...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return queued, nil
}

// DeleteArtifact removes the artifact — its tags, collections, shares, origin
// decisions, state and versions going with it by ON DELETE CASCADE — and
// returns the blob ids it enqueued for deletion, for the caller to drain
// (blobqueue.go).
//
// Reading the blob ids, dropping the row and enqueuing what is now
// unreferenced happen in one transaction, which is what makes the intent to
// delete those bytes durable at the same instant the last reference to them
// disappears. Nothing outside this transaction could reconstruct that list
// afterwards: once the rows are gone, nothing names the blobs.
func (s *SQLiteStore) DeleteArtifact(ctx context.Context, ownerID int64, id string) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	var body, widget string
	err = tx.QueryRowContext(ctx,
		"SELECT source_blob_id, widget_blob_id FROM artifacts WHERE id=? AND owner_id=?",
		id, ownerID).Scan(&body, &widget)
	if errors.Is(err, sql.ErrNoRows) {
		// Another owner's artifact and one that never existed are the same
		// answer here, as everywhere else on this interface.
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Read before the delete: ON DELETE CASCADE takes the version and asset rows
	// with the artifact, and once they are gone nothing names those blobs
	// (av-20fk). An older version's body is a blob no other row names.
	versionBlobs, err := versionBlobIDs(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	assetBlobs, err := assetBlobIDs(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM artifacts WHERE id=? AND owner_id=?", id, ownerID); err != nil {
		return nil, err
	}
	condemned := append([]string{body, widget}, versionBlobs...)
	queued, err := enqueueUnreferencedBlobs(ctx, tx, append(condemned, assetBlobs...)...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return queued, nil
}

// blobGetter is the read side of blob.Store — the minimal seam
// BackfillSourceText needs, so the store package doesn't import blob.
type blobGetter interface {
	Get(ctx context.Context, id string) (io.ReadCloser, error)
}

// BackfillSourceText populates source_text for artifacts left over from
// before migration 010 (an empty source_text), so their pre-existing bodies
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
		if _, err := s.db.ExecContext(ctx, "UPDATE artifacts SET source_text = ? WHERE id = ?", ExtractSearchText(string(body)), p.id); err != nil {
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

// AddArtifactToCollection requires *both* rows to be the caller's — the same
// double-EXISTS gate AddArtifactTag uses. Checking only one would let a
// caller file a foreign artifact onto their own shelf (leaking its existence)
// or file their own artifact onto someone else's.
func (s *SQLiteStore) AddArtifactToCollection(ctx context.Context, ownerID int64, artifactID, collectionID string) error {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)
		    AND EXISTS(SELECT 1 FROM collections WHERE id=? AND owner_id=?)`,
		artifactID, ownerID, collectionID, ownerID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	_, err = s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO artifact_collections (artifact_id, collection_id) VALUES (?, ?)",
		artifactID, collectionID)
	return err
}

func (s *SQLiteStore) RemoveArtifactFromCollection(ctx context.Context, ownerID int64, artifactID, collectionID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM artifact_collections WHERE artifact_id=? AND collection_id=?
		    AND `+ownedArtifact("artifact_collections")+`
		    AND collection_id IN (SELECT id FROM collections WHERE owner_id=?)`,
		artifactID, collectionID, ownerID, ownerID)
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
		    AND `+ownedArtifact("artifact_tags")+`
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

// The four state methods carry both principals the Store interface describes:
// ownerID authorizes reaching the artifact (the ownedArtifact predicate), and
// userID selects whose rows. Every query below therefore filters on user_id in
// addition to the owner predicate — the two are ANDed, never substituted for
// one another.

func getStateQuery() string {
	return "SELECT key, value FROM artifact_state WHERE artifact_id=? AND user_id=? AND " + ownedArtifact("artifact_state")
}

func (s *SQLiteStore) GetState(ctx context.Context, ownerID OwnerID, artifactID string, userID ViewerID) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		getStateQuery(),
		artifactID, int64(userID), int64(ownerID))
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

func (s *SQLiteStore) SetState(ctx context.Context, ownerID OwnerID, artifactID string, userID ViewerID, key, value string) error {
	if err := s.ownsArtifact(ctx, int64(ownerID), artifactID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifact_state (artifact_id, user_id, key, value, updated_at)
         VALUES (?, ?, ?, ?, datetime('now'))
         ON CONFLICT(artifact_id, user_id, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		artifactID, int64(userID), key, value)
	return err
}

// DeleteState removes one key's row for one viewer. A key that was never stored
// is not an error: the row is gone either way, which is the only thing the
// caller asked for.
func (s *SQLiteStore) DeleteState(ctx context.Context, ownerID OwnerID, artifactID string, userID ViewerID, key string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM artifact_state WHERE artifact_id=? AND user_id=? AND key=? AND "+ownedArtifact("artifact_state"),
		artifactID, int64(userID), key, int64(ownerID))
	return err
}

// ClearState removes every state row this viewer holds on the artifact, leaving
// the artifact itself — body, origin decisions, capability approvals —
// untouched, and leaving any other viewer's rows alone. "Erase all my state" is
// the operation the state inspector offers; erasing *someone else's* state is a
// different, deliberate act that no route grants today.
func (s *SQLiteStore) ClearState(ctx context.Context, ownerID OwnerID, artifactID string, userID ViewerID) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM artifact_state WHERE artifact_id=? AND user_id=? AND "+ownedArtifact("artifact_state"),
		artifactID, int64(userID), int64(ownerID))
	return err
}

// The four state methods a VIEWER reaches (av-v991) — see the Store interface
// for why they take one principal and resolve the target themselves.
//
// Each is authorization plus resolution, then a delegation to the owner-scoped
// method above. Delegating rather than reimplementing is the point: there is
// still exactly one statement per operation, so the owner's path and a
// grantee's cannot drift into writing different rows. The owner predicate the
// delegate carries is satisfied by construction, because stateTarget read that
// owner off the very artifact row it authorized against.

// stateTarget resolves the artifact a viewer is addressing state on, and
// answers the two things the methods below need from it: the owner whose
// predicate the delegate will check, and whose rows this viewer is on.
//
// Authorization is readableByViewer, deliberately not ownedArtifact — that is
// the whole of what makes these a parallel path rather than a widening. An
// artifact the viewer neither owns nor holds a grant on is ErrNotFound, which
// is what a nonexistent id answers too.
func (s *SQLiteStore) stateTarget(ctx context.Context, viewerID ViewerID, artifactID string) (OwnerID, ViewerID, error) {
	var a Artifact
	err := s.db.QueryRowContext(ctx,
		"SELECT artifacts.owner_id, artifacts.share_state_mode FROM artifacts WHERE artifacts.id=? AND "+readableByViewer,
		artifactID, int64(viewerID), int64(viewerID)).Scan(&a.OwnerID, &a.ShareStateMode)
	if err == sql.ErrNoRows {
		return 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, err
	}
	return OwnerID(a.OwnerID), a.StatePrincipal(viewerID), nil
}

// GetStateAsViewer reads the rows this viewer is on. An artifact they cannot
// reach reads empty rather than erroring, matching GetState's answer for
// another owner's id: absence and inaccessibility must look the same.
func (s *SQLiteStore) GetStateAsViewer(ctx context.Context, viewerID ViewerID, artifactID string) (map[string]string, error) {
	owner, principal, err := s.stateTarget(ctx, viewerID, artifactID)
	if errors.Is(err, ErrNotFound) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return s.GetState(ctx, owner, artifactID, principal)
}

// SetStateAsViewer is the write av-awr4 found missing. Without it 'own' mode —
// the default — leaves a recipient reading state they can never change, so a
// shared tool forgets everything the moment its frame reloads.
func (s *SQLiteStore) SetStateAsViewer(ctx context.Context, viewerID ViewerID, artifactID, key, value string, version ...int) error {
	owner, principal, err := s.stateTarget(ctx, viewerID, artifactID)
	if err != nil {
		return err
	}
	if len(version) > 0 {
		return s.mutateVersionedState(ctx, owner, artifactID, principal, &key, &value, version[0])
	}
	return s.SetState(ctx, owner, artifactID, principal, key, value)
}

// DeleteStateAsViewer removes one key from the rows this viewer is on. An
// unreachable artifact holds nothing of theirs, so it is a silent no-op — the
// idempotent contract the owner-scoped deletes already keep, extended to the
// wider "cannot reach it" case for the same reason: the caller asked for a key
// not to exist, and there it does not.
func (s *SQLiteStore) DeleteStateAsViewer(ctx context.Context, viewerID ViewerID, artifactID, key string, version ...int) error {
	owner, principal, err := s.stateTarget(ctx, viewerID, artifactID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(version) > 0 {
		return s.mutateVersionedState(ctx, owner, artifactID, principal, &key, nil, version[0])
	}
	return s.DeleteState(ctx, owner, artifactID, principal, key)
}

// ClearStateAsViewer erases everything this viewer holds on the artifact —
// *theirs*, which under ShareStateOwn is their own rows and under
// ShareStateShared is the board they are playing on. Either way it is the
// erase-all av-q0ub defined: one viewer's state, never the artifact's, and
// never another viewer's.
func (s *SQLiteStore) ClearStateAsViewer(ctx context.Context, viewerID ViewerID, artifactID string, version ...int) error {
	owner, principal, err := s.stateTarget(ctx, viewerID, artifactID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(version) > 0 {
		return s.mutateVersionedState(ctx, owner, artifactID, principal, nil, nil, version[0])
	}
	return s.ClearState(ctx, owner, artifactID, principal)
}

func (s *SQLiteStore) SetAgentKey(ctx context.Context, k *AgentKey) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_keys (owner_id, provider, model, thinking_level, key_ciphertext, updated_at)
         VALUES (?, ?, ?, ?, ?, datetime('now'))
         ON CONFLICT(owner_id) DO UPDATE SET provider=excluded.provider,
             model=excluded.model, thinking_level=excluded.thinking_level,
             key_ciphertext=excluded.key_ciphertext,
             updated_at=excluded.updated_at`,
		k.OwnerID, k.Provider, k.Model, k.ThinkingLevel, k.KeyCiphertext)
	return err
}

func (s *SQLiteStore) GetAgentKey(ctx context.Context, ownerID int64) (*AgentKey, error) {
	k := &AgentKey{OwnerID: ownerID}
	var updated string
	err := s.db.QueryRowContext(ctx,
		"SELECT provider, model, thinking_level, key_ciphertext, updated_at FROM agent_keys WHERE owner_id=?",
		ownerID).Scan(&k.Provider, &k.Model, &k.ThinkingLevel, &k.KeyCiphertext, &updated)
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

// RecordAgentUsage appends one usage event to the metering ledger (av-2yws).
// recorded_at is left to the schema's default so rows carry the database's
// clock — the sums below compare against it, and two clocks is one more way
// for a period boundary to disagree with itself.
func (s *SQLiteStore) RecordAgentUsage(ctx context.Context, u AgentUsage) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_usage (owner_id, session_id, provider, model, paid_by, source,
		     input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_micros)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.OwnerID, u.SessionID, u.Provider, u.Model, u.PaidBy, u.Source,
		u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, u.CostMicros)
	return err
}

const usageSumColumns = `COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
    COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(cache_write_tokens), 0),
    COALESCE(SUM(cost_micros), 0)`

// usageTotals runs one aggregate over the ledger. A row-less table sums to
// zero rather than no-row, so every caller gets a usable total.
func (s *SQLiteStore) usageTotals(ctx context.Context, where string, args ...any) (UsageTotals, error) {
	var t UsageTotals
	err := s.db.QueryRowContext(ctx,
		"SELECT "+usageSumColumns+" FROM agent_usage WHERE "+where, args...).
		Scan(&t.InputTokens, &t.OutputTokens, &t.CacheReadTokens, &t.CacheWriteTokens, &t.CostMicros)
	return t, err
}

// datetimeLayout is the shape SQLite's datetime('now') stores, and so the
// shape a since-bound has to be formatted in to compare as a string.
const datetimeLayout = "2006-01-02 15:04:05"

// OwnerAgentSpend is one owner's platform-paid agent spend since a time —
// what the per-owner budget meters (av-99f4). BYO-key rows are the owner's
// own money and guardrail rows are the operator's overhead, so neither
// counts against the owner's budget.
func (s *SQLiteStore) OwnerAgentSpend(ctx context.Context, ownerID int64, since time.Time) (UsageTotals, error) {
	return s.usageTotals(ctx,
		"owner_id=? AND paid_by='platform' AND source != 'guardrail' AND recorded_at >= ?",
		ownerID, since.UTC().Format(datetimeLayout))
}

// SessionAgentSpend is one session's own agent spend — what the per-session
// ceiling meters — owner-scoped like every sum (av-ep8k). Guardrail rows are
// excluded for the same reason they are excluded from the owner's budget:
// screening is the operator's cost of doing business, never a property of
// one conversation, and only the instance ceiling bounds it.
func (s *SQLiteStore) SessionAgentSpend(ctx context.Context, ownerID int64, sessionID string) (UsageTotals, error) {
	return s.usageTotals(ctx, "owner_id=? AND session_id=? AND source != 'guardrail'", ownerID, sessionID)
}

// InstanceAgentSpend is every platform-paid row since a time, guardrail
// included whatever session it came from: the operator's total exposure,
// what the instance ceiling meters.
func (s *SQLiteStore) InstanceAgentSpend(ctx context.Context, since time.Time) (UsageTotals, error) {
	return s.usageTotals(ctx, "paid_by='platform' AND recorded_at >= ?",
		since.UTC().Format(datetimeLayout))
}

// CreateGrant grants access to one named account. A nonpositive recipient returns
// ErrInvalidRecipient before any insert, so forgetting it cannot mint a public link.
func (s *SQLiteStore) CreateGrant(ctx context.Context, ownerID int64, artifactID string, recipientID int64) (*Grant, error) {
	if recipientID <= 0 {
		return nil, ErrInvalidRecipient
	}
	if err := s.ownsArtifact(ctx, ownerID, artifactID); err != nil {
		return nil, err
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO share_grants (artifact_id, recipient_id) VALUES (?, ?)",
		artifactID, recipientID)
	if isUniqueViolation(err) {
		return nil, ErrDuplicateShare
	}
	if err != nil {
		return nil, err
	}
	return &Grant{ArtifactID: artifactID, RecipientID: recipientID}, nil
}

// CreateAnonymousLink explicitly publishes the artifact through /s/:id.
// Uniqueness is enforced by the schema, just as it is for grants.
func (s *SQLiteStore) CreateAnonymousLink(ctx context.Context, ownerID int64, artifactID string) (*Share, error) {
	if err := s.ownsArtifact(ctx, ownerID, artifactID); err != nil {
		return nil, err
	}
	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO share_links (id, artifact_id) VALUES (?, ?)", id, artifactID)
	if isUniqueViolation(err) {
		return nil, ErrDuplicateShare
	}
	if err != nil {
		return nil, err
	}
	return &Share{ID: id, ArtifactID: artifactID}, nil
}

// GetShare resolves a link owned through its artifact. Grants have no ids.
func (s *SQLiteStore) GetShare(ctx context.Context, ownerID int64, id string) (*Share, error) {
	return s.getShareWhere(ctx, getShareQuery(), id, ownerID)
}

func getShareQuery() string {
	return `SELECT id, artifact_id FROM share_links
		  WHERE id=? AND ` + ownedArtifact("share_links")
}

// GetAnonymousShareUnscoped resolves only a link, with no owner check. The
// unscoped read surface stays inside internal/render; grants have no link id.
func (s *SQLiteStore) GetAnonymousShareUnscoped(ctx context.Context, id string) (*Share, error) {
	return s.getShareWhere(ctx,
		"SELECT id, artifact_id FROM share_links WHERE id=?", id)
}

func (s *SQLiteStore) getShareWhere(ctx context.Context, query string, args ...any) (*Share, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	var sh Share
	err := row.Scan(&sh.ID, &sh.ArtifactID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sh, nil
}

// DeleteShare revokes only an anonymous link owned by the caller. Grant ids,
// missing ids and links owned by someone else return ErrNotFound.
func (s *SQLiteStore) DeleteShare(ctx context.Context, ownerID int64, id string) error {
	return s.execOwned(ctx, deleteShareQuery(), id, ownerID)
}

func deleteShareQuery() string {
	return "DELETE FROM share_links WHERE id=? AND " + ownedArtifact("share_links")
}

// DeleteGrant revokes one (artifact, recipient) grant. Missing grants and
// artifacts outside this owner's library both return ErrNotFound, like DeleteShare.
func (s *SQLiteStore) DeleteGrant(ctx context.Context, ownerID int64, artifactID string, recipientID int64) error {
	return s.execOwned(ctx,
		"DELETE FROM share_grants WHERE artifact_id=? AND recipient_id=? AND "+ownedArtifact("share_grants"),
		artifactID, recipientID, ownerID)
}

func listArtifactSharesQuery() string {
	return `SELECT id, artifact_id, recipient_id FROM (
		 SELECT id, artifact_id, NULL AS recipient_id, NULL AS created_at FROM share_links
		 UNION ALL
		 SELECT NULL, artifact_id, recipient_id, created_at FROM share_grants
		 ) AS artifact_shares WHERE artifact_id = ? AND ` + ownedArtifact("artifact_shares") + `
		 ORDER BY recipient_id IS NOT NULL, created_at, recipient_id`
}

// ListArtifactShares enumerates one artifact's shares for its owner (av-6xjd)
// — the link, and every grant with the account it names.
//
// The rows and the accounts they name are two queries rather than a LEFT JOIN,
// which is the shape attachTags and attachAllowlists already use here. The
// join would need a second, all-nullable scan of userColumns to survive the
// anonymous link's missing partner — a parallel copy of a projection whose
// whole point (userScan) is that there is only one of it. Two statements, both
// bounded by the artifact's share count, cost less than that duplication.
//
// Links come first explicitly, then grants in creation order. Grants created
// in the same millisecond tie-break by recipient id (Exh-grj9).
func (s *SQLiteStore) ListArtifactShares(ctx context.Context, ownerID int64, artifactID string) ([]ArtifactShare, error) {
	rows, err := s.db.QueryContext(ctx,
		listArtifactSharesQuery(), artifactID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ArtifactShare{}
	for rows.Next() {
		var sh ArtifactShare
		var linkID sql.NullString
		var recipient sql.NullInt64
		if err := rows.Scan(&linkID, &sh.ArtifactID, &recipient); err != nil {
			return nil, err
		}
		sh.ID = linkID.String
		if recipient.Valid {
			id := recipient.Int64
			sh.RecipientID = &id
		}
		out = append(out, sh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.attachShareRecipients(ctx, out)
}

// attachShareRecipients fills in the account each grant names, in one query
// over the ids the listing collected. A recipient row that has vanished leaves
// Recipient nil rather than failing the listing: the FK cascade means that
// should not happen, and an owner whose share panel refuses to render because
// of one dangling id has lost the ability to revoke the others.
func (s *SQLiteStore) attachShareRecipients(ctx context.Context, shares []ArtifactShare) error {
	placeholders := []string{}
	args := []any{}
	for _, sh := range shares {
		if sh.RecipientID != nil {
			placeholders = append(placeholders, "?")
			args = append(args, *sh.RecipientID)
		}
	}
	if len(placeholders) == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE id IN ("+strings.Join(placeholders, ",")+")", args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := map[int64]*User{}
	for rows.Next() {
		var sc userScan
		if err := rows.Scan(sc.dest()...); err != nil {
			return err
		}
		u := sc.user()
		byID[u.ID] = u
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range shares {
		if shares[i].RecipientID != nil {
			shares[i].Recipient = byID[*shares[i].RecipientID]
		}
	}
	return nil
}

// ReplaceAnonymousLink rotates an artifact's public link in one transaction —
// see the Store interface for why the two halves must not be two requests.
//
// The delete is unconditional and matches nothing when there is no link, so
// "replace" and "mint" are the same call and a caller need not know which case
// it is in. A duplicate is still possible in principle (two rotations racing)
// and is reported as ErrDuplicateShare rather than swallowed: it means the row
// this transaction did not see is the one somebody else just minted.
func (s *SQLiteStore) ReplaceAnonymousLink(ctx context.Context, ownerID int64, artifactID, newID string) (*Share, error) {
	if err := s.ownsArtifact(ctx, ownerID, artifactID); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	if _, err := tx.ExecContext(ctx,
		"DELETE FROM share_links WHERE artifact_id=?", artifactID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO share_links (id, artifact_id) VALUES (?, ?)",
		newID, artifactID); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicateShare
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Share{ID: newID, ArtifactID: artifactID}, nil
}
