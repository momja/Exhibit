package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrConnectionRequestLimit = errors.New("this artifact already has 8 pending connection requests; finish or dismiss one first")

// ArtifactConnectionRequest asks for acquisition in the app UI. It contains no
// credential, recipe or grant, and creating one never changes access policy.
type ArtifactConnectionRequest struct {
	ID         string    `json:"id"`
	OwnerID    int64     `json:"-"`
	ArtifactID string    `json:"artifact_id"`
	Provider   string    `json:"provider"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

// ConnectionRequestStore is separate from Store so existing optional database
// implementations can leave experimental connectors disabled.
type ConnectionRequestStore interface {
	RequestArtifactConnection(context.Context, int64, string, string, string) (*ArtifactConnectionRequest, error)
	ListArtifactConnectionRequests(context.Context, int64, string) ([]ArtifactConnectionRequest, error)
	DeleteArtifactConnectionRequest(context.Context, int64, string, string) error
}

var connectionProviderPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,79}$`)

func (s *SQLiteStore) RequestArtifactConnection(ctx context.Context, ownerID int64, artifactID, provider, reason string) (*ArtifactConnectionRequest, error) {
	reason = strings.TrimSpace(reason)
	if !connectionProviderPattern.MatchString(provider) || reason == "" || utf8.RuneCountInString(reason) > 1000 {
		return nil, fmt.Errorf("provider and a reason of at most 1000 characters are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var owned bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)`, artifactID, ownerID).Scan(&owned); err != nil {
		return nil, err
	}
	if !owned {
		return nil, ErrNotFound
	}
	request, err := scanConnectionRequest(tx.QueryRowContext(ctx,
		`SELECT id,owner_id,artifact_id,provider,reason,created_at FROM artifact_connection_requests WHERE artifact_id=? AND owner_id=? AND provider=?`,
		artifactID, ownerID, provider))
	if err == nil {
		// Idempotent retries retain both the request identity and the original
		// explanation, rather than letting a busy agent duplicate app prompts.
		return request, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_connection_requests WHERE artifact_id=? AND owner_id=?`, artifactID, ownerID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= 8 {
		return nil, ErrConnectionRequestLimit
	}
	request = &ArtifactConnectionRequest{ID: uuid.NewString(), OwnerID: ownerID, ArtifactID: artifactID, Provider: provider, Reason: reason, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if _, err := tx.ExecContext(ctx, `INSERT INTO artifact_connection_requests(id,owner_id,artifact_id,provider,reason,created_at) VALUES(?,?,?,?,?,?)`,
		request.ID, ownerID, artifactID, provider, reason, request.CreatedAt.Format(time.RFC3339)); err != nil {
		return nil, err
	}
	return request, tx.Commit()
}

func scanConnectionRequest(row interface{ Scan(...any) error }) (*ArtifactConnectionRequest, error) {
	var request ArtifactConnectionRequest
	var createdAt any
	if err := row.Scan(&request.ID, &request.OwnerID, &request.ArtifactID, &request.Provider, &request.Reason, &createdAt); err != nil {
		return nil, err
	}
	request.CreatedAt = anyToTime(createdAt)
	return &request, nil
}

func (s *SQLiteStore) ListArtifactConnectionRequests(ctx context.Context, ownerID int64, artifactID string) ([]ArtifactConnectionRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id,r.owner_id,r.artifact_id,r.provider,r.reason,r.created_at
		FROM artifact_connection_requests r JOIN artifacts a ON a.id=r.artifact_id AND a.owner_id=r.owner_id
		WHERE r.owner_id=? AND r.artifact_id=? ORDER BY r.created_at,r.id`, ownerID, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := make([]ArtifactConnectionRequest, 0)
	for rows.Next() {
		request, err := scanConnectionRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, *request)
	}
	return requests, rows.Err()
}

func (s *SQLiteStore) DeleteArtifactConnectionRequest(ctx context.Context, ownerID int64, artifactID, requestID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM artifact_connection_requests WHERE id=? AND owner_id=? AND artifact_id=?
		AND EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)`, requestID, ownerID, artifactID, artifactID, ownerID)
	return err
}
