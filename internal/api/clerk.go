package api

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
)

// Only public configuration reaches templates. The secret key stays in auth.Clerk.
type clerkView struct {
	PublishableKey string
	FrontendAPI    string
	// How long the request's session token has left, in milliseconds, so the
	// page knows how long it may put off starting ClerkJS (Exh-g0fx). Zero
	// when there is none, which starts it at once.
	SessionExpiresInMS int64
}

func (ro *Router) clerkView(r *http.Request) *clerkView {
	if ro.cfg.Clerk == nil {
		return nil
	}
	return &clerkView{ro.cfg.ClerkPublishableKey, ro.cfg.ClerkFrontendAPI, ro.cfg.Clerk.SessionExpiresIn(r).Milliseconds()}
}

func (ro *Router) clerkSessionUser(r *http.Request) (int64, bool) {
	subject := ro.cfg.Clerk.Subject(r)
	if subject == "" {
		return 0, false
	}
	identity, err := ro.cfg.Clerk.Identity(r)
	if err != nil || identity.Subject != subject {
		return 0, false
	}
	u, err := ro.cfg.Store.UpsertHostedUser(r.Context(), subject, identity.Email, identity.SessionCreatedAt)
	if err != nil || u == nil || u.Disabled {
		return 0, false
	}
	return u.ID, true
}

func (ro *Router) renderClerkLogin(w http.ResponseWriter, r *http.Request, mode string) {
	// Clerk relies on secure browser APIs. When deployed behind an HTTPS
	// proxy, move old direct HTTP links to the configured public origin.
	origin, err := url.Parse(ro.cfg.AppOrigin)
	if err == nil && origin.Scheme == "https" && r.Host != origin.Host {
		http.Redirect(w, r, ro.cfg.AppOrigin+r.URL.RequestURI(), http.StatusSeeOther)
		return
	}
	if mode == "" {
		mode = "signin"
		if r.URL.Path == "/auth/signup" {
			mode = "signup"
		}
	}
	next := safeNext(r.URL.Query().Get("next"))
	if next == "" {
		next = "/"
	}
	title := "Sign in"
	if mode == "signup" {
		title = "Create account"
	} else if mode == "logout" {
		title = "Sign out"
	}
	page, err := renderPage("clerkLogin", struct {
		Header  appHeaderView
		Favicon template.URL
		Clerk   *clerkView
		Mode    string
		Next    string
	}{authPageHeader(title), template.URL(exhibitLogoDataURI), ro.clerkView(r), mode, next})
	if err != nil {
		serverError(w, r, "Clerk login render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, page)
}
