package api

import (
	"net/http"
	"net/url"

	"github.com/momja/Exhibit/internal/store"
)

type editNavigation struct {
	URL   string
	Label string
}

func editURL(id, returnTo string) string {
	return "/artifacts/" + id + "/edit?" + url.Values{"return_to": {returnTo}}.Encode()
}

// Only a library or this artifact can be the editor's return destination.
// Carry it in the link so reloads and new tabs keep the same task context.
func editorReturn(r *http.Request, artifact *store.Artifact) editNavigation {
	candidate := r.URL.Query().Get("return_to")
	if !r.URL.Query().Has("return_to") {
		if ref, err := url.Parse(r.Referer()); err == nil && ref.Host == r.Host && (ref.Scheme == "http" || ref.Scheme == "https") {
			candidate = ref.RequestURI()
		}
	}
	u, err := url.Parse(candidate)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.RawPath != "" {
		return editNavigation{URL: "/", Label: "My library"}
	}
	var label string
	switch u.Path {
	case "/":
		label = "My library"
	case "/shared":
		label = "Shared with me"
	case "/shared-by-me":
		label = "Shared by me"
	case "/artifacts/" + artifact.ID:
		label = artifact.Title
	default:
		return editNavigation{URL: "/", Label: "My library"}
	}
	return editNavigation{URL: u.String(), Label: label}
}

func (ro *Router) editVersionsPartial(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("artifact")
	artifact, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), id)
	if err != nil {
		serverError(w, r, "edit versions lookup", err)
		return
	}
	if artifact == nil {
		ro.notFound(w, r)
		return
	}
	versions, err := ro.cfg.Store.ListVersions(r.Context(), ownerIDFromCtx(r.Context()), id)
	if err != nil {
		serverError(w, r, "edit versions", err)
		return
	}
	page, err := renderPage("editVersionRows", struct {
		ID       string
		Versions []versionView
	}{ID: id, Versions: newVersionViews(versions)})
	if err != nil {
		serverError(w, r, "edit versions render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}
