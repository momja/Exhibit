package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

type contextKey string

const ownerIDKey contextKey = "ownerID"

const defaultOwnerID int64 = 1

// authMiddleware authenticates an API request and, when it can, says who made
// it.
//
// Two credentials are accepted, in this order:
//
//  1. A session cookie, when an identity provider is configured (av-30rj).
//     Browser requests carry it automatically, so a logged-in user's page JS
//     is attributed to that user rather than to whatever token the page was
//     rendered with. The session is looked up per request, which is what makes
//     logout immediate.
//  2. The static bearer token — the API/CLI credential, and the only
//     credential a single-user instance has.
//
// With no provider configured the session branch is unreachable and this is
// byte-for-byte the check it has always been.
func (ro *Router) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ownerID, ok := ro.sessionUser(r); ok {
			ctx := context.WithValue(r.Context(), ownerIDKey, ownerID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if ro.cfg.AuthToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+ro.cfg.AuthToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ownerMiddleware supplies the owner for requests authMiddleware did not
// attribute to a session — token-authenticated API clients, and every request
// on a single-user instance. It never overwrites an owner already resolved
// upstream.
func ownerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value(ownerIDKey).(int64); ok {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), ownerIDKey, defaultOwnerID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ownerIDFromCtx(ctx context.Context) int64 {
	if v, ok := ctx.Value(ownerIDKey).(int64); ok {
		return v
	}
	return defaultOwnerID
}

// publicPathPrefixes are the app-origin paths the login gate must never guard:
// the login flow itself (guarding it would be a redirect loop), the static
// assets a login page would need, the API group (which authenticates itself,
// and answers JSON clients rather than redirecting them), and the public share
// route, whose share row *is* its authorization.
var publicPathPrefixes = []string{
	"/auth/",
	"/assets/",
	"/api/",
	"/s/",
	"/manifest.json",
	"/favicon",
}

// sessionGate protects the server-rendered pages once an identity provider is
// configured, sending an unauthenticated visitor to the provider and back to
// the page they asked for.
//
// With no provider configured it is a pass-through — an unconfigured instance
// has no login to send anyone to, and its pages stay exactly as open as they
// have always been.
func (ro *Router) sessionGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ro.identityEnabled() || isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := ro.sessionUser(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		// Only a top-level navigation can survive a trip to the provider.
		// A fetch (htmx fragment, page JS) gets a status its caller can
		// act on instead of an opaque redirect to an HTML login page.
		if r.Method != http.MethodGet || r.Header.Get("HX-Request") != "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		dest := "/auth/login"
		if next := safeNext(r.URL.RequestURI()); next != "" {
			dest += "?next=" + url.QueryEscape(next)
		}
		http.Redirect(w, r, dest, http.StatusFound)
	})
}

func isPublicPath(path string) bool {
	for _, prefix := range publicPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
