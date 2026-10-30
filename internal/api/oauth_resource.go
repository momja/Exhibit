package api

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/momja/Exhibit/internal/agentscope"
)

// The OAuth resource-server half of MCP authorization (Exh-354p): what
// Exhibit owns whoever issues the tokens. An MCP client that meets a 401 at
// /mcp reads WWW-Authenticate, fetches the protected-resource metadata it
// names (RFC 9728), and from there finds an authorization server.
//
// Whatever issues a token, the only authority it can carry here is an agent
// grant: one owner, one artifact. A token that authorizes a *user* is never
// artifact authority by itself (the invariant the epic names); a verifier
// that accepts such a token must resolve it to a grant, or refuse it.

// protectedResourcePath is where the metadata lives. RFC 9728 §3.1 inserts
// the resource's path after the well-known prefix, and clients try that
// form first; the bare form is served too, for clients that do not.
const protectedResourcePath = "/.well-known/oauth-protected-resource"

// mcpResource is the resource identifier of the MCP endpoint, the audience
// every token accepted there must have been issued for.
func (ro *Router) mcpResource() string { return ro.cfg.AppOrigin + mcpPath }

func (ro *Router) protectedResourceMetadataURL() string {
	return ro.cfg.AppOrigin + protectedResourcePath + mcpPath
}

func (ro *Router) protectedResourceMetadata() http.Handler {
	return auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               ro.mcpResource(),
		AuthorizationServers:   ro.authorizationServers(),
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "Exhibit",
	})
}

// authorizationServers are the issuers whose tokens /mcp accepts, as the
// metadata advertises them: this instance's own (oauth.go, Exh-rhlq), whose
// access tokens are agent grants.
func (ro *Router) authorizationServers() []string { return []string{ro.cfg.AppOrigin} }

// agentToken is what a verified bearer value carries: the grant it acts
// under, and a stable id the MCP transport binds a session to, so a session
// opened under one credential cannot be continued under another.
type agentToken struct {
	grant     *agentscope.Grant
	bindingID string
}

// agentTokenVerifier maps a bearer value to the agent grant it carries, or
// (nil, nil) for a value it does not recognize. A verifier for tokens some
// other party issues must check that the token was issued for mcpResource
// (its audience) and refuse anything else.
type agentTokenVerifier func(ctx context.Context, token string) (*agentToken, error)

// agentTokenVerifiers are asked in order, by authMiddleware and by /mcp
// alike; the first to recognize a token decides it.
func (ro *Router) agentTokenVerifiers() []agentTokenVerifier {
	return []agentTokenVerifier{ro.verifyPersistedGrant}
}

// verifyAgentToken runs the verifiers over token.
func (ro *Router) verifyAgentToken(ctx context.Context, token string) (*agentToken, error) {
	for _, verify := range ro.agentTokenVerifiers() {
		t, err := verify(ctx, token)
		if err != nil || t != nil {
			return t, err
		}
	}
	return nil, nil
}

// verifyPersistedGrant resolves an owner-issued agent grant (Exh-l2xj) by its
// token's digest. A value without the grant prefix cannot be one, so it costs
// no query.
//
// The version provenance is fixed here, from the row: the session id names
// the grant, and the label the owner gave it is the message. Nothing the
// agent sends can change either.
func (ro *Router) verifyPersistedGrant(ctx context.Context, token string) (*agentToken, error) {
	if !agentscope.IsPersistedToken(token) {
		return nil, nil
	}
	row, err := ro.cfg.Store.ResolveAgentGrant(ctx, agentscope.PersistedTokenDigest(token))
	if err != nil || row == nil {
		return nil, err
	}
	id := persistedGrantSessionID(row.ID)
	return &agentToken{grant: agentscope.Persisted(row.OwnerID, row.ArtifactID, id, row.Label), bindingID: id}, nil
}

// persistedGrantSessionID is the session id an owner-issued grant's versions
// record. The prefix keeps it from ever naming a Pi conversation's
// transcript.
func persistedGrantSessionID(grantID string) string {
	return "grant:" + grantID
}
