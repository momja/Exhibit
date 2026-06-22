package store

import (
	"context"
	"time"
)

type Tier int

const (
	Tier1 Tier = 1
	Tier2 Tier = 2
)

type Artifact struct {
	ID               string    `json:"id"`
	OwnerID          int64     `json:"owner_id"`
	Title            string    `json:"title"`
	SourceBlobID     string    `json:"source_blob_id"`
	SourceURL        string    `json:"source_url"`
	Tier             Tier      `json:"tier"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	NetworkAllowlist []string  `json:"network_allowlist"`
}

type Collection struct {
	ID      string `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Name    string `json:"name"`
}

type Tag struct {
	ID      string `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Name    string `json:"name"`
}

type Share struct {
	ID         string     `json:"id"`
	ArtifactID string     `json:"artifact_id"`
	Public     bool       `json:"public"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type ListOptions struct {
	Query       string
	Tags        []string
	Collections []string
	Limit       int
	Offset      int
}

type Store interface {
	// Artifacts
	PutArtifact(ctx context.Context, a *Artifact) error
	GetArtifact(ctx context.Context, id string) (*Artifact, error)
	ListArtifacts(ctx context.Context, opts ListOptions) ([]*Artifact, error)
	UpdateArtifact(ctx context.Context, id string, updates map[string]any) error
	DeleteArtifact(ctx context.Context, id string) error

	// Collections
	CreateCollection(ctx context.Context, c *Collection) error
	ListCollections(ctx context.Context, ownerID int64) ([]*Collection, error)
	AddArtifactToCollection(ctx context.Context, artifactID, collectionID string) error
	RemoveArtifactFromCollection(ctx context.Context, artifactID, collectionID string) error

	// Tags
	CreateTag(ctx context.Context, t *Tag) error
	ListTags(ctx context.Context, ownerID int64) ([]*Tag, error)
	AddArtifactTag(ctx context.Context, artifactID, tagID string) error
	RemoveArtifactTag(ctx context.Context, artifactID, tagID string) error

	// State
	GetState(ctx context.Context, artifactID string) (map[string]string, error)
	SetState(ctx context.Context, artifactID, key, value string) error

	// Shares
	CreateShare(ctx context.Context, s *Share) error
	GetShare(ctx context.Context, id string) (*Share, error)
	DeleteShare(ctx context.Context, id string) error

	// Lifecycle
	Close() error
}
