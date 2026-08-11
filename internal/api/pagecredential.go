// What credential a server-rendered page hands its own JavaScript (av-5imk).
//
// The gallery pages are HTML: they sit outside the API's auth group and their
// scripts authenticate their own fetches. For as long as every page visitor was
// the operator, embedding the process's `AUTH_TOKEN` in each page's bootstrap
// <script> was the whole mechanism, and it was correct.
//
// Sessions (av-30rj) and public mode (av-wmp6) ended that. A page's visitor is
// now a property of the *request* — a logged-in user, an anonymous reader, or
// the operator of a single-user instance — and only the last of the three is
// entitled to the service token. Emitting it unconditionally handed a
// session-authenticated browser a second, stronger credential that logout
// cannot revoke: deleting the session row does nothing about a token already
// sitting in page source the visitor has loaded, and that token can only be
// withdrawn by rotating the secret for everyone. The session layer keeps its
// promise; the page bootstrap was breaking it.
//
// So the credential is derived here, from the request, and nowhere else. Every
// page render calls pageCredentials and passes the result to its template; no
// handler reads ro.cfg.AuthToken for this purpose again.
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
// Two fields rather than one because "no token" means two different things and
// the page has to tell them apart: a session-authenticated browser holds a
// credential it simply does not need to be handed again, while an anonymous
// visitor holds none at all. The first may still write; the second may not, and
// a page that cannot distinguish them would either send doomed mutations or
// refuse legitimate ones.
type pageCredentials struct {
	// Token is the bearer credential the page's fetches carry, or "" when the
	// visitor already has one (a session cookie the browser attaches
	// automatically) or is entitled to none.
	Token string
	// ReadOnly says this visitor may not mutate, so the page's JS refuses
	// writes locally rather than sending them to be refused. It is the seam
	// the public-page tickets (av-eu3v, av-epnt, av-n8v5) hang adaptive chrome
	// off; withholding a credential is this ticket's job, dressing the page
	// for it is theirs.
	ReadOnly bool
}

// pageCredentials answers what this request's page render may embed.
//
// Three cases, in the order they are decided:
//
//  1. **Session-authenticated browser → no token.** The cookie is a real,
//     per-user, server-side revocable credential that the browser already
//     sends on every same-origin fetch, so the embedded bearer token is
//     vestigial — and, being the operator's, strictly worse than vestigial.
//     authMiddleware checks the session before anything else, so the page's
//     API calls authenticate on the cookie alone.
//
//  2. **Anonymous visitor on a public instance → no token, read-only.** There
//     is no credential to give someone who presented none. Nothing marks a
//     *page* request as a public visitor yet (av-wmp6 marks API requests
//     only), so this branch is dormant; it exists so that the ticket which
//     opens a page to anonymous readers inherits the answer instead of
//     inventing one.
//
//  3. **No login configured at all → the static token, as before.** Such an
//     instance issues no sessions, so the static token is the only credential
//     its page JS can authenticate with, and its page visitor is by
//     construction the operator who holds that token anyway. Nothing changes
//     for the self-hoster; TestSingleUserPageStillCarriesTheStaticToken pins
//     that.
//
// Case 3 is written as "no login" rather than "no session", deliberately. On an
// instance that *has* one, a page render that resolved no session is either a
// public visitor or a hole in sessionGate — and the token is not the right
// answer to either. Falling back to it would make every future gap in the gate
// a credential leak instead of a 401.
//
// It asks loginEnabled, not identityEnabled. The argument above never had
// anything to do with OIDC specifically — it is about whether this instance
// issues sessions at all — but the test was spelled as the provider, which left
// local-credential instances (av-q30x, av-rzvf) falling through to the token on
// exactly the reasoning that says they should not. No page route could reach
// that branch, because every one of them sits inside sessionGate's group; the
// asymmetry was a latent one, and it is the *default* shape now that av-jviu
// seeds an account on first boot. A defence that holds only for the
// configuration nobody runs is not a defence.
func (ro *Router) pageCredentials(r *http.Request) pageCredentials {
	ctx := r.Context()
	anonymous := publicVisitor(ctx)
	return pageCredentials{
		Token:    ro.pageToken(ctx, anonymous),
		ReadOnly: anonymous,
	}
}

func (ro *Router) pageToken(ctx context.Context, anonymous bool) string {
	if sessionAuthed(ctx) || anonymous || ro.loginEnabled() {
		return ""
	}
	return ro.cfg.AuthToken
}
