package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/momja/Exhibit/internal/kit"
)

// The component kit routes (Exh-61ix) describe what an artifact can build
// with. They are read-only and instance-wide: the kit is authored by Exhibit,
// ships in the binary, and is the same for every caller, so reading it reveals
// nothing about anyone's library. That is why an agent credential may read it
// (kitPathReadable) even though every other route outside its one artifact is
// refused.

// kitVersionView is one version as the API lists it: everything the agent
// needs to choose components, without any stylesheet.
type kitVersionView struct {
	Version     string          `json:"version"`
	Marker      string          `json:"marker"`
	Description string          `json:"description"`
	Base        kit.Base        `json:"base"`
	Tokens      []kit.Token     `json:"tokens"`
	Components  []kit.Component `json:"components"`
}

func newKitVersionView(k *kit.Kit) kitVersionView {
	components := make([]kit.Component, len(k.Components))
	for i, c := range k.Components {
		c.CSS = ""
		components[i] = c
	}
	return kitVersionView{
		Version:     k.Version,
		Marker:      k.Marker(),
		Description: k.Description,
		Base:        k.Base,
		Tokens:      k.Tokens,
		Components:  components,
	}
}

// listKit answers GET /api/kit: the latest version's name and every released
// version's index, newest first.
func (ro *Router) listKit(w http.ResponseWriter, r *http.Request) {
	versions := kit.Versions()
	views := make([]kitVersionView, 0, len(versions))
	for i := len(versions) - 1; i >= 0; i-- {
		k, _ := kit.Get(versions[i])
		views = append(views, newKitVersionView(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"latest":   kit.Latest(),
		"versions": views,
	})
}

// getKitComponent answers GET /api/kit/{version}/components/{name}: one
// component's documentation and its stylesheet.
func (ro *Router) getKitComponent(w http.ResponseWriter, r *http.Request) {
	k, ok := kit.Get(chi.URLParam(r, "version"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such kit version")
		return
	}
	c, ok := k.Component(chi.URLParam(r, "name"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such kit component")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   k.Version,
		"component": c,
	})
}

// kitPathReadable reports whether an agent credential may make this request
// to the kit routes: GET only, and only under /api/kit.
func kitPathReadable(method, escapedPath string) bool {
	if method != http.MethodGet {
		return false
	}
	return escapedPath == "/api/kit" || strings.HasPrefix(escapedPath, "/api/kit/")
}
