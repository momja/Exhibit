// The app-origin HTML 404 page.
package api

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

// notFound serves the app's HTML 404 (av-at2v). It is both the mux's fallback
// for unrouted paths and what the gallery pages call for a missing artifact,
// so a 404 looks the same however it was arrived at.
//
// /api/* is deliberately excluded: chi propagates this handler into every
// subrouter, and those routes have JSON/text clients that must keep getting
// the plain error they always got, not a page.
func (ro *Router) notFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	page, err := renderNotFoundPage(r.URL.Path)
	if err != nil {
		serverError(w, r, "not found render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, page)
}

// notFoundPageData feeds the 404 page. RequestedPath is attacker-controlled
// URL text — html/template's contextual escaping is the whole defence, so it
// must reach the page as a template value and never as concatenated markup.
// LogoImage is the brand mark as an image source: the page inlines the SVG
// once in its header and draws the hero frame from the data URI, so one
// artwork serves both without duplicating its element ids (logo.go).
type notFoundPageData struct {
	Favicon       template.URL
	LogoSVG       template.HTML
	LogoImage     template.URL
	RequestedPath string
}

func renderNotFoundPage(requestedPath string) (string, error) {
	return renderPage("notfound", notFoundPageData{
		Favicon:       template.URL(exhibitLogoDataURI),
		LogoSVG:       template.HTML(exhibitLogoSVG),
		LogoImage:     template.URL(exhibitLogoDataURI),
		RequestedPath: requestedPath,
	})
}
