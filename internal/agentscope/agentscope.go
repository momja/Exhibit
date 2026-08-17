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
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
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

	mu    sync.Mutex
	scope Scope
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
// just created under it. The first binding wins and later ones are ignored, so
// the scope only ever narrows: a session cannot walk itself from artifact to
// artifact by creating more of them.
//
// Callers must pass an id they established themselves — the API's create
// handler passes the id it just wrote. An id read back out of model-supplied
// tool arguments would make the binding forgeable, which is the whole thing
// this prevents.
func (g *Grant) BindArtifact(artifactID string) {
	if artifactID == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.scope.ArtifactID == "" {
		g.scope.ArtifactID = artifactID
	}
}

// Registry holds the live grants. Grants are in-memory and per-process,
// exactly like the sessions they belong to: a restart drops both together.
type Registry struct {
	mu     sync.Mutex
	grants map[string]*Grant
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{grants: map[string]*Grant{}}
}

// Issue mints a credential scoped to one owner and, when artifactID is
// non-empty, one artifact.
func (r *Registry) Issue(ownerID int64, artifactID string) (*Grant, error) {
	tok, err := newToken()
	if err != nil {
		return nil, err
	}
	g := &Grant{token: tok, scope: Scope{OwnerID: ownerID, ArtifactID: artifactID}}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grants[tok] = g
	return g, nil
}

// Resolve returns the grant a bearer token names, or nil. Unknown tokens are
// indistinguishable from no token at all to the caller.
func (r *Registry) Resolve(token string) *Grant {
	if token == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Constant-time compare against each live token: the map lookup alone
	// would answer "is this a valid credential?" in token-dependent time.
	// The registry holds one grant per live chat session, so this is a very
	// short walk.
	for tok, g := range r.grants {
		if len(tok) == len(token) && subtle.ConstantTimeCompare([]byte(tok), []byte(token)) == 1 {
			return g
		}
	}
	return nil
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
