package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
)

// The session layer (av-30rj). An identity provider is exchanged exactly once,
// here at the callback, for a session this service owns. Everything after that
// point — the cookie, the per-request lookup, owner_id — is provider-agnostic,
// which is why swapping providers touches one constructor in cmd/server and
// nothing in this file.
//
// When no provider is configured this whole surface is absent: the /auth routes
// are never registered, the gate is a pass-through, and the static token plus
// owner 1 behave exactly as they always have.
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
// False is the default and means single-user: static token, owner 1.
func (ro *Router) identityEnabled() bool { return ro.cfg.Identity != nil }

func (ro *Router) sessionTTL() time.Duration {
	if ro.cfg.SessionTTL > 0 {
		return ro.cfg.SessionTTL
	}
	return DefaultSessionTTL
}

// setupAuthRoutes registers the login flow, and only when a provider exists.
// Registering them unconditionally would change what an unconfigured instance
// answers at /auth/login — today the styled 404 page — for no benefit.
func (ro *Router) setupAuthRoutes(r chi.Router) {
	if !ro.identityEnabled() {
		return
	}
	r.Get("/auth/login", ro.authLogin)
	r.Get("/auth/callback", ro.authCallback)
	// Logout accepts both verbs: a link is the obvious affordance, and
	// revoking a session is not a destructive action a forged request could
	// abuse for anything worse than an inconvenience.
	r.Get("/auth/logout", ro.authLogout)
	r.Post("/auth/logout", ro.authLogout)
}

// authLogin starts Authorization Code + PKCE: mint a state and a verifier,
// park both in short-lived cookies, and hand the browser to the provider.
func (ro *Router) authLogin(w http.ResponseWriter, r *http.Request) {
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

	user, err := ro.cfg.Store.UpsertUser(r.Context(), identity.ExternalID, identity.Email)
	if err != nil {
		ro.failLogin(w, "could not record identity", slog.String("err", err.Error()))
		return
	}

	sid, err := auth.NewSessionID()
	if err != nil {
		ro.failLogin(w, "could not start session")
		return
	}
	sess := &store.Session{
		ID:        sid,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(ro.sessionTTL()),
	}
	if err := ro.cfg.Store.CreateSession(r.Context(), sess); err != nil {
		ro.failLogin(w, "could not start session", slog.String("err", err.Error()))
		return
	}
	ro.setCookie(w, sessionCookieName, sid, ro.sessionTTL())
	slog.Info("login", slog.Int64("user_id", user.ID))

	dest := "/"
	if c, err := r.Cookie(nextCookieName); err == nil {
		if next := safeNext(c.Value); next != "" {
			dest = next
		}
	}
	http.Redirect(w, r, dest, http.StatusFound)
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
func (ro *Router) sessionUser(r *http.Request) (int64, bool) {
	if !ro.identityEnabled() {
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
	if strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return ""
	}
	return next
}
