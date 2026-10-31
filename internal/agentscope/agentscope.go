// Package agentscope represents agent credentials and their authority.
// Pi sessions use process-lifetime Registry grants, limited to one artifact.
// External agents use persisted grants over their owner's library, resolved
// by the API from a stored token digest. Both carry server-owned provenance
// and go through the API's deny-by-default route policy.
//
// The registry lives outside internal/agent so API authorization remains
// independent of the Pi subprocess lifecycle.
package agentscope

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// Scope is the whole authority one session credential carries.
//
// Library grants cover all artifacts owned by OwnerID. Otherwise an empty
// ArtifactID means a Pi create-mode session before its first save: it may
// create exactly one artifact, which binds it for the rest of the session.
type Scope struct {
	OwnerID    int64
	ArtifactID string
	// Library distinguishes whole-library access from Pi create mode.
	Library bool
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
	if !g.scope.Library && g.scope.ArtifactID == "" {
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
// non-empty, one artifact, for the chat session named by sessionID.
func (r *Registry) Issue(ownerID int64, artifactID, sessionID string) (*Grant, error) {
	tok, err := newToken()
	if err != nil {
		return nil, err
	}
	g := &Grant{token: tok, sessionID: sessionID, scope: Scope{OwnerID: ownerID, ArtifactID: artifactID}}
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

// --- Persisted grants (Exh-l2xj) --------------------------------------------
//
// An agent outside the server cannot hold a Registry grant: those live exactly
// as long as the Pi subprocess they were minted for. The owner instead issues
// a persisted grant, whose token is stored as a digest and resolved per
// request. It resolves to a Grant like any other, so everything downstream of
// authentication (agentScopeAllows, version provenance, the revision
// preconditions) uses the same authority and provenance representation.

// persistedTokenPrefix marks a persisted grant's token. Like the session
// token's prefix it carries no authority; it lets the middleware skip the
// database for a value that cannot be one, and lets a person recognize a
// leaked token for what it is.
const persistedTokenPrefix = "exgrant_"

// NewPersistedToken returns a fresh 256-bit token for a persisted grant and
// the digest the store keeps in its place.
func NewPersistedToken() (token, digest string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate agent grant token: %w", err)
	}
	token = persistedTokenPrefix + hex.EncodeToString(buf)
	return token, PersistedTokenDigest(token), nil
}

// IsPersistedToken reports whether token has a persisted grant's shape.
func IsPersistedToken(token string) bool {
	return strings.HasPrefix(token, persistedTokenPrefix)
}

// PersistedTokenDigest is what the store keeps for token, and what a request's
// bearer value is looked up by.
func PersistedTokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Persisted constructs a library grant with server-owned version provenance.
// Binding after creation is deliberately a no-op for library grants.
func Persisted(ownerID int64, grantID, label string) *Grant {
	return &Grant{
		sessionID: "grant:" + grantID,
		scope:     Scope{OwnerID: ownerID, Library: true},
		prompt:    label,
	}
}
