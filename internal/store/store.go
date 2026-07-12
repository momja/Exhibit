package store

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by Store implementations; handlers map them to
// HTTP status codes.
var (
	// ErrNotFound means the row (or an owner-scoped row it references) does not exist.
	ErrNotFound = errors.New("not found")
	// ErrDuplicateName means a per-owner name uniqueness constraint was violated.
	ErrDuplicateName = errors.New("name already exists")
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
	Tags             []*Tag    `json:"tags"` // populated on read by GetArtifact/ListArtifacts
}

type Collection struct {
	ID      string `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Name    string `json:"name"`
}

// DefaultTagColor is applied to a tag when no color is supplied.
const DefaultTagColor = "#6B7280"

type Tag struct {
	ID      string `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
}

type Share struct {
	ID         string     `json:"id"`
	ArtifactID string     `json:"artifact_id"`
	Public     bool       `json:"public"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// AgentKey is an owner's BYO agent provider credential. KeyCiphertext is the
// AES-GCM-sealed API key; the store never sees plaintext (Exh-ky6e).
type AgentKey struct {
	OwnerID       int64     `json:"owner_id"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	KeyCiphertext string    `json:"-"`
	UpdatedAt     time.Time `json:"updated_at"`
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

	// Tags. All mutations are owner-scoped: rows belonging to another owner
	// are treated as nonexistent (ErrNotFound). Tag names are unique per
	// owner (ErrDuplicateName on conflict).
	CreateTag(ctx context.Context, t *Tag) error
	ListTags(ctx context.Context, ownerID int64) ([]*Tag, error)
	// UpdateTag renames and/or recolors a tag; a nil name or color leaves
	// that field unchanged. Returns the updated tag.
	UpdateTag(ctx context.Context, ownerID int64, id string, name, color *string) (*Tag, error)
	// DeleteTag removes the tag globally; its artifact associations are
	// removed via ON DELETE CASCADE.
	DeleteTag(ctx context.Context, ownerID int64, id string) error
	AddArtifactTag(ctx context.Context, ownerID int64, artifactID, tagID string) error
	// RemoveArtifactTag detaches a tag from an artifact; ErrNotFound if the
	// pairing did not exist.
	RemoveArtifactTag(ctx context.Context, ownerID int64, artifactID, tagID string) error

	// State
	GetState(ctx context.Context, artifactID string) (map[string]string, error)
	SetState(ctx context.Context, artifactID, key, value string) error

	// Agent (Exh-yvhp). SetAgentKey upserts the owner's single configured
	// provider key; GetAgentKey returns nil when none is set.
	SetAgentKey(ctx context.Context, k *AgentKey) error
	GetAgentKey(ctx context.Context, ownerID int64) (*AgentKey, error)
	DeleteAgentKey(ctx context.Context, ownerID int64) error
	// SaveTranscript upserts the agent conversation that produced an
	// artifact (messagesJSON is the Pi session's message list).
	SaveTranscript(ctx context.Context, artifactID, sessionID, messagesJSON string) error
	// ListTranscripts returns messagesJSON per session for an artifact.
	ListTranscripts(ctx context.Context, artifactID string) (map[string]string, error)

	// Shares
	CreateShare(ctx context.Context, s *Share) error
	GetShare(ctx context.Context, id string) (*Share, error)
	DeleteShare(ctx context.Context, id string) error

	// Lifecycle
	Close() error
}
