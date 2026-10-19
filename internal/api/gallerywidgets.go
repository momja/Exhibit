package api

import "net/http"

// openGalleryWidget refreshes the short-lived render credential when an
// offscreen gallery widget is brought back. Its iframe remains sandboxed and
// the final document is still served exclusively by the render origin.
func (ro *Router) openGalleryWidget(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), id)
	if err != nil {
		serverError(w, r, "gallery widget reload lookup", err)
		return
	}
	if a == nil {
		ro.notFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, ro.renderURLs(r).widget(a.ID), http.StatusFound)
}
