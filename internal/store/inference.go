package store

import (
	"context"
	"database/sql"
	"time"
)

// Artifact inference (Exh-zue3): the store's half of "who may cause an AI
// call, and what did those calls cost".
//
// Two decisions gate a call. The artifact's own approval, inference_approved,
// is the owner's and lives on the artifacts row with the other first-use
// approvals. The viewer's consent to spend their own allowance lives here, one
// row per (artifact, viewer), because a named recipient pays for their own
// calls and so has a decision of their own to make. The owner's consent is the
// approval itself and never a row: an owner who approved their tool has said
// yes to both questions in one prompt.
//
// Spend lands in agent_usage (migration 056) so the shared ceilings keep
// reading one sum; the queries below are the inference-only views of it.

// InferenceUsage is one artifact's inference spend by one payer, per kind,
// over a period — a row of the Settings usage table.
type InferenceUsage struct {
	ArtifactID string
	// Title is empty when the artifact has since been deleted or is no longer
	// readable by the payer; the spend happened all the same.
	Title  string
	Kind   string
	PaidBy string
	Calls  int64
	UsageTotals
}

// InferencePermission is one artifact a viewer has allowed to use AI at their
// expense: their own artifacts with inference_approved set, and other owners'
// artifacts they consented to and can still read.
type InferencePermission struct {
	ArtifactID string
	Title      string
	// Owned is true for the viewer's own artifact, where revoking means
	// clearing inference_approved, and false for a shared one, where it means
	// deleting the consent row.
	Owned bool
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func costSourceOrDefault(v string) string {
	if v == "" {
		return CostSourceCatalog
	}
	return v
}

// OwnerInferenceSpend is one payer's platform-paid inference spend since a
// time — the inference share of what OwnerAgentSpend already counts (that sum
// includes inference rows, which is the shared-limit rule).
func (s *SQLiteStore) OwnerInferenceSpend(ctx context.Context, ownerID int64, since time.Time) (UsageTotals, error) {
	return s.usageTotals(ctx,
		"owner_id=? AND paid_by='platform' AND source='artifact_inference' AND recorded_at >= ?",
		ownerID, since.UTC().Format(datetimeLayout))
}

// InferenceUsageByArtifact groups one payer's inference rows by artifact and
// kind. Whoever paid sees it, including for an artifact somebody else owns;
// the owner of that artifact never sees a recipient's rows, because those were
// the recipient's money. The title is joined only where the payer can still
// read the artifact.
func (s *SQLiteStore) InferenceUsageByArtifact(ctx context.Context, payerID int64, since time.Time) ([]InferenceUsage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.artifact_id, COALESCE((SELECT a.title FROM artifacts a WHERE a.id = u.artifact_id AND
		         (a.owner_id = ? OR EXISTS (SELECT 1 FROM share_grants g WHERE g.artifact_id = a.id AND g.recipient_id = ?))), ''),
		       u.kind, u.paid_by, COUNT(*), `+usageSumColumns+`
		  FROM agent_usage u
		 WHERE u.owner_id = ? AND u.source = 'artifact_inference' AND u.recorded_at >= ?
		 GROUP BY u.artifact_id, u.kind, u.paid_by
		 ORDER BY COUNT(*) DESC, u.artifact_id, u.kind`,
		payerID, payerID, payerID, since.UTC().Format(datetimeLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InferenceUsage
	for rows.Next() {
		var u InferenceUsage
		var artifactID, kind sql.NullString
		if err := rows.Scan(&artifactID, &u.Title, &kind, &u.PaidBy, &u.Calls,
			&u.InputTokens, &u.OutputTokens, &u.CacheReadTokens, &u.CacheWriteTokens, &u.CostMicros); err != nil {
			return nil, err
		}
		u.ArtifactID, u.Kind = artifactID.String, kind.String
		out = append(out, u)
	}
	return out, rows.Err()
}

// HasInferenceConsent reports whether a viewer has consented to spending their
// own allowance on an artifact. Only meaningful for a non-owner; the API asks
// the owner's approval instead.
func (s *SQLiteStore) HasInferenceConsent(ctx context.Context, viewerID ViewerID, artifactID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM artifact_inference_consents WHERE artifact_id=? AND viewer_id=?`,
		artifactID, int64(viewerID)).Scan(&n)
	return n > 0, err
}

// SetInferenceConsent records or withdraws a viewer's consent. The artifact
// must be readable by the viewer — their own or granted to them — so a viewer
// cannot leave a consent row on an artifact they cannot see. A missing or
// unreadable artifact is ErrNotFound, like every other viewer write.
func (s *SQLiteStore) SetInferenceConsent(ctx context.Context, viewerID ViewerID, artifactID string, allowed bool) error {
	if !allowed {
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM artifact_inference_consents WHERE artifact_id=? AND viewer_id=?`, artifactID, int64(viewerID))
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO artifact_inference_consents (artifact_id, viewer_id)
		SELECT id, ? FROM artifacts WHERE id=? AND `+readableByViewer+`
		ON CONFLICT (artifact_id, viewer_id) DO NOTHING`,
		int64(viewerID), artifactID, int64(viewerID), int64(viewerID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		ok, err := s.HasInferenceConsent(ctx, viewerID, artifactID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
	}
	return nil
}

// ListInferencePermissions lists every artifact allowed to use AI at this
// viewer's expense: owned artifacts with the approval set, then shared ones
// the viewer consented to and still holds a grant for. A consent whose grant
// was revoked is not listed and cannot be used (the endpoint re-resolves
// readability), and it is removed with the artifact.
func (s *SQLiteStore) ListInferencePermissions(ctx context.Context, viewerID ViewerID) ([]InferencePermission, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, 1 FROM artifacts WHERE owner_id=? AND inference_approved=1
		UNION ALL
		SELECT a.id, a.title, 0 FROM artifact_inference_consents c
		  JOIN artifacts a ON a.id = c.artifact_id
		 WHERE c.viewer_id=? AND a.owner_id != ?
		   AND EXISTS (SELECT 1 FROM share_grants g WHERE g.artifact_id = a.id AND g.recipient_id = ?)
		ORDER BY 3 DESC, 2 COLLATE NOCASE, 1`,
		int64(viewerID), int64(viewerID), int64(viewerID), int64(viewerID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InferencePermission
	for rows.Next() {
		var p InferencePermission
		if err := rows.Scan(&p.ArtifactID, &p.Title, &p.Owned); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
