package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
)

// The session layer (av-30rj, extended by av-q30x and av-rzvf). Two login
// paths reach it:
//
//   - An identity provider, exchanged exactly once at /auth/callback.
//   - A local login name and password, checked at /auth/local.
//
// They converge on startSession and diverge nowhere after it. Everything past
// that point — the cookie, the per-request lookup, owner_id — knows only that
// it holds a *store.User, which is why swapping providers touches one
// constructor in cmd/server, and why adding local credentials added a handler
// rather than a second session mechanism.
//
// Since av-rzvf the local path resolves its credential against the users table
// rather than against a single environment variable, so an instance can have
// more than one local account. resolveLocalLogin below is the whole of that
// change; nothing about the form, the bcrypt compare, the cookie or the CSRF
// posture moved with it.
//
// The local credential is deliberately not an IdentityProvider; internal/auth's
// local.go says why that interface does not fit a form post.
//
// When neither is configured this whole surface is absent: the /auth routes are
// never registered, the gate is a pass-through, and the static token plus owner
// 1 behave exactly as they always have.
const (
	// sessionCookieName holds an opaque random session id, looked up on
	// every request. It is deliberately not a signed token carrying claims:
	// a row can be deleted, so logout takes effect on the next request
	// rather than whenever a token would have expired.
	sessionCookieName = "exhibit_session"
	// The login-flow cookies, alive only for the round trip to the provider.
	stateCookieName    = "exhibit_auth_state"
	verifierCookieName = "exhibit_auth_verifier"
	nextCookieName     = "exhibit_auth_next"

	// loginFlowTTL bounds how long a half-finished login may sit.
	loginFlowTTL = 10 * time.Minute
)

// DefaultSessionTTL is how long a login lasts unless the operator says
// otherwise. It is an upper bound, not a promise: logout revokes sooner.
const DefaultSessionTTL = 30 * 24 * time.Hour

// identityEnabled reports whether this instance delegates login to a provider.
func (ro *Router) identityEnabled() bool { return ro.cfg.Identity != nil }

// localLoginEnabled reports whether this instance has passwords of its own
// (av-q30x, av-rzvf) — either the environment credential, or at least one
// account already provisioned into the users table.
//
// Both count, because either alone is a complete answer. An operator may set
// only LOGIN_USERNAME and never provision a second account; another may
// provision accounts with the CLI and unset the environment pair afterwards.
// Neither set is the single-user default, unchanged.
func (ro *Router) localLoginEnabled() bool {
	return ro.cfg.LocalCredential != nil || ro.cfg.LocalUsers
}

// loginEnabled reports whether this instance has *any* way for a person to log
// in. It, not identityEnabled, is what arms the session gate: before av-q30x an
// instance with no OIDC issuer had no gate at all, so securing a self-hosted
// library meant standing up an identity server or putting auth in the proxy.
// A local credential is now a second answer, and the gate has to recognize it —
// otherwise configuring a password would leave every page as open as before.
//
// False is still the default and still means single-user: static token, owner 1,
// no gate.
func (ro *Router) loginEnabled() bool { return ro.identityEnabled() || ro.localLoginEnabled() }

func (ro *Router) sessionTTL() time.Duration {
	if ro.cfg.SessionTTL > 0 {
		return ro.cfg.SessionTTL
	}
	return DefaultSessionTTL
}

// setupAuthRoutes registers the login flow, and only the parts this instance
// can actually serve. Registering them unconditionally would change what an
// unconfigured instance answers at /auth/login — today the styled 404 page —
// for no benefit, and would offer a visitor an SSO button or a password form
// leading nowhere.
func (ro *Router) setupAuthRoutes(r chi.Router) {
	if !ro.loginEnabled() {
		return
	}
	// The single entry point, whatever is configured: the gate redirects here,
	// and this decides whether that means a form or a trip to the provider.
	r.Get("/auth/login", ro.authLogin)
	if ro.localLoginEnabled() {
		// The only endpoint that answers a guessed credential, and so the only
		// one that is throttled (av-t21v). It sits on the route rather than in
		// the handler so that what a credential *is* and how often it may be
		// asked stay separable — see loginratelimit.go.
		r.With(ro.loginRateLimit).Post("/auth/local", ro.authLocal)
	}
	if ro.identityEnabled() {
		// Split out of /auth/login so the login page has something to point
		// its "continue with SSO" button at when both paths exist.
		r.Get("/auth/sso", ro.authStartSSO)
		r.Get("/auth/callback", ro.authCallback)
	}
	// Logout accepts both verbs: a link is the obvious affordance, and
	// revoking a session is not a destructive action a forged request could
	// abuse for anything worse than an inconvenience.
	r.Get("/auth/logout", ro.authLogout)
	r.Post("/auth/logout", ro.authLogout)
}

// authLogin is where an unauthenticated visitor lands.
//
// With a local credential configured it renders the login page. With only a
// provider it redirects straight there, which is both this route's original
// behaviour and the right one: a page whose only control is "continue to the
// provider" is a choice that does not exist, presented as one.
func (ro *Router) authLogin(w http.ResponseWriter, r *http.Request) {
	if !ro.localLoginEnabled() {
		ro.authStartSSO(w, r)
		return
	}
	ro.renderLogin(w, r, http.StatusOK, safeNext(r.URL.Query().Get("next")), "", "")
}

// authStartSSO starts Authorization Code + PKCE: mint a state and a verifier,
// park both in short-lived cookies, and hand the browser to the provider.
func (ro *Router) authStartSSO(w http.ResponseWriter, r *http.Request) {
	state, err := auth.NewState()
	if err != nil {
		http.Error(w, "login unavailable", http.StatusInternalServerError)
		return
	}
	verifier, err := auth.NewVerifier()
	if err != nil {
		http.Error(w, "login unavailable", http.StatusInternalServerError)
		return
	}
	ro.setCookie(w, stateCookieName, state, loginFlowTTL)
	ro.setCookie(w, verifierCookieName, verifier, loginFlowTTL)
	if next := safeNext(r.URL.Query().Get("next")); next != "" {
		ro.setCookie(w, nextCookieName, next, loginFlowTTL)
	}
	http.Redirect(w, r, ro.cfg.Identity.AuthURL(state, verifier), http.StatusFound)
}

// authCallback is the one point in the system that talks to the provider. It
// redeems the code for an identity, resolves that identity to a user row
// (creating it on first login), and issues our own session.
func (ro *Router) authCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// A provider that refuses the login says so here rather than by
	// omitting the code; surface its reason instead of a generic failure.
	if e := q.Get("error"); e != "" {
		ro.failLogin(w, "identity provider refused the login", slog.String("provider_error", e))
		return
	}

	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value == "" {
		ro.failLogin(w, "login expired — start again")
		return
	}
	if subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(q.Get("state"))) != 1 {
		ro.failLogin(w, "login state mismatch")
		return
	}
	verifierCookie, err := r.Cookie(verifierCookieName)
	if err != nil || verifierCookie.Value == "" {
		ro.failLogin(w, "login expired — start again")
		return
	}
	code := q.Get("code")
	if code == "" {
		ro.failLogin(w, "identity provider returned no code")
		return
	}

	// The flow cookies have done their job whatever happens next.
	ro.clearCookie(w, stateCookieName)
	ro.clearCookie(w, verifierCookieName)
	ro.clearCookie(w, nextCookieName)

	identity, err := ro.cfg.Identity.Exchange(r.Context(), code, verifierCookie.Value)
	if err != nil {
		ro.failLogin(w, "could not complete login", slog.String("err", err.Error()))
		return
	}

	// The provider identity becomes a users row here — created on first
	// login, its email refreshed on every later one. A local account is the
	// same row with a password column filled in, which is what puts both
	// kinds of user in one owner_id space (av-rzvf).
	user, err := ro.cfg.Store.UpsertUser(r.Context(), identity.ExternalID, identity.Email)
	if err != nil {
		ro.failLogin(w, "could not resolve identity", slog.String("err", err.Error()))
		return
	}
	// A disabled account is refused here rather than at the provider, because
	// the provider does not know about it: disabling is an Exhibit decision
	// (av-utap) and this is the one point a provider identity becomes a
	// session. The message names no account and no reason.
	if user.Disabled {
		slog.Warn("login refused: account disabled", slog.Int64("user_id", user.ID))
		ro.failLogin(w, "this account cannot sign in")
		return
	}
	if err := ro.startSession(w, r, user, "oidc"); err != nil {
		ro.failLogin(w, "could not start session", slog.String("err", err.Error()))
		return
	}

	dest := "/"
	if c, err := r.Cookie(nextCookieName); err == nil {
		if next := safeNext(c.Value); next != "" {
			dest = next
		}
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// authLocal is the local credential's whole login path: check a form post
// against the one configured credential, and on success take the same last step
// the provider callback takes.
//
// It has no CSRF defence of its own and needs none. The session cookie's
// SameSite=Lax (av-ke2m) protects requests that *carry* a credential, and this
// one runs before any session exists; the only thing a cross-site page could
// forge here is a login it must already know the password to complete.
func (ro *Router) authLocal(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ro.renderLogin(w, r, http.StatusBadRequest, "", "", "Could not read that form. Try again.")
		return
	}
	username := r.PostFormValue("username")
	next := safeNext(r.PostFormValue("next"))

	// One message for a wrong name and a wrong password, and nothing about
	// either in the log line — a failed login is worth recording, the
	// credential someone tried is not.
	user, err := ro.resolveLocalLogin(r.Context(), username, r.PostFormValue("password"))
	if err != nil {
		slog.Error("resolve local login", slog.String("err", err.Error()))
		ro.renderLogin(w, r, http.StatusInternalServerError, next, username,
			"Could not check that login. Try again.")
		return
	}
	if user == nil {
		slog.Warn("local login failed", slog.String("remote_addr", r.RemoteAddr))
		ro.renderLogin(w, r, http.StatusUnauthorized, next, username,
			"That username and password don't match.")
		return
	}

	if err := ro.startSession(w, r, user, "local"); err != nil {
		slog.Error("start local session", slog.String("err", err.Error()))
		ro.renderLogin(w, r, http.StatusInternalServerError, next, username,
			"Could not start a session. Try again.")
		return
	}

	dest := "/"
	if next != "" {
		dest = next
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// resolveLocalLogin answers "which account, if any, does this name and
// password belong to?" — nil for no match, an error only when the lookup
// itself failed.
//
// The precedence is the security decision in av-rzvf, and it runs one way
// round on purpose:
//
//  1. The environment credential, when the submitted name is the one it is
//     configured for. It is checked *first* so that no state in the database
//     can shadow it — that is the whole of its break-glass value. Its account
//     row is created on demand, which is also how it bootstraps the first
//     admin on an empty instance.
//  2. Otherwise, or if that password did not match, the users table: by login
//     name, against the stored hash.
//
// Read together: LOGIN_USERNAME names an account and LOGIN_PASSWORD_HASH is an
// *additional* always-accepted password for it. An operator locked out of
// their own account points the pair at their own login name and is back in it —
// not in some separate rescue account holding none of their artifacts — and
// the `user passwd` they then run takes effect immediately rather than at the
// next restart, because the stored password was never displaced.
//
// One bcrypt compare happens per attempt, and it happens whether or not the
// name exists (auth.VerifyStoredPassword spends the compare on an absent
// account too). Cost that varies with the input is how a login endpoint tells
// an attacker which names are real. The single exception is a failed attempt
// against the one name the environment credential configures, which costs two —
// that name is in the operator's own environment, not something an attacker
// learns anything by probing for.
// A disabled account (av-utap) is refused on *both* branches, including the
// environment credential's. That is the one place this function departs from
// "no state in the database can shadow the break-glass credential", and it
// departs deliberately: a disable a documented environment variable defeats is
// not a disable, and the break-glass value survives intact because the last
// enabled admin can never be disabled (store.ErrLastAdmin) — so there is always
// an account for LOGIN_USERNAME to name.
func (ro *Router) resolveLocalLogin(ctx context.Context, name, password string) (*store.User, error) {
	if cred := ro.cfg.LocalCredential; cred != nil && cred.Names(name) && cred.VerifyPassword(password) {
		identity := cred.Identity()
		user, err := ro.cfg.Store.UpsertUser(ctx, identity.ExternalID, identity.Email)
		if err != nil {
			return nil, err
		}
		return enabledOrNil(user), nil
	}
	user, hash, err := ro.cfg.Store.LookupLocalCredential(ctx, auth.LocalExternalID(name))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	// The bcrypt compare runs first and unconditionally, so a disabled account
	// costs a login attempt exactly what an enabled one does. Checking Disabled
	// before spending it would make the endpoint answer faster for accounts an
	// admin has turned off, which is a fact about the directory an attacker has
	// no business timing out of it.
	if !auth.VerifyStoredPassword(hash, password) {
		return nil, nil
	}
	return enabledOrNil(user), nil
}

// enabledOrNil collapses "disabled account" into the same nil the login path
// already means by "no match". The caller renders one message for every way a
// sign-in can fail, so a person locked out learns that they are, and an
// attacker learns nothing about which accounts exist or which are switched off.
func enabledOrNil(u *store.User) *store.User {
	if u == nil || u.Disabled {
		return nil
	}
	return u
}

// startSession is where the two login paths meet, and the only place a session
// is ever created.
//
// Whatever proved who the visitor is — a provider's token exchange or a
// password — reaches here as a resolved users row and produces the same
// sessions row and the same cookie. `method` is for the log line only; nothing
// downstream may branch on it, because a session that remembered how it was
// created would be the beginning of a second session layer.
func (ro *Router) startSession(w http.ResponseWriter, r *http.Request, user *store.User, method string) error {
	sid, err := auth.NewSessionID()
	if err != nil {
		return err
	}
	sess := &store.Session{
		ID:        sid,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(ro.sessionTTL()),
	}
	if err := ro.cfg.Store.CreateSession(r.Context(), sess); err != nil {
		return err
	}
	ro.setCookie(w, sessionCookieName, sid, ro.sessionTTL())
	slog.Info("login", slog.Int64("user_id", user.ID), slog.String("method", method))
	return nil
}

// authLogout revokes the session server-side. Dropping the cookie alone would
// leave a usable credential in anything that had already copied it; deleting
// the row is what makes the session unusable on the very next request.
func (ro *Router) authLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		if err := ro.cfg.Store.DeleteSession(r.Context(), c.Value); err != nil {
			slog.Warn("revoke session", slog.String("err", err.Error()))
		}
	}
	ro.clearCookie(w, sessionCookieName)
	http.Redirect(w, r, "/", http.StatusFound)
}

// loginPageData feeds the login page. Username and Error are echoed back into
// the form so a failed attempt does not clear it; Username is whatever was
// typed, so html/template's contextual escaping is what keeps it text.
//
// SSO/SSOHref are present only when a provider is *also* configured. The page
// exists to offer a choice, and it must not offer one that does not exist.
type loginPageData struct {
	Favicon  template.URL
	LogoSVG  template.HTML
	Next     string
	Username string
	Error    string
	SSO      bool
	SSOHref  string
}

// renderLogin writes the login page. next must already have been through
// safeNext — it is rendered into a hidden field and posted straight back.
func (ro *Router) renderLogin(w http.ResponseWriter, r *http.Request, status int, next, username, errMsg string) {
	data := loginPageData{
		Favicon:  template.URL(exhibitLogoDataURI),
		LogoSVG:  template.HTML(exhibitLogoSVG),
		Next:     next,
		Username: username,
		Error:    errMsg,
		SSO:      ro.identityEnabled(),
		SSOHref:  "/auth/sso",
	}
	if data.SSO && next != "" {
		data.SSOHref += "?next=" + url.QueryEscape(next)
	}
	page, err := renderPage("login", data)
	if err != nil {
		serverError(w, r, "login page render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A submitted username is on this page; a shared or proxy cache holding it
	// would hand the next visitor somebody else's half-filled form.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprint(w, page)
}

func (ro *Router) failLogin(w http.ResponseWriter, msg string, attrs ...slog.Attr) {
	args := []any{slog.String("reason", msg)}
	for _, a := range attrs {
		args = append(args, a)
	}
	slog.Warn("login failed", args...)
	http.Error(w, msg, http.StatusBadRequest)
}

// sessionUser resolves a request's session cookie to an owner id. A missing,
// unknown, revoked, or expired session all answer the same way — not
// authenticated — because that is the only distinction a caller acts on.
//
// An instance with no login configured issues no sessions, so it recognizes
// none: a cookie presented there is somebody else's leftover, not a credential.
func (ro *Router) sessionUser(r *http.Request) (int64, bool) {
	if !ro.loginEnabled() {
		return 0, false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return 0, false
	}
	sess, err := ro.cfg.Store.GetSession(r.Context(), c.Value)
	if err != nil || sess == nil {
		return 0, false
	}
	return sess.UserID, true
}

// setCookie writes an app-origin cookie.
//
// No Domain attribute, so the cookie is host-only: it is sent to the app
// origin and nowhere else. That is load-bearing rather than incidental — a
// cookie scoped loosely enough to reach RENDER_ORIGIN would be readable by
// artifact code, because a top-level /a/:id is a real-origin document with the
// artifact's own script running in it.
//
// Secure follows the app origin's scheme. Setting it unconditionally would
// make a plain-HTTP deployment (the documented local default) unable to log in
// at all, with a browser silently dropping the cookie as the only symptom.
func (ro *Router) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   ro.cookieSecure(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (ro *Router) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   ro.cookieSecure(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (ro *Router) cookieSecure() bool {
	return strings.HasPrefix(ro.cfg.AppOrigin, "https://")
}

// safeNext keeps a post-login redirect on this origin. Anything that is not a
// single-slash absolute path — an absolute URL, a protocol-relative "//host",
// a backslash some browsers normalize to a slash — is discarded rather than
// sanitized, since the only correct values are paths we emitted ourselves.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") {
		return ""
	}
	// A control character or backslash inside the path is how a browser gets
	// tricked into collapsing "/\t/evil.test" into a scheme-relative
	// "//evil.test" after the checks below have already run — reject them
	// outright rather than trust that url.Parse sees what the browser will.
	for _, r := range next {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return ""
		}
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return ""
	}
	return next
}
