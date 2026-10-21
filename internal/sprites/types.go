// Package sprites provisions isolated standalone workers through the Fly Sprites
// API. It has no dependency on Exhibit's agent, API, or storage packages.
package sprites

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrCapacity = errors.New("Sprites capacity unavailable; retry after capacity or creation-rate limits recover")
	ErrNotFound = errors.New("Sprite no longer exists; restore the central snapshot into a new assignment")
	ErrConflict = errors.New("Sprite assignment, credential, or release differs; create a new execution identity")
)

// Identity is an immutable conversation assignment. ExecutionID is the centrally
// persisted assignment/fencing generation, not an individual prompt ID. Never
// recycle it, including after deleting the Sprite.
type Identity struct {
	OwnerID     string `json:"owner_id"`
	SessionID   string `json:"session_id"`
	ExecutionID string `json:"execution_id"`
}

// Release is a trusted, immutable linux/amd64 tar.gz release. The entire runtime
// (worker, Node, Pi, extensions) must be inside it; no latest/npm install at boot.
type Release struct {
	Version     string
	URL         string
	SHA256      string
	NodeVersion string // default 22.22.0, aligned with the standalone runtime
	PiVersion   string // default 1.3.0, aligned with the standalone runtime
}

// Config belongs only in the coordinator. Defaults bound startup, cleanup, and
// retries. Prefix must be exclusive to this coordinator deployment.
type Config struct {
	Token      string
	BaseURL    string
	Prefix     string
	Release    Release
	Port       int
	WorkerArgs []string
	// Environment is static, trusted supervisor configuration, never execution
	// grants or provider keys. The launcher supplies EXHIBIT_WORKER_* separately.
	Environment    map[string]string
	MaxWorkers     int
	MaxStartups    int
	StartupTimeout time.Duration
	RequestTimeout time.Duration
	CleanupTimeout time.Duration
	IdleRetention  time.Duration
	OrphanGrace    time.Duration
	CreateInterval time.Duration
	Retry          RetryPolicy
	// ReadyPath answers JSON ReadyResponse. Override CheckReady to adapt a daemon's
	// readiness protocol. Checks must be read-only and must not start an execution.
	ReadyPath  string
	CheckReady func(context.Context, *Endpoint) error
	// HTTPClient is for management REST only; it is copied and redirects disabled.
	HTTPClient *http.Client
}

func (Config) String() string     { return "sprites.Config{credentials redacted}" }
func (c Config) GoString() string { return c.String() }

type RetryPolicy struct {
	Attempts     int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// Request must be durably saved before Provision: uncertain retries use exactly
// the same identity and endpoint token. EndpointToken is NOT an execution grant.
type Request struct {
	Identity      Identity
	EndpointToken string
}

func (r Request) String() string {
	return fmt.Sprintf("sprites.Request{%v, token redacted}", r.Identity)
}
func (r Request) GoString() string { return r.String() }

func NewEndpointToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// Endpoint.Client uses a coordinator-authenticated TLS WebSocket tunnel. Only
// the per-worker bearer token enters the Sprite. URL alone cannot authenticate.
// Close each response/event body; CloseIdleConnections cannot close active SSE.
type Endpoint struct {
	Identity      Identity
	Name          string
	URL           string
	EndpointToken string
	Client        *http.Client
	Transport     http.RoundTripper // same authenticated transport as Client.Transport
	Destroy       func(context.Context) error
}

func (e Endpoint) String() string         { return fmt.Sprintf("sprites.Endpoint{%s, token redacted}", e.Name) }
func (e Endpoint) GoString() string       { return e.String() }
func (e *Endpoint) CloseIdleConnections() { e.Client.CloseIdleConnections() }

type ReadyResponse struct {
	Version      string   `json:"version"` // worker protocol version, currently 1
	PiVersion    string   `json:"pi_version"`
	ExecutionID  string   `json:"execution_id"`
	SessionID    string   `json:"session_id"`
	Capabilities []string `json:"capabilities"`
}

type Resource struct {
	Name        string    `json:"name"`
	ID          string    `json:"id"`
	Status      string    `json:"status"` // cold, warm, running; querying never wakes it
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"created_at"`
	URLSettings *struct {
		Auth string `json:"auth"`
	} `json:"url_settings"`
}

type OrgLimits struct {
	Running      int `json:"running"`
	Warm         int `json:"warm"`
	Cold         int `json:"cold"`
	RunningLimit int `json:"running_limit"`
	WarmLimit    int `json:"warm_limit"`
}

type Inventory struct {
	Resources []Resource
	Org       OrgLimits
}

// Assignment is a centrally persisted retention decision. Active assignments
// are preserved regardless of timestamps. IdleSince must be nonzero when idle.
type Assignment struct {
	Identity  Identity
	Active    bool
	IdleSince time.Time
}

type Candidate struct {
	Resource Resource
	Identity *Identity // nil for an orphan missing from the authoritative snapshot
	Reason   string    // idle or orphan
}

type ReconcileResult struct {
	Deleted []string
	Failed  []string
	Org     OrgLimits
}

// BeforeDestroy must flush acknowledged state and revoke execution authority.
// Returning an error retains the Sprite and reports a cleanup failure.
type BeforeDestroy func(context.Context, Candidate) error

// Backend is the integration seam; the parent coordinator owns admission,
// durable assignment/token storage, fencing, grants, snapshots, and scheduling.
type Backend interface {
	Provision(context.Context, Request) (*Endpoint, error)
	Wake(context.Context, Request) (*Endpoint, error)
	Query(context.Context, Identity) (Resource, error)
	Destroy(context.Context, Identity) error
	Reconcile(context.Context, []Assignment, BeforeDestroy) (ReconcileResult, error)
}

func identityDigest(identity Identity) string {
	data, _ := json.Marshal(identity)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}
