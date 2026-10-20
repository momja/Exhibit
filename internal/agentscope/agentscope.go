// Package agentscope issues and resolves the credentials an agent chat
// session authenticates to the Exhibit API with (av-e0yj).
//
// The agent sidecar is driven by model output, and model output is steered by
// text Exhibit does not author: artifact bodies and titles arrive verbatim
// from URL ingest, and the render surface's element picker ships whatever
// markup the user clicked. Handing such a session the operator's service token
// would put the whole library inside the blast radius of one hostile page.
//
// So a session does not get the service token. It gets its own bearer token
// that resolves to one Scope — an owner, plus at most one artifact — and the
// API refuses anything outside it. The extension's id-less tools are the
// ergonomic half of that guarantee; this registry is the enforced half, which
// is what makes it hold even if the model talks the extension into something
// else.
//
// The registry deliberately lives outside internal/agent: authorization is
// Exhibit's to keep when the agent moves into its own service (Exh-k75k).
package agentscope

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// Scope is the whole authority one session credential carries.
//
// An empty ArtifactID means the session has not bound to an artifact yet —
// a create-mode session before its first save. It may create exactly one
// artifact; the create binds it, and from then on the scope names a single
// artifact for the rest of the session.
type Scope struct {
	OwnerID    int64
	ArtifactID string
}

// Grant is one issued credential: an opaque token and the scope it resolves
// to. Safe for concurrent use — the API middleware reads the scope on every
// request while the create handler may be binding it.
type Grant struct {
	token string
	// sessionID names the chat the credential was minted for. It is fixed at
	// issue, by the server, so a version an agent writes can record which
	// conversation produced it without anything the model emits being able to
	// say otherwise.
	sessionID string
	execution Execution
	validate  Validator
	guard     Guard
	bind      Binder

	mu    sync.Mutex
	scope Scope
	// prompt is the user's latest message, set by the session as each arrives.
	// It labels the versions the agent writes while answering it.
	prompt string
}

// SessionID is the chat this credential belongs to.
func (g *Grant) SessionID() string { return g.sessionID }

// SetPrompt records the user message the session is now answering.
func (g *Grant) SetPrompt(p string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prompt = p
}

// Prompt returns the user message the session is answering, or "".
func (g *Grant) Prompt() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.prompt
}

// Token is the bearer value handed to the session's subprocess.
func (g *Grant) Token() string { return g.token }

// Scope returns the credential's current authority.
func (g *Grant) Scope() Scope {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.scope
}

// BindArtifact binds an unbound (create-mode) grant to the artifact that was
// just created under it. The first binding wins; fenced grants report a
// conflicting binding as an error. The scope only ever narrows.
//
// Callers must pass an id they established themselves — the API's create
// handler passes the id it just wrote. An id read back out of model-supplied
// tool arguments would make the binding forgeable, which is the whole thing
// this prevents.
func (g *Grant) BindArtifact(artifactID string) error {
	return g.BindArtifactContext(context.Background(), artifactID)
}

// BindArtifactContext persists a fenced create-mode binding before publishing
// it to readers. The caller supplies the artifact it created, never tool input.
func (g *Grant) BindArtifactContext(ctx context.Context, artifactID string) error {
	if artifactID == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.scope.ArtifactID == "" {
		if g.bind != nil {
			if err := g.bind(ctx, g.scope, g.sessionID, g.execution, artifactID); err != nil {
				return err
			}
		}
		g.scope.ArtifactID = artifactID
	} else if g.bind != nil && g.scope.ArtifactID != artifactID {
		return fmt.Errorf("execution already bound to another artifact")
	}
	return nil
}

// Execution is issued by the core, not decoded from a tool's request body.
type Execution struct {
	ID         string
	WorkerID   string
	Generation int64
}

type Validator func(context.Context, Scope, string, Execution) error

// Guard callbacks must serialize unbound creates by conversation, including
// across reissued grants, and allow context cancellation while waiting.
type Guard func(context.Context, Scope, string, Execution) (func(), error)
type Binder func(context.Context, Scope, string, Execution, string) error

func (g *Grant) Execution() Execution { return g.execution }

// Guard keeps the execution authoritative through an entire API operation.
// Middleware MUST defer its returned release until the handler finishes: a
// Resolve check alone does not fence an in-flight write against takeover.
func (g *Grant) Guard(ctx context.Context) (func(), error) {
	if g.guard == nil {
		return func() {}, nil
	}
	// The authority callback owns conversation-wide, cancellable create
	// serialization. Middleware must still check Scope after acquiring Guard.
	release, err := g.guard(ctx, g.Scope(), g.sessionID, g.execution)
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(release)
	}, nil
}

// IsCredential recognizes retired/unknown tool credentials too, allowing auth
// middleware to reject them before anonymous/auth-disabled fallback.
func IsCredential(token string) bool { return strings.HasPrefix(token, "exagent_") }

// Registry holds in-memory, per-process grants. Restart drops these bearer
// credentials; durable execution assignments remain in the core repository.
type Registry struct {
	mu     sync.Mutex
	grants map[string]*Grant
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{grants: map[string]*Grant{}}
}

// Issue mints a credential scoped to one owner and, when artifactID is
// non-empty, one artifact, for the chat session named by sessionID.
func (r *Registry) Issue(ownerID int64, artifactID, sessionID string) (*Grant, error) {
	return r.issue(ownerID, artifactID, sessionID, Execution{}, nil, nil, nil)
}

// IssueFenced requires both a point-in-time validator and an operation guard.
func (r *Registry) IssueFenced(ownerID int64, artifactID, sessionID string, execution Execution, validate Validator, guard Guard, bind Binder) (*Grant, error) {
	if execution.ID == "" || execution.WorkerID == "" || execution.Generation <= 0 || validate == nil || guard == nil || bind == nil {
		return nil, fmt.Errorf("incomplete execution authority")
	}
	return r.issue(ownerID, artifactID, sessionID, execution, validate, guard, bind)
}

func (r *Registry) issue(ownerID int64, artifactID, sessionID string, execution Execution, validate Validator, guard Guard, bind Binder) (*Grant, error) {
	tok, err := newToken()
	if err != nil {
		return nil, err
	}
	g := &Grant{token: tok, sessionID: sessionID, scope: Scope{OwnerID: ownerID, ArtifactID: artifactID}}
	g.execution, g.validate, g.guard, g.bind = execution, validate, guard, bind
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grants[tok] = g
	return g, nil
}

// Resolve returns the grant a bearer token names, or nil. Unknown tokens are
// indistinguishable from no token at all to the caller.
func (r *Registry) Resolve(token string) *Grant {
	return r.ResolveContext(context.Background(), token)
}

func (r *Registry) ResolveContext(ctx context.Context, token string) *Grant {
	if token == "" {
		return nil
	}
	r.mu.Lock()
	var found *Grant
	// Constant-time compare against each live token: the map lookup alone
	// would answer "is this a valid credential?" in token-dependent time.
	// The registry holds one grant per live chat session, so this is a very
	// short walk.
	for tok, g := range r.grants {
		if len(tok) == len(token) && subtle.ConstantTimeCompare([]byte(tok), []byte(token)) == 1 {
			found = g
			break
		}
	}
	r.mu.Unlock()
	// Never hold the registry mutex during database access or callbacks.
	if found != nil && found.validate != nil && found.validate(ctx, found.Scope(), found.sessionID, found.execution) != nil {
		return nil
	}
	return found
}

// Revoke retires a credential. Called when the session it belongs to ends, so
// a token cannot outlive its subprocess.
func (r *Registry) Revoke(g *Grant) {
	if g == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.grants, g.token)
}

// newToken returns a 256-bit random bearer value. The prefix is there so the
// value is recognizable as an agent credential in a log or a bug report — it
// carries no authority of its own.
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate agent session token: %w", err)
	}
	return "exagent_" + hex.EncodeToString(buf), nil
}
