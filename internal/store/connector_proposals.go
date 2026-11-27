package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/momja/Exhibit/internal/origin"
)

var ErrConnectionDuplicate = errors.New("a connection already exists for this credential destination")
var ErrConnectorProposalLimit = errors.New("this artifact already has 8 pending connector proposals")

// ConnectorProposalRecord contains public metadata and a durable decision,
// never credentials, artifact authority, or permission to make network calls.
type ConnectorProposalRecord struct {
	ID, ArtifactID, Name, Origin, Header, Auth, Reason, State, ConnectionID string
	OwnerID                                                                 int64
}

const connectorProposalColumns = `id,owner_id,artifact_id,name,origin,header,auth,reason,state,connection_id`

func scanConnectorProposal(row interface{ Scan(...any) error }) (*ConnectorProposalRecord, error) {
	var p ConnectorProposalRecord
	var connectionID sql.NullString
	err := row.Scan(&p.ID, &p.OwnerID, &p.ArtifactID, &p.Name, &p.Origin, &p.Header, &p.Auth, &p.Reason, &p.State, &connectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	p.ConnectionID = connectionID.String
	return &p, err
}

// lockProposalArtifact checks current ownership while acquiring SQLite's
// writer lock before any decision reads. Competing approvals, denials, normal
// creates and account/artifact deletion cannot interleave with the decision.
func lockProposalArtifact(ctx context.Context, tx *sql.Tx, ownerID int64, artifactID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE artifacts SET id=id WHERE id=? AND owner_id=?`, artifactID, ownerID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrNotFound
	}
	return err
}

func (s *SQLiteStore) CreateConnectorProposal(ctx context.Context, p *ConnectorProposalRecord) (*ConnectorProposalRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := lockProposalArtifact(ctx, tx, p.OwnerID, p.ArtifactID); err != nil {
		return nil, err
	}
	existing, err := scanConnectorProposal(tx.QueryRowContext(ctx, `SELECT `+connectorProposalColumns+` FROM connector_proposals WHERE owner_id=? AND artifact_id=? AND origin=? AND auth=? AND header=?`, p.OwnerID, p.ArtifactID, p.Origin, p.Auth, p.Header))
	if err == nil {
		return existing, tx.Commit()
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	p.State = "pending"
	p.ConnectionID, err = findNativeCredentialDestination(ctx, tx, p.OwnerID, []string{p.Origin}, p.Header)
	if err != nil {
		return nil, err
	}
	if p.ConnectionID != "" {
		// Reusing an existing account adds no grant or other authority.
		p.State = "approved"
	} else {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM connector_proposals WHERE owner_id=? AND artifact_id=? AND state='pending'`, p.OwnerID, p.ArtifactID).Scan(&count); err != nil {
			return nil, err
		}
		if count >= 8 {
			return nil, ErrConnectorProposalLimit
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO connector_proposals(id,owner_id,artifact_id,name,origin,header,auth,reason,state,connection_id) VALUES(?,?,?,?,?,?,?,?,?,?)`, p.ID, p.OwnerID, p.ArtifactID, p.Name, p.Origin, p.Header, p.Auth, p.Reason, p.State, nullString(p.ConnectionID))
	if err != nil {
		return nil, err
	}
	return p, tx.Commit()
}

func (s *SQLiteStore) GetConnectorProposal(ctx context.Context, ownerID int64, artifactID, id string) (*ConnectorProposalRecord, error) {
	return scanConnectorProposal(s.db.QueryRowContext(ctx, `SELECT `+connectorProposalColumns+` FROM connector_proposals WHERE id=? AND owner_id=? AND artifact_id=? AND EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)`, id, ownerID, artifactID, artifactID, ownerID))
}

// ResolveConnectorProposal commits the new account and approval together.
// Repeated decisions return the original terminal result, including denial.
// The service must validate and seal c before calling this persistence method.
func (s *SQLiteStore) ResolveConnectorProposal(ctx context.Context, ownerID int64, artifactID, id string, approve bool, c *ConnectionRecord, limit int) (*ConnectorProposalRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := lockProposalArtifact(ctx, tx, ownerID, artifactID); err != nil {
		return nil, err
	}
	p, err := scanConnectorProposal(tx.QueryRowContext(ctx, `SELECT `+connectorProposalColumns+` FROM connector_proposals WHERE id=? AND owner_id=? AND artifact_id=?`, id, ownerID, artifactID))
	if err != nil {
		return nil, err
	}
	if p.State != "pending" {
		return p, tx.Commit()
	}
	p.State = "denied"
	if approve {
		if c == nil || c.OwnerID != ownerID || c.Kind != "native" || c.Provider != p.Auth || c.EncryptedSecret == "" {
			return nil, errors.New("invalid connector proposal approval")
		}
		origins, header, err := nativeCredentialDestination(c)
		if err != nil || len(origins) != 1 || origins[0] != p.Origin || !strings.EqualFold(header, p.Header) {
			return nil, errors.New("connector proposal approval changed its destination")
		}
		p.ConnectionID, err = findNativeCredentialDestination(ctx, tx, ownerID, origins, header)
		if err != nil {
			return nil, err
		}
		if p.ConnectionID == "" {
			if err := createConnectionTx(ctx, tx, c, limit); err != nil {
				return nil, err
			}
			p.ConnectionID = c.ID
		}
		p.State = "approved"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE connector_proposals SET state=?,connection_id=? WHERE id=?`, p.State, nullString(p.ConnectionID), p.ID); err != nil {
		return nil, err
	}
	return p, tx.Commit()
}

// The credential destination is its origin and header. Changing a display name
// or switching the value's authentication scheme must not overwrite an account
// or reserve another account slot for the same injection position.
func nativeCredentialDestination(c *ConnectionRecord) ([]string, string, error) {
	if c.Kind != "native" {
		return nil, "", nil
	}
	var policy struct {
		Origins []string `json:"origins"`
		Header  string   `json:"header"`
	}
	if err := json.Unmarshal([]byte(c.RecipeJSON), &policy); err != nil {
		return nil, "", err
	}
	if policy.Header == "" {
		switch c.Provider {
		case "bearer", "basic":
			policy.Header = "Authorization"
		case "api-key":
			policy.Header = "X-Api-Key"
		}
	}
	origins, err := origin.NormalizeOrigins(policy.Origins)
	return origins, strings.ToLower(policy.Header), err
}

func findNativeCredentialDestination(ctx context.Context, tx *sql.Tx, ownerID int64, origins []string, header string) (string, error) {
	if len(origins) == 0 || header == "" {
		return "", nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,provider,recipe_json FROM connections WHERE owner_id=? AND kind='native' AND status='active' ORDER BY created_at,id`, ownerID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		c := ConnectionRecord{Kind: "native"}
		if err := rows.Scan(&c.ID, &c.Provider, &c.RecipeJSON); err != nil {
			return "", err
		}
		otherOrigins, otherHeader, err := nativeCredentialDestination(&c)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(header, otherHeader) {
			continue
		}
		for _, candidate := range origins {
			for _, existing := range otherOrigins {
				if candidate == existing {
					return c.ID, nil
				}
			}
		}
	}
	return "", rows.Err()
}
