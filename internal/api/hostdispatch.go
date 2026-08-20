package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// envSingleListener asks for both surfaces on one port, dispatched by Host
// (av-xath).
const envSingleListener = "SINGLE_LISTENER"

// SingleListenerFromEnv reports whether this process should serve the app and
// render surfaces from one listener instead of two.
//
// Absent means absent, in the OIDC_ISSUER shape: unset leaves the two listeners
// exactly as they were, which is what every existing deployment has and what
// an operator-supplied proxy expects (deployment.md §5). Only a platform whose
// proxy routes by port rather than by Host — Fly.io is the case this exists
// for — has to opt in.
func SingleListenerFromEnv() bool { return envBool(envSingleListener) }

// NewHostDispatcher serves the app surface and the render surface from a single
// listener, choosing between them by each request's Host header.
//
// Normally the *port* is the discriminator: the process binds ADDR and
// RENDER_ADDR, and the operator's proxy maps one hostname to each. A platform
// proxy that routes by port cannot do that — every hostname it terminates
// arrives on one internal port — so the two origins would land on one handler
// and collapse into one origin. That is not a routing inconvenience: the origin
// split *is* the artifact sandbox boundary (architecture.md §3.2, §4), so a
// collapsed boundary puts /api/* on the origin where artifact code runs.
//
// The Host header survives that flattening and is therefore what this dispatches
// on. Two decisions in it are load-bearing:
//
// Only RENDER_ORIGIN's host is matched, and *everything else* falls through to
// the app. The render surface must be reachable at exactly one name and no
// other, whereas the app surface being reachable by container IP or by a
// platform-assigned hostname (appname.fly.dev) is merely untidy — it is
// authenticated either way. Defaulting the other direction would serve
// artifacts to anything that connected without a recognized Host.
//
// And a collapsed boundary is refused here rather than served. Same host for
// both origins yields a process that runs, answers, and is silently insecure;
// the only honest response is to not start, because the alternative is
// discovering it by noticing that an artifact can reach the API.
func NewHostDispatcher(app, render http.Handler, appOrigin, renderOrigin string) (http.Handler, error) {
	appHost, err := originHost(appOrigin)
	if err != nil {
		return nil, fmt.Errorf("APP_ORIGIN: %w", err)
	}
	renderHost, err := originHost(renderOrigin)
	if err != nil {
		return nil, fmt.Errorf("RENDER_ORIGIN: %w", err)
	}
	// Ports are stripped before this comparison, and deliberately: with one
	// listener a port cannot distinguish anything, so two origins differing
	// only by port are two origins that cannot both be served here.
	if appHost == renderHost {
		return nil, fmt.Errorf(
			"%s needs APP_ORIGIN and RENDER_ORIGIN on different hostnames, and both are %q; "+
				"serving artifacts from the app's own origin would put the API inside the artifact sandbox",
			envSingleListener, appHost)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestHost(r) == renderHost {
			render.ServeHTTP(w, r)
			return
		}
		app.ServeHTTP(w, r)
	}), nil
}

// originHost is the lowercased hostname of a configured origin, without scheme
// or port. A scheme is required rather than guessed at: url.Parse reads a bare
// "artifacts.example.com" as a *path* with no host at all, so accepting one
// would produce an empty discriminator that silently matches nothing and sends
// every request to the app surface.
func originHost(origin string) (string, error) {
	// Distinguished from a malformed value because the causes differ: an unset
	// origin is a deployment that never supplied one, and on a platform whose
	// proxy hands us a hostname we do not otherwise know, there is nothing
	// sensible to fall back to.
	if strings.TrimSpace(origin) == "" {
		return "", errors.New("is not set")
	}
	u, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", origin, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("%q is not an absolute origin (want scheme://host)", origin)
	}
	return strings.ToLower(stripPort(u.Host)), nil
}

// requestHost is the lowercased hostname the client asked for, without port.
func requestHost(r *http.Request) string {
	return strings.ToLower(stripPort(r.Host))
}

// stripPort drops a trailing :port, leaving IPv6 literals and bare hosts alone.
// net.SplitHostPort reports an error for a host that carries no port, which is
// the ordinary case here rather than a failure.
func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return strings.Trim(host, "[]")
}
