// Package agentcontrol is Exhibit's durable execution authority. It has no
// dependency on the agent Manager or execution runner. One Control must be
// shared by all core managers and tool middleware using a SQLite repository.
package agentcontrol

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/agentscope"
)

const (
	MaxOwnerExecutions    = 10
	DefaultFleetLimit     = 32
	MaxSnapshotBytes      = 64 << 20
	Reserved              = "reserved"
	Active                = "active"
	Closing               = "closing"
	Closed                = "closed"
	Failed                = "failed"
	Interrupted           = "interrupted"
	CheckpointTool        = "tool"
	CheckpointSettled     = "settled"
	CheckpointInterrupted = "interrupted"
)

var (
	ErrNotFound     = errors.New("assigned conversation not found")
	ErrNotResumable = errors.New("conversation can be read but cannot be resumed")
	ErrAssigned     = errors.New("conversation already has an execution")
	ErrOwnerLimit   = errors.New("owner already has ten open agent executions")
	ErrFleetLimit   = errors.New("agent execution capacity reached")
	ErrStale        = errors.New("execution is closed, expired, or superseded")
	ErrCredential   = errors.New("invalid supervisor credential")
	ErrConflict     = errors.New("checkpoint revision or request identity conflicts")
	ErrInvalid      = errors.New("invalid authority request")
	ErrTooLarge     = errors.New("snapshot exceeds 64 MiB")
)

// Identity is trusted core state. Worker requests obtain it only by resolving a
// credential; owner and generation are never accepted from a request body.
type Identity struct {
	OwnerID        int64  `json:"-"`
	ConversationID string `json:"conversation_id"`
	ExecutionID    string `json:"execution_id"`
	WorkerID       string `json:"worker_id"`
	Generation     int64  `json:"generation"`
}

type Assignment struct {
	Identity
	ArtifactID string    `json:"artifact_id"`
	Status     string    `json:"status"`
	LeaseUntil time.Time `json:"lease_until"`
	Revision   int64     `json:"revision"`
	// PreviousStatus survives restart so a replacement can surface interruption
	// even if its last durable file happened to be from an earlier settled turn.
	PreviousStatus string `json:"previous_status,omitempty"`
}

type Reservation struct {
	OwnerID                                           int64
	ConversationID, ExecutionID, WorkerID, ArtifactID string
	Resume                                            bool
	FleetLimit                                        int
	LeaseUntil                                        time.Time
}

// Snapshot is internal supervisor data, never a browser History DTO.
type Snapshot struct {
	SessionFile    string `json:"-"`
	Title          string `json:"title"`
	Revision       int64  `json:"revision"`
	Kind           string `json:"kind"`
	VersionSeq     int    `json:"version_seq"`
	PreviousStatus string `json:"previous_status,omitempty"`
}

type Checkpoint struct {
	Revision                 int64
	Kind, Title, SessionFile string
}

// Usage is a delta with an identity stable across transport retries. It is NOT
// a cumulative worker counter or a second budget ledger. PaidBy is bound by the
// core when issuing the supervisor credential, not selected in this payload.
type Usage struct {
	EventID          string `json:"event_id"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Source           string `json:"source"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	CostMicros       int64  `json:"cost_micros"`
}

type Supervisor struct {
	Assignment
	PaidBy string
}

// Repository is deliberately separate from store.Store. Transactions implement
// admission, validation+binding, checkpoint ordering, and ledger dedup atomically.
// now always comes from the core clock, not the worker.
type Repository interface {
	ReserveAgentExecution(context.Context, Reservation, time.Time) (Assignment, error)
	GetAgentExecution(context.Context, Identity, time.Time) (Assignment, error)
	TransitionAgentExecution(context.Context, Identity, string, time.Time, time.Time) error
	BindAgentExecution(context.Context, Identity, string, time.Time) error
	RecordAgentArtifactVersion(context.Context, Identity, int, time.Time) error
	ReconcileAgentExecutions(context.Context, bool, time.Time) ([]Assignment, error)
	SaveAgentSupervisor(context.Context, Identity, []byte, string, time.Time, time.Time) error
	ResolveAgentSupervisor(context.Context, []byte, time.Time) (Supervisor, error)
	ReadAgentSnapshot(context.Context, Identity, time.Time) (Snapshot, error)
	WriteAgentCheckpoint(context.Context, Identity, Checkpoint, time.Time) (Snapshot, error)
	ReportAgentUsage(context.Context, Identity, string, Usage) (bool, error)
}

type Config struct {
	FleetLimit    int
	LeaseTTL      time.Duration
	SupervisorTTL time.Duration
	Now           func() time.Time
}

type Control struct {
	repo     Repository
	registry *agentscope.Registry
	cfg      Config
	// A handler holds RLock through its last write. Reassignment/release must
	// wait, including when the lease expires while that handler is running.
	fence sync.RWMutex
	// Separate from the takeover fence, so a create can bind while holding
	// shared authority. Shared across grants to prevent reissued create tokens
	// from authorizing two simultaneous first saves.
	createFence sync.Mutex
}

func New(repo Repository, registry *agentscope.Registry, cfg Config) *Control {
	if cfg.FleetLimit <= 0 {
		cfg.FleetLimit = DefaultFleetLimit
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 2 * time.Minute
	}
	if cfg.SupervisorTTL <= 0 {
		cfg.SupervisorTTL = 15 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Control{repo: repo, registry: registry, cfg: cfg}
}

// Reserve happens before provisioning or loading any resume file/baseline.
// ExecutionID is a stable request identity supplied by the core; retrying it
// returns the same still-live reservation, never resurrects a terminal attempt.
func (c *Control) Reserve(ctx context.Context, r Reservation) (Assignment, error) {
	c.fence.Lock()
	defer c.fence.Unlock()
	now := c.cfg.Now()
	r.FleetLimit = c.cfg.FleetLimit
	r.LeaseUntil = now.Add(c.cfg.LeaseTTL)
	return c.repo.ReserveAgentExecution(ctx, r, now)
}

func (c *Control) transition(ctx context.Context, id Identity, status string) error {
	c.fence.Lock()
	defer c.fence.Unlock()
	now := c.cfg.Now()
	return c.repo.TransitionAgentExecution(ctx, id, status, now.Add(c.cfg.LeaseTTL), now)
}
func (c *Control) Activate(ctx context.Context, id Identity) error {
	return c.transition(ctx, id, Active)
}
func (c *Control) Renew(ctx context.Context, id Identity) error { return c.transition(ctx, id, "") }

// BeginClose retains assignment/capacity and supervisor authority through the
// final durable flush. Release is called only after that flush or failed startup.
func (c *Control) BeginClose(ctx context.Context, id Identity) error {
	return c.transition(ctx, id, Closing)
}
func (c *Control) Release(ctx context.Context, id Identity, terminal string) error {
	if terminal != Closed && terminal != Failed && terminal != Interrupted {
		return ErrInvalid
	}
	return c.transition(ctx, id, terminal)
}

// ReconcileExpired permits replacement after worker loss without heartbeats
// keeping idle workers awake. InterruptAll is the startup fence when this core
// cannot reattach its previous workers; both return cleanup targets to Manager.
func (c *Control) ReconcileExpired(ctx context.Context) ([]Assignment, error) {
	return c.reconcile(ctx, false)
}
func (c *Control) InterruptAll(ctx context.Context) ([]Assignment, error) {
	return c.reconcile(ctx, true)
}
func (c *Control) reconcile(ctx context.Context, all bool) ([]Assignment, error) {
	c.fence.Lock()
	defer c.fence.Unlock()
	return c.repo.ReconcileAgentExecutions(ctx, all, c.cfg.Now())
}

func (c *Control) Guard(ctx context.Context, id Identity) (func(), error) {
	c.fence.RLock()
	if _, err := c.repo.GetAgentExecution(ctx, id, c.cfg.Now()); err != nil {
		c.fence.RUnlock()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(c.fence.RUnlock) }, nil
}
func (c *Control) WithExecution(ctx context.Context, id Identity, fn func(context.Context) error) error {
	release, err := c.Guard(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	return fn(ctx)
}

func toolIdentity(scope agentscope.Scope, conversation string, e agentscope.Execution) Identity {
	return Identity{OwnerID: scope.OwnerID, ConversationID: conversation, ExecutionID: e.ID, WorkerID: e.WorkerID, Generation: e.Generation}
}
func (c *Control) validateTool(ctx context.Context, scope agentscope.Scope, conversation string, e agentscope.Execution) error {
	a, err := c.repo.GetAgentExecution(ctx, toolIdentity(scope, conversation, e), c.cfg.Now())
	if err != nil {
		return err
	}
	if a.ArtifactID != scope.ArtifactID {
		return ErrStale
	}
	return nil
}
func (c *Control) guardTool(ctx context.Context, scope agentscope.Scope, conversation string, e agentscope.Execution) (func(), error) {
	creating := scope.ArtifactID == ""
	if creating {
		c.createFence.Lock()
	}
	c.fence.RLock()
	if err := c.validateTool(ctx, scope, conversation, e); err != nil {
		c.fence.RUnlock()
		if creating {
			c.createFence.Unlock()
		}
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			c.fence.RUnlock()
			if creating {
				c.createFence.Unlock()
			}
		})
	}, nil
}

// Bind checks and narrows the durable binding in one transaction. It must not
// acquire the exclusive fence: create handlers already hold its shared side.
func (c *Control) Bind(ctx context.Context, id Identity, artifact string) error {
	return c.repo.BindAgentExecution(ctx, id, artifact, c.cfg.Now())
}

// RecordArtifactVersion is called by the core after committing a tool source
// or widget write, while its grant Guard is held. Pass the committed version,
// not the current head read later. The repository verifies its conversation
// provenance. Checkpoints then retain that version even if a browser edits the
// artifact before the worker uploads its file. This has no worker HTTP route.
func (c *Control) RecordArtifactVersion(ctx context.Context, id Identity, version int) error {
	return c.repo.RecordAgentArtifactVersion(ctx, id, version, c.cfg.Now())
}
func (c *Control) IssueTool(ctx context.Context, id Identity) (*agentscope.Grant, error) {
	release, err := c.Guard(ctx, id)
	if err != nil {
		return nil, err
	}
	defer release()
	a, err := c.repo.GetAgentExecution(ctx, id, c.cfg.Now())
	if err != nil {
		return nil, err
	}
	if c.registry == nil {
		return nil, fmt.Errorf("tool registry unavailable")
	}
	return c.registry.IssueFenced(id.OwnerID, a.ArtifactID, id.ConversationID,
		agentscope.Execution{ID: id.ExecutionID, WorkerID: id.WorkerID, Generation: id.Generation}, c.validateTool, c.guardTool,
		func(ctx context.Context, scope agentscope.Scope, conv string, e agentscope.Execution, artifact string) error {
			return c.Bind(ctx, toolIdentity(scope, conv, e), artifact)
		})
}

// IssueSupervisor rotates the assigned supervisor secret, persists only its
// SHA-256 hash, and binds the centrally selected funding source for usage.
// The returned value belongs only in the trusted supervisor channel.
func (c *Control) IssueSupervisor(ctx context.Context, id Identity, paidBy string) (string, error) {
	if paidBy != "platform" && paidBy != "user" {
		return "", ErrInvalid
	}
	release, err := c.Guard(ctx, id)
	if err != nil {
		return "", err
	}
	defer release()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := "exsuper_" + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	now := c.cfg.Now()
	if err := c.repo.SaveAgentSupervisor(ctx, id, hash[:], paidBy, now.Add(c.cfg.SupervisorTTL), now); err != nil {
		return "", err
	}
	return token, nil
}
func IsSupervisorCredential(token string) bool { return strings.HasPrefix(token, "exsuper_") }
func (c *Control) AuthenticateSupervisor(ctx context.Context, token string) (Supervisor, error) {
	if !IsSupervisorCredential(token) || len(token) != 72 {
		return Supervisor{}, ErrCredential
	}
	h := sha256.Sum256([]byte(token))
	return c.repo.ResolveAgentSupervisor(ctx, h[:], c.cfg.Now())
}
func (c *Control) ReadSnapshot(ctx context.Context, id Identity) (Snapshot, error) {
	release, err := c.Guard(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	defer release()
	return c.repo.ReadAgentSnapshot(ctx, id, c.cfg.Now())
}
func (c *Control) WriteCheckpoint(ctx context.Context, id Identity, p Checkpoint) (Snapshot, error) {
	if err := ValidateCheckpoint(p); err != nil {
		return Snapshot{}, err
	}
	release, err := c.Guard(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	defer release()
	return c.repo.WriteAgentCheckpoint(ctx, id, p, c.cfg.Now())
}

// ReportUsage intentionally permits a closed/superseded execution. Authentication
// still requires its unexpired assigned supervisor secret. The ledger transaction
// verifies historical attribution and deduplicates by (execution,event).
func (c *Control) ReportUsage(ctx context.Context, s Supervisor, u Usage) (bool, error) {
	if err := ValidateUsage(u); err != nil {
		return false, err
	}
	return c.repo.ReportAgentUsage(ctx, s.Identity, s.PaidBy, u)
}
