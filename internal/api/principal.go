// The single answer to "who is this request, and how far may it reach"
// (av-o5cf). Five call sites used to derive that answer independently —
// authMiddleware, sessionGate, adminRequest, authorizeEventStream, and
// pageCredentials each re-checked some subset of {session cookie, service
// token, agent grant, public mode} against the raw request, and the
// invariant that they agree lived only in comments and tests. It broke once
// already (av-syug: the page chain wasn't wired to the session and quietly
// served owner 1's library to everyone).
//
// Now there is one type and one rule: whichever gate admits a request
// (authMiddleware for the API group, sessionGate for the page group,
// authorizeEventStream for the SSE route, which cannot run either — Recorder
// sets no headers) constructs exactly one Principal and stores it in the
// request's context. Every other function in this package — adminRequest,
// pageCredentials, ownerIDFromCtx, agentGrantFromCtx — reads that value. None
// of them re-derives it from the request.
package api

import (
	"context"

	"github.com/momja/Exhibit/internal/agentscope"
)

// PrincipalKind names which credential, if any, resolved a request.
type PrincipalKind int

const (
	// PrincipalNone is the zero value: no credential resolved this request,
	// and it reached a handler only because a pass-through case allowed it —
	// app auth off entirely (authMiddleware's last branch), or an instance
	// with no login configured at all (sessionGate's first branch, and
	// ownerMiddleware's backstop for both groups). adminRequest's answer for
	// this kind is "only if the instance has no login", which is a property
	// of the instance rather than of the Principal — see its own comment.
	PrincipalNone PrincipalKind = iota
	// PrincipalSession is a person, authenticated by a session cookie
	// (av-30rj, av-q30x).
	PrincipalSession
	// PrincipalServiceToken is the operator's static bearer token — full
	// authority, every route.
	PrincipalServiceToken
	// PrincipalAgentGrant is a Pi sidecar session's scoped credential
	// (agentscope, av-e0yj): authority over exactly the one artifact that
	// session was opened against.
	PrincipalAgentGrant
	// PrincipalPublic is an anonymous visitor admitted only because this
	// instance publishes its library (av-wmp6). ReadOnly is always true.
	PrincipalPublic
)

// Principal is the request's resolved identity and reach.
type Principal struct {
	// OwnerID is whose library this request may touch. It is meaningful for
	// every kind, including PrincipalNone: ownerMiddleware's backstop and
	// authMiddleware's public branch both set it to a real value, never left
	// for a later guess (av-syug AC#6 — see ownerIDFromCtx).
	OwnerID int64
	Kind    PrincipalKind
	// ReadOnly says this principal may not mutate, whatever route it reaches.
	// Only PrincipalPublic sets it today; it is its own field rather than
	// `Kind == PrincipalPublic` because a future principal (a shared
	// artifact's viewer, av-7k7b) may be read-only without being anonymous.
	ReadOnly bool
	// Grant is the scoped credential this request authenticated with, non-nil
	// only when Kind is PrincipalAgentGrant.
	Grant *agentscope.Grant
}

const principalKey contextKey = "principal"

// withPrincipal stores p as ctx's resolved Principal. Called exactly once per
// request, by whichever gate admitted it.
func withPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// principalFromCtx returns the request's resolved Principal, or the zero
// Principal — OwnerID noOwner, Kind PrincipalNone — for a request no gate
// marked. That default is deliberately the same fail-closed answer
// ownerIDFromCtx has always given a request nobody attributed (av-syug
// AC#6): a scoped Store call made with noOwner matches no row, and
// adminRequest's PrincipalNone case is never true authority, only "maybe, if
// the instance has no login at all."
func principalFromCtx(ctx context.Context) Principal {
	p, ok := ctx.Value(principalKey).(Principal)
	if !ok {
		return Principal{OwnerID: noOwner}
	}
	return p
}

// principalResolved reports whether some upstream gate already stored a
// Principal on ctx. It is ownerMiddleware's only question: back-stop with the
// single-user default when nothing resolved one, and otherwise leave whatever
// authMiddleware or sessionGate decided untouched.
func principalResolved(ctx context.Context) bool {
	_, ok := ctx.Value(principalKey).(Principal)
	return ok
}

// ownerIDFromCtx returns the owner this request was attributed to, and fails
// closed when it was attributed to nobody (av-syug AC#6).
//
// It used to answer defaultOwnerID for a request with nothing in context, and
// that silence is the entire reason the page routes once shipped unscoped: `/`
// served owner 1's library to whoever asked, with no error, no zero value and
// no failing test — just the wrong shelf. The store layer had already made
// the opposite choice (ListArtifacts treats an unset OwnerID as matching
// nothing, with a test pinning it); this helper was the asymmetry.
//
// Failing closed is affordable because no caller depends on the guess any
// more: every credential path resolves a full Principal — authMiddleware from
// the session, service token, agent grant, or public-mode configuration;
// sessionGate from the session cookie — and ownerMiddleware backstops both
// groups with the single-user default. A request that reaches a handler with
// no Principal in context is therefore a wiring defect, not a deployment
// shape, and the two ways it can fail are not comparable: an empty library is
// a visible bug its own operator reports, while the wrong library is an
// invisible cross-tenant read its victim never learns about.
//
// The stricter forms were considered and rejected. Returning (int64, bool)
// makes the omission a compile error, but at ~40 call sites that mostly answer
// it identically it buys a mechanical `if !ok` that is copied rather than
// thought about; the tripwire that actually catches a new unscoped page is the
// route walk in pageowner_test.go, which fails on a route nobody declared.
// Panicking would turn a wiring slip into an outage on a surface where the
// degraded answer is already safe.
func ownerIDFromCtx(ctx context.Context) int64 {
	return principalFromCtx(ctx).OwnerID
}

// agentGrantFromCtx returns the scoped credential this request authenticated
// with, or nil when it was a person's or nobody's.
func agentGrantFromCtx(ctx context.Context) *agentscope.Grant {
	return principalFromCtx(ctx).Grant
}

// publicVisitor reports whether this request was let through with no
// credential because the instance is public (av-wmp6 AC#5). It is the branch
// a handler takes to render a page with no edit controls, and to mint render
// tokens that carry no principal.
//
// False is the safe answer and the default: a request nobody marked reads
// back PrincipalNone, which is not PrincipalPublic — the reading that
// withholds rather than publishes.
func publicVisitor(ctx context.Context) bool {
	return principalFromCtx(ctx).Kind == PrincipalPublic
}

// sessionAuthed reports whether this request presented a session cookie that
// resolved to a live session. False is the safe default, for the same reason
// publicVisitor's is.
func sessionAuthed(ctx context.Context) bool {
	return principalFromCtx(ctx).Kind == PrincipalSession
}
