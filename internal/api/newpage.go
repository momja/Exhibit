// The add-artifact page (/new).
package api

import (
	"fmt"
	"html/template"
	"net/http"
)

// galleryNew serves the add-artifact page (av-qo0j). It reads nothing from the
// store: ingest is entirely a client-side conversation with POST
// /api/artifacts, so the page needs only the API token its script posts with.
func (ro *Router) galleryNew(w http.ResponseWriter, r *http.Request) {
	page, err := renderNewPage(ro.pageCredentials(r))
	if err != nil {
		serverError(w, r, "gallery new render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

// newPageData feeds the add-artifact page. It is deliberately thin: the page
// creates artifacts through the API like any other client, so the only
// per-request values it needs are the credential its script posts with and
// whether it is allowed to post at all.
type newPageData struct {
	Favicon template.URL
	pageCredentials
}

func renderNewPage(creds pageCredentials) (string, error) {
	return renderPage("new", newPageData{
		Favicon:         template.URL(exhibitLogoDataURI),
		pageCredentials: creds,
	})
}
