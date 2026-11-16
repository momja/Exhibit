package api

import (
	"context"
	"net/http"
)

// pageCredentials is what a server-rendered page's bootstrap <script> tells the
// page's own JavaScript about the authority it is running with. It is embedded
// in each page's view model, so the templates read `.Token` and `.ReadOnly`
// whichever page they are.
//
// Token and ReadOnly answer separate questions: sessions need no embedded
// token, while artifact ownership determines whether the page may mutate it.
type pageCredentials struct {
	Clerk *clerkView

	// Token is the bearer credential the page's fetches carry, or "" when the
	// visitor already has one (a session cookie the browser attaches
	// automatically) or is entitled to none.
	Token string
	// ReadOnly says this visitor may not mutate, so the page's JS refuses
	// writes locally rather than sending them to be refused.
	ReadOnly bool
	// StateWritable is the one exception carved out of ReadOnly, and it exists
	// because the two answer different questions (av-v991). ReadOnly is about
	// the artifact — its body, its allowlist, its capability approvals, all of
	// which belong to its owner. This is about what the artifact *saves while
	// you use it*, which belongs to whoever is using it: a recipient's todo
	// list is theirs, and on a 'shared' artifact the board is the point of the
	// grant. Refusing it would leave a granted tool forgetting everything on
	// every reload, which is not a shared tool at all.
	//
	// A recipient may write state on an artifact they may reach, which is
	// the same predicate the store's
	// ...AsViewer methods enforce in SQL. The page and the server therefore
	// agree by construction rather than by two rules that must be kept in step.
	StateWritable      bool
	RelayEnabled       bool
	ConnectionsEnabled bool
}

// pageCredentials derives bootstrap authority from the resolved request.
// Sessions use their revocable cookie; the static token is embedded only on
// instances with no login. A missing session must never leak that token.
func (ro *Router) pageCredentials(r *http.Request) pageCredentials {
	ctx := r.Context()
	p := principalFromCtx(ctx)
	readOnly := p.ReadOnly
	return pageCredentials{
		Clerk:              ro.clerkView(r),
		Token:              ro.pageToken(ctx),
		RelayEnabled:       ro.cfg.Relay.Enabled,
		ConnectionsEnabled: ro.cfg.Connections != nil,
		ReadOnly:           readOnly,
		// Match the state API: unresolved requests on a login-enabled instance cannot write.
		StateWritable: !readOnly && (!ro.loginEnabled() || p.Kind != PrincipalNone),
	}
}

// forArtifactOwnedBy narrows this request's credentials to the one artifact a
// page is about: a visitor who does not own it may not mutate it (av-awr4).
//
// It only ever *removes* authority — `||`, never an assignment — so a
// principal that was already read-only stays read-only whoever owns what it is
// looking at. That direction is the whole safety of putting a per-artifact
// fact into a per-request field.
//
// A recipient is exactly the principal pageCredentials.ReadOnly was given its
// own field for: read-only without being anonymous. It is deliberately the
// same flag rather than a second one beside it, because the failure this
// prevents is a page offering a control the credential will refuse — two flags
// are two things to keep in agreement, and the disagreement *is* the bug. So
// the detail template's chrome and api.js's local refusal are one decision:
// nothing is rendered that could not be sent, and nothing is sent that would
// not be honoured.
//
// The narrowing lives here rather than in the resolved Principal because
// ownership is a property of an (viewer, artifact) pair and a Principal is
// resolved before any artifact is known. A recipient still owns their own
// library, and reading somebody else's shelf must not lock them out of it.
//
// StateWritable is deliberately left alone here, and that asymmetry is the
// point (av-v991): not owning an artifact is a reason you may not change it,
// and no reason at all that the data you type into it should evaporate. This
// method is only ever reached after the grant accessor admitted the visitor,
// so "not the owner" means "a recipient" and never "a stranger" — and the
// store enforces that same predicate again on the write, so nothing here is
// the thing holding the line.
func (c pageCredentials) forArtifactOwnedBy(viewerID, ownerID int64) pageCredentials {
	c.ReadOnly = c.ReadOnly || viewerID != ownerID
	return c
}

func (ro *Router) pageToken(ctx context.Context) string {
	if sessionAuthed(ctx) || ro.loginEnabled() {
		return ""
	}
	return ro.cfg.AuthToken
}
