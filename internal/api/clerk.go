package api

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/momja/Exhibit/internal/store"
)

// Only public configuration reaches templates. The secret key stays in auth.Clerk.
type clerkView struct {
	PublishableKey string
	FrontendAPI    string
}

func (ro *Router) clerkView() *clerkView {
	if ro.cfg.Clerk == nil {
		return nil
	}
	return &clerkView{ro.cfg.ClerkPublishableKey, ro.cfg.ClerkFrontendAPI}
}

func (ro *Router) clerkSessionUser(r *http.Request) (int64, bool) {
	subject := ro.cfg.Clerk.Subject(r)
	if subject == "" {
		return 0, false
	}
	u, err := ro.cfg.Store.GetUserByExternalID(r.Context(), subject)
	if errors.Is(err, store.ErrNotFound) {
		// Fetch the email once, for a new identity. Never link accounts by email:
		// Clerk's stable user ID is the identity and Exhibit retains ownership IDs.
		email, fetchErr := ro.cfg.Clerk.Email(r.Context(), subject)
		if fetchErr != nil {
			return 0, false
		}
		u, err = ro.cfg.Store.UpsertUser(r.Context(), subject, email)
	}
	if err != nil || u == nil || u.Disabled {
		return 0, false
	}
	return u.ID, true
}

func (ro *Router) renderClerkLogin(w http.ResponseWriter, r *http.Request, mode string) {
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
	page, err := renderPage("clerkLogin", struct {
		Favicon template.URL
		LogoSVG template.HTML
		Clerk   *clerkView
		Mode    string
		Next    string
	}{template.URL(exhibitLogoDataURI), template.HTML(exhibitLogoSVG), ro.clerkView(), mode, next})
	if err != nil {
		serverError(w, r, "Clerk login render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, page)
}
