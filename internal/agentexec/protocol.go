// Package agentexec supervises Pi processes and transports their RPC protocol.
// It has no storage, policy, UI, or credential-issuing authority.
package agentexec

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

const (
	ProtocolVersion = "1"
	VersionHeader   = "Exhibit-Agent-Version"
	PiVersion       = "1.3.0"
	NodeVersion     = "22.22.0"
	MaxSessionBytes = 64 << 20
	MaxCommandBytes = 32 << 20
	MaxEventBytes   = 8 << 20
	MaxQueueBytes   = 16 << 20
	MaxEvents       = 4096
	MaxCommands     = 4096
	SelectionFile   = "selection.json"
	SessionFile     = "session.jsonl"
)

// Identity names one attempt, independently of the durable conversation.
// OwnerID is coordinator/provisioner metadata; it confers no worker authority.
type Identity struct {
	ExecutionID string `json:"execution_id"`
	SessionID   string `json:"session_id"`
	OwnerID     int64  `json:"-"`
	Resume      bool   `json:"-"`
	ArtifactID  string `json:"-"`
}

type Guardrail struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
}

// StartConfig contains only assigned execution configuration. ToolToken must
// already be scoped/fenced by core; it is never a supervisor/operator token.
type StartConfig struct {
	Identity
	Provider            string     `json:"provider"`
	Model               string     `json:"model,omitempty"`
	APIKey              string     `json:"api_key"`
	APIBaseURL          string     `json:"api_base_url"`
	ToolToken           string     `json:"tool_token"`
	TargetArtifactID    string     `json:"artifact_id,omitempty"`
	SystemPrompt        string     `json:"system_prompt"`
	SessionSnapshot     string     `json:"session_snapshot,omitempty"`
	MockLLMURL          string     `json:"mock_llm_url,omitempty"`
	WidgetOnly          bool       `json:"widget_only,omitempty"`
	Guardrail           *Guardrail `json:"guardrail,omitempty"`
	MaxRequestBodyBytes int64      `json:"max_request_body_bytes,omitempty"`
	// A worker independently stops on expiry, coordinator loss, or runaway
	// work. These are backstops; central policy may impose tighter deadlines.
	ExpiresAt   time.Time     `json:"expires_at"`
	TurnTimeout time.Duration `json:"turn_timeout_ns,omitempty"`
}

type Runner interface {
	Start(context.Context, StartConfig) (Process, error)
}

// Process preserves the existing Pi RPC/event adapter in core. Closing input
// stops execution but leaves Snapshot readable until Cleanup, after persistence.
type Process interface {
	io.WriteCloser
	ErasureProcess
	Stdout() io.Reader
	Stderr() io.Reader
	Wait() error
	Kill() error
	Snapshot(context.Context) (string, error)
	WriteSelection(context.Context, []string) error
	Cleanup() error
}

// Reclaimer retains only the capability needed to remove an execution. Cleanup
// is idempotent and retryable after failure.
type Reclaimer interface{ Cleanup() error }

// ErasureProcess discards private in-memory execution payloads after delivery
// has stopped, transferring only transport/scratch cleanup state to core. The
// returned capability must remain available until reclamation succeeds.
type ErasureProcess interface{ ErasureCleanup() Reclaimer }

// EventSynchronizer lets a coordinator acknowledge an event AFTER its policy,
// accounting, and durable checkpoint work. A remote idle hook must not release
// compute merely because stdout bytes have reached a reader's buffer.
// Direct Process consumers may omit this and process events asynchronously.
type EventSynchronizer interface{ SynchronizeEvents() (handled func()) }

// LeaseProcess accepts explicit core-approved deadlines. Call only during
// active execution or immediately before a new prompt, never from idle polling.
type LeaseProcess interface {
	SetLease(context.Context, time.Time) error
}

type LeaseUpdate struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Command struct {
	ID       string          `json:"id"`
	Deadline time.Time       `json:"deadline"`
	RPC      json.RawMessage `json:"rpc"`
	// Optional selection is applied atomically with the command.
	Selection *[]string `json:"selection,omitempty"`
}

type Event struct {
	Sequence uint64          `json:"sequence"`
	Line     json.RawMessage `json:"line"`
}

// An idle batch is terminal for this HTTP connection. No idle polling is needed.
type EventBatch struct {
	Events  []Event `json:"events"`
	Cursor  uint64  `json:"cursor"`
	Idle    bool    `json:"idle"`
	Settled bool    `json:"settled"` // Idle may also mean waiting on a human; only settled is safe to reconstruct.
	Closed  bool    `json:"closed"`
}

type Snapshot struct {
	SessionFile string `json:"session_file"`
	Cursor      uint64 `json:"cursor"`
	Closed      bool   `json:"closed"`
	Settled     bool   `json:"settled"`
}

type Readiness struct {
	Version      string   `json:"version"`
	PiVersion    string   `json:"pi_version"`
	ExecutionID  string   `json:"execution_id"`
	SessionID    string   `json:"session_id"`
	Started      bool     `json:"started"`
	Closed       bool     `json:"closed"`
	Unassigned   bool     `json:"unassigned,omitempty"`
	Capabilities []string `json:"capabilities"`
}

type WireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var ProviderEnvironment = map[string]string{
	"anthropic": "ANTHROPIC_API_KEY", "openai": "OPENAI_API_KEY",
	"google": "GEMINI_API_KEY", "openrouter": "OPENROUTER_API_KEY",
	"opencode-go": "OPENCODE_API_KEY", "exhibit-mock": "EXHIBIT_MOCK_API_KEY",
}
