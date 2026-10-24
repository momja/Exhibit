package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrConnectionLimit = errors.New("connection limit reached")
var ErrConnectionQuota = errors.New("connection daily request quota reached")
var ErrConnectionOverlap = errors.New("this artifact already has a connection for that origin; disconnect the existing grant before attaching another")
var ErrConnectionSharing = errors.New("review connections and explicitly enable sharing before publishing this artifact or sharing its state")

type connectionSharingQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func validateConnectionSharing(ctx context.Context, q connectionSharingQuery, ownerID int64, artifactID string) error {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_connection_data_policies WHERE artifact_id=? AND viewer_id=? AND share_with_recipients=0`, artifactID, ownerID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrConnectionSharing
	}
	return nil
}
func (s *SQLiteStore) ValidateConnectionSharing(ctx context.Context, ownerID int64, artifactID string) error {
	if err := s.ownsArtifact(ctx, ownerID, artifactID); err != nil {
		return err
	}
	return validateConnectionSharing(ctx, s.db, ownerID, artifactID)
}

func jsonMarshalStrings(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	return string(raw), err
}
func jsonUnmarshalStrings(raw string, values *[]string) error {
	return json.Unmarshal([]byte(raw), values)
}

// ConnectionRecord is a persistence record, never an API response. Even an
// accidental JSON encoding cannot disclose its recipe or sealed credential.
type ConnectionRecord struct {
	ID              string     `json:"id"`
	OwnerID         int64      `json:"-"`
	Name            string     `json:"name"`
	Provider        string     `json:"provider"`
	Kind            string     `json:"kind"`
	Engine          string     `json:"-"`
	ExternalRef     string     `json:"-"`
	RecipeJSON      string     `json:"-"`
	EncryptedSecret string     `json:"-"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

type ConnectionGrantRecord struct {
	ArtifactID          string
	ConnectionID        string
	ViewerID            int64
	PermittedEgress     []string
	ShareWithRecipients bool
	CreatedAt           time.Time
}

// ConnectionDataPolicy retains saved-data provenance for sharing and private
// render controls after a connection is removed. Origins is legacy destination
// metadata and supplies no network authority. It contains no credential,
// private recipe path, or provider response.
type ConnectionDataPolicy struct {
	ArtifactID          string   `json:"artifact_id"`
	ConnectionID        string   `json:"connection_id"`
	ViewerID            int64    `json:"viewer_id"`
	Origins             []string `json:"origins"`
	ShareWithRecipients bool     `json:"share_with_recipients"`
}

// ListConnectionDataPolicies selects a viewer's retained policies and the
// artifact owner's explicitly shared policies. Artifact access is authorized
// separately by the caller, as for ListConnectionGrants; public renders may
// need an owner's shared policy without an authenticated viewer account.
func (s *SQLiteStore) ListConnectionDataPolicies(ctx context.Context, viewerID int64, artifactID string) ([]ConnectionDataPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.artifact_id,p.connection_id,p.viewer_id,p.origins_json,p.share_with_recipients
 FROM artifact_connection_data_policies p JOIN artifacts a ON a.id=p.artifact_id
 WHERE p.artifact_id=? AND (p.viewer_id=? OR (p.share_with_recipients=1 AND p.viewer_id=a.owner_id))
 ORDER BY p.viewer_id,p.connection_id`, artifactID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConnectionDataPolicy{}
	for rows.Next() {
		var p ConnectionDataPolicy
		var raw string
		if err := rows.Scan(&p.ArtifactID, &p.ConnectionID, &p.ViewerID, &raw, &p.ShareWithRecipients); err != nil {
			return nil, err
		}
		if err := jsonUnmarshalStrings(raw, &p.Origins); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RetainSharedConnectionDataPolicies records custody before a recipient can
// receive provider data using the owner's shared credential. The recipient
// can save that response in their own state: an owner's later reset must not
// release those copies. Repeated receipt can only narrow a retained copy.
func (s *SQLiteStore) RetainSharedConnectionDataPolicies(ctx context.Context, viewerID int64, artifactID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO artifact_connection_data_policies(artifact_id,viewer_id,connection_id,origins_json,share_with_recipients)
 SELECT p.artifact_id,?,p.connection_id,p.origins_json,0
 FROM artifact_connection_data_policies p JOIN artifacts a ON a.id=p.artifact_id
 WHERE p.artifact_id=? AND p.viewer_id=a.owner_id AND p.share_with_recipients=1 AND a.owner_id!=?
 AND EXISTS(SELECT 1 FROM share_grants WHERE share_grants.artifact_id=a.id AND share_grants.recipient_id=?)
 ON CONFLICT(artifact_id,viewer_id,connection_id) DO UPDATE SET origins_json=(
  SELECT json_group_array(value) FROM (
   SELECT value FROM json_each(artifact_connection_data_policies.origins_json)
   INTERSECT SELECT value FROM json_each(excluded.origins_json) ORDER BY value))`, viewerID, artifactID, viewerID, viewerID)
	return err
}

// connectionDataPolicySelect is shared by grant creation and reset. A grant's
// trusted UI consent and its durable data policy always commit together.
const connectionDataPolicySelect = `SELECT g.artifact_id,g.viewer_id,g.connection_id,
 (SELECT json_group_array(value) FROM (
   SELECT value FROM json_each(c.recipe_json,'$.origins')
   UNION SELECT value FROM json_each(g.permitted_egress_json) ORDER BY value
 )),g.share_with_recipients
 FROM artifact_connection_grants g JOIN connections c ON c.id=g.connection_id`

// ClearConnectionData clears a viewer's effective saved state and releases
// only their obsolete data policies in one transaction. Active grants retain
// their policies, and recipients cannot clear shared state for everybody or
// release the owner's shared policies. No standalone policy-delete operation
// is exposed: removing restrictions while retaining old data is unsafe.
func (s *SQLiteStore) ClearConnectionData(ctx context.Context, viewerID int64, artifactID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var artifact Artifact
	err = tx.QueryRowContext(ctx, `SELECT artifacts.owner_id,artifacts.share_state_mode FROM artifacts WHERE artifacts.id=? AND `+readableByViewer, artifactID, viewerID, viewerID).Scan(&artifact.OwnerID, &artifact.ShareStateMode)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if artifact.ShareStateMode == ShareStateShared && artifact.OwnerID != viewerID {
		return ErrNotFound
	}
	principal := artifact.StatePrincipal(ViewerID(viewerID))
	if _, err = tx.ExecContext(ctx, `DELETE FROM artifact_state WHERE artifact_id=? AND user_id=?`, artifactID, int64(principal)); err != nil {
		return err
	}
	if int64(principal) == artifact.OwnerID {
		// Version restore must not resurrect cleared provider data after its
		// data restrictions have been released. Snapshots contain owner state.
		if _, err = tx.ExecContext(ctx, `UPDATE artifact_versions SET state_json='{}' WHERE artifact_id=? AND state_json IS NOT NULL`, artifactID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM artifact_connection_data_policies
 WHERE artifact_id=? AND viewer_id=? AND NOT EXISTS (
  SELECT 1 FROM artifact_connection_grants g JOIN connections c ON c.id=g.connection_id
  WHERE g.artifact_id=artifact_connection_data_policies.artifact_id
   AND g.connection_id=artifact_connection_data_policies.connection_id AND c.status='active'
   AND (g.viewer_id=artifact_connection_data_policies.viewer_id OR
    (g.share_with_recipients=1 AND g.viewer_id=(SELECT owner_id FROM artifacts WHERE id=g.artifact_id))))`, artifactID, viewerID); err != nil {
		return err
	}
	// Preserve the existing policy if present; the UI may have narrowed it
	// since the grant was first made. Rebuild only a missing active policy.
	if _, err = tx.ExecContext(ctx, `INSERT INTO artifact_connection_data_policies(artifact_id,viewer_id,connection_id,origins_json,share_with_recipients) `+connectionDataPolicySelect+`
 WHERE g.artifact_id=? AND g.viewer_id=? AND c.status='active'
 ON CONFLICT(artifact_id,viewer_id,connection_id) DO NOTHING`, artifactID, viewerID); err != nil {
		return err
	}
	return tx.Commit()
}

type ConnectionUsageArtifact struct {
	ArtifactID          string   `json:"artifact_id"`
	Title               string   `json:"title"`
	ViewerID            int64    `json:"viewer_id"`
	PermittedEgress     []string `json:"permitted_egress"`
	ShareWithRecipients bool     `json:"share_with_recipients"`
}

func (s *SQLiteStore) ConnectionArtifacts(ctx context.Context, ownerID int64, id string) ([]ConnectionUsageArtifact, error) {
	if _, err := s.GetConnection(ctx, ownerID, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT g.artifact_id,a.title,g.viewer_id,g.permitted_egress_json,g.share_with_recipients FROM artifact_connection_grants g JOIN artifacts a ON a.id=g.artifact_id JOIN connections c ON c.id=g.connection_id WHERE c.id=? AND c.owner_id=? ORDER BY a.title,g.viewer_id`, id, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConnectionUsageArtifact{}
	for rows.Next() {
		var a ConnectionUsageArtifact
		var raw string
		if err := rows.Scan(&a.ArtifactID, &a.Title, &a.ViewerID, &raw, &a.ShareWithRecipients); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &a.PermittedEgress); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

const connectionColumns = `id,owner_id,name,provider,kind,engine,external_ref,recipe_json,encrypted_secret,status,created_at,last_used_at`

func scanConnection(row interface{ Scan(...any) error }) (*ConnectionRecord, error) {
	var c ConnectionRecord
	var created, last any
	err := row.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Provider, &c.Kind, &c.Engine, &c.ExternalRef, &c.RecipeJSON, &c.EncryptedSecret, &c.Status, &created, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt = anyToTime(created)
	if last != nil {
		t := anyToTime(last)
		c.LastUsedAt = &t
	}
	return &c, nil
}

// CreateConnection enforces the kind's resolved entitlement in the insertion
// transaction. A zero limit refuses all; only -1 means unlimited.
func (s *SQLiteStore) CreateConnection(ctx context.Context, c *ConnectionRecord, limit int) error {
	if c.Kind == "vendor" && c.Engine == "" {
		// Compatibility for callers creating the pre-engine vendor shape.
		c.Engine = "nango"
	}
	if limit < -1 {
		return ErrConnectionLimit
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM connections WHERE owner_id=? AND kind=? AND (status='active' OR (kind='vendor' AND status='reconnect'))`, c.OwnerID, c.Kind).Scan(&count); err != nil {
		return err
	}
	if limit >= 0 && count >= limit {
		return ErrConnectionLimit
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO connections(id,owner_id,name,provider,kind,engine,external_ref,recipe_json,encrypted_secret,status) VALUES(?,?,?,?,?,?,?,?,?,'active')`, c.ID, c.OwnerID, c.Name, c.Provider, c.Kind, c.Engine, c.ExternalRef, c.RecipeJSON, c.EncryptedSecret)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListConnections(ctx context.Context, ownerID int64) ([]*ConnectionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+connectionColumns+` FROM connections WHERE owner_id=? ORDER BY created_at DESC,id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ConnectionRecord{}
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetConnection(ctx context.Context, ownerID int64, id string) (*ConnectionRecord, error) {
	return scanConnection(s.db.QueryRowContext(ctx, `SELECT `+connectionColumns+` FROM connections WHERE id=? AND owner_id=?`, id, ownerID))
}

// GetGrantedConnection reads only a grant already held by this viewer or an
// owner's explicit opt-in grant. Callers must separately authorize artifact
// viewing; a grant never confers access to the artifact itself. Reconnect rows
// stay visible so an expired credential cannot become an unauthenticated relay.
func (s *SQLiteStore) GetGrantedConnection(ctx context.Context, viewerID int64, artifactID, id string) (*ConnectionRecord, error) {
	return scanConnection(s.db.QueryRowContext(ctx, `SELECT `+prefixedConnectionColumns()+` FROM connections c WHERE c.id=? AND c.status IN ('active','reconnect') AND EXISTS (
 SELECT 1 FROM artifact_connection_grants g JOIN artifacts a ON a.id=g.artifact_id
 WHERE g.connection_id=c.id AND g.artifact_id=? AND (g.viewer_id=? OR (g.share_with_recipients=1 AND g.viewer_id=a.owner_id)))`, id, artifactID, viewerID))
}

func prefixedConnectionColumns() string {
	return `c.id,c.owner_id,c.name,c.provider,c.kind,c.engine,c.external_ref,c.recipe_json,c.encrypted_secret,c.status,c.created_at,c.last_used_at`
}

// UpdateVendorConnectionRef lazily pins a legacy connection's external account
// ID after its encrypted credential is read. It never changes the engine or
// replaces a previously pinned account, even if operator routing has changed.
func (s *SQLiteStore) UpdateVendorConnectionRef(ctx context.Context, id, engine, externalRef string) error {
	if !validVendorEngine(engine) || externalRef == "" || len(externalRef) > 1024 || strings.ContainsAny(externalRef, "\r\n\x00") {
		return errors.New("invalid managed connection reference")
	}
	return s.exactlyOneRow(ctx, `UPDATE connections SET external_ref=? WHERE id=? AND kind='vendor' AND engine=? AND (external_ref='' OR external_ref=?)`, externalRef, id, engine, externalRef)
}

func (s *SQLiteStore) MarkVendorConnectionReconnect(ctx context.Context, ownerID int64, id string) error {
	return s.exactlyOneRow(ctx, `UPDATE connections SET status='reconnect' WHERE id=? AND owner_id=? AND kind='vendor' AND status IN ('active','reconnect')`, id, ownerID)
}

func (s *SQLiteStore) UpdateConnectionSecret(ctx context.Context, ownerID int64, id, encrypted string) error {
	return s.exactlyOneRow(ctx, `UPDATE connections SET encrypted_secret=? WHERE id=? AND owner_id=? AND status='active'`, encrypted, id, ownerID)
}

func (s *SQLiteStore) TouchConnection(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE connections SET last_used_at=datetime('now') WHERE id=? AND status='active'`, id)
	return err
}
func (s *SQLiteStore) DeleteConnection(ctx context.Context, ownerID int64, id string) error {
	return s.exactlyOneRow(ctx, `DELETE FROM connections WHERE id=? AND owner_id=?`, id, ownerID)
}

func (s *SQLiteStore) PutConnectionGrant(ctx context.Context, g ConnectionGrantRecord) error {
	raw, err := jsonMarshalStrings(g.PermittedEgress)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A shared owner grant applies to every recipient, including viewers who
	// already attached their own account. Check both directions atomically so
	// attachment order can never choose whose credential a schedule spends.
	var overlap bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts a
	 JOIN connections nc ON nc.id=? AND nc.owner_id=?
	 JOIN json_each(nc.recipe_json,'$.origins') no
	 JOIN artifact_connection_grants og ON og.artifact_id=a.id AND og.connection_id!=nc.id
	 JOIN connections oc ON oc.id=og.connection_id AND oc.status='active'
	 JOIN json_each(oc.recipe_json,'$.origins') oo ON oo.value=no.value
	 WHERE a.id=? AND (og.viewer_id=? OR (og.share_with_recipients=1 AND og.viewer_id=a.owner_id) OR (?=1 AND ?=a.owner_id)))`, g.ConnectionID, g.ViewerID, g.ArtifactID, g.ViewerID, g.ShareWithRecipients, g.ViewerID).Scan(&overlap)
	if err != nil {
		return err
	}
	if overlap {
		return ErrConnectionOverlap
	}
	// The connection must belong to the grant's viewer. Only an artifact owner
	// may opt their own credential into use by share recipients.
	res, err := tx.ExecContext(ctx, `INSERT INTO artifact_connection_grants(artifact_id,connection_id,viewer_id,permitted_egress_json,share_with_recipients)
 SELECT ?,c.id,?,?,? FROM connections c JOIN artifacts a ON a.id=?
 WHERE c.id=? AND c.owner_id=? AND c.status='active' AND (?=0 OR a.owner_id=?)
 AND (?=1 OR (a.share_state_mode!='shared' AND a.share_link=0))
 ON CONFLICT(artifact_id,connection_id,viewer_id) DO UPDATE SET permitted_egress_json=excluded.permitted_egress_json,share_with_recipients=excluded.share_with_recipients`, g.ArtifactID, g.ViewerID, raw, g.ShareWithRecipients, g.ArtifactID, g.ConnectionID, g.ViewerID, g.ShareWithRecipients, g.ViewerID, g.ShareWithRecipients)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO artifact_connection_data_policies(artifact_id,viewer_id,connection_id,origins_json,share_with_recipients) `+connectionDataPolicySelect+`
 WHERE g.artifact_id=? AND g.connection_id=? AND g.viewer_id=?
 ON CONFLICT(artifact_id,viewer_id,connection_id) DO UPDATE SET origins_json=excluded.origins_json,share_with_recipients=excluded.share_with_recipients`, g.ArtifactID, g.ConnectionID, g.ViewerID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListConnectionGrants(ctx context.Context, viewerID int64, artifactID string) ([]ConnectionGrantRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.artifact_id,g.connection_id,g.viewer_id,g.permitted_egress_json,g.share_with_recipients,g.created_at
 FROM artifact_connection_grants g JOIN artifacts a ON a.id=g.artifact_id JOIN connections c ON c.id=g.connection_id
 WHERE g.artifact_id=? AND c.status IN ('active','reconnect') AND (g.viewer_id=? OR (g.share_with_recipients=1 AND g.viewer_id=a.owner_id)) ORDER BY g.viewer_id=? DESC,g.connection_id`, artifactID, viewerID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConnectionGrantRecord{}
	for rows.Next() {
		var g ConnectionGrantRecord
		var raw string
		var created any
		if err := rows.Scan(&g.ArtifactID, &g.ConnectionID, &g.ViewerID, &raw, &g.ShareWithRecipients, &created); err != nil {
			return nil, err
		}
		if err := jsonUnmarshalStrings(raw, &g.PermittedEgress); err != nil {
			return nil, err
		}
		g.CreatedAt = anyToTime(created)
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) DeleteConnectionGrant(ctx context.Context, viewerID int64, artifactID, id string) error {
	return s.exactlyOneRow(ctx, `DELETE FROM artifact_connection_grants WHERE artifact_id=? AND connection_id=? AND viewer_id=?`, artifactID, id, viewerID)
}

func (s *SQLiteStore) ConnectionRequestsToday(ctx context.Context, id string, now time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT requests FROM connection_usage WHERE connection_id=? AND day=?`, id, now.UTC().Format("2006-01-02")).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// ReserveConnectionRequest charges before any network I/O, atomically even
// across process restarts. Failed provider calls consume quota too.
func (s *SQLiteStore) ReserveConnectionRequest(ctx context.Context, id string, limit int, now time.Time) error {
	if limit <= 0 {
		return nil
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO connection_usage(connection_id,day,requests) VALUES(?,?,1)
 ON CONFLICT(connection_id,day) DO UPDATE SET requests=requests+1 WHERE requests<?`, id, now.UTC().Format("2006-01-02"), limit)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrConnectionQuota
	}
	return err
}
func (s *SQLiteStore) GetConnectionCache(ctx context.Context, id, hash string, now time.Time) (string, error) {
	var sealed string
	// Bound retrieval before SQLite copies an old oversized encrypted record
	// into Go. New entries contain at most 4 MiB decoded body plus metadata;
	// base64 for the JSON body and encryption accounts for the extra room.
	err := s.db.QueryRowContext(ctx, `SELECT encrypted_response FROM connection_cache WHERE connection_id=? AND request_hash=? AND expires_at>? AND length(encrypted_response)<=?`, id, hash, now.Unix(), 12<<20).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return sealed, err
}
func (s *SQLiteStore) PutConnectionCache(ctx context.Context, id, hash, sealed string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO connection_cache(connection_id,request_hash,encrypted_response,expires_at) VALUES(?,?,?,?) ON CONFLICT(connection_id,request_hash) DO UPDATE SET encrypted_response=excluded.encrypted_response,expires_at=excluded.expires_at`, id, hash, sealed, expires.Unix())
	return err
}
func (s *SQLiteStore) ListIdleVendorConnections(ctx context.Context, before time.Time) ([]*ConnectionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+connectionColumns+` FROM connections WHERE kind='vendor' AND COALESCE(last_used_at,created_at)<?`, before.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ConnectionRecord{}
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
