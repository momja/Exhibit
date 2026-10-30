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

// kitView is one kit as the API lists it: what it looks like and the marker
// that pins it. What to write is its contract's business, listed once per
// contract rather than once per kit.
type kitView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Marker      string `json:"marker"`
	Description string `json:"description"`
	Contract    string `json:"contract"`
}

// listKit answers GET /api/kit: the default kit, every released kit, and the
// contracts they implement.
func (ro *Router) listKit(w http.ResponseWriter, r *http.Request) {
	kits := []kitView{}
	contracts := []*kit.Contract{}
	seen := map[string]bool{}
	for _, id := range kit.IDs() {
		k, _ := kit.Get(id)
		kits = append(kits, kitView{
			ID:          k.ID(),
			Name:        k.Name,
			Version:     k.Version,
			Marker:      k.Marker(),
			Description: k.Description,
			Contract:    k.Contract.Version,
		})
		if !seen[k.Contract.Version] {
			seen[k.Contract.Version] = true
			contracts = append(contracts, k.Contract)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"default":   kit.Default(),
		"kits":      kits,
		"contracts": contracts,
	})
}

// getKitComponent answers GET /api/kit/{name}/{version}/components/{component}:
// one component's documentation and that kit's stylesheet for it.
func (ro *Router) getKitComponent(w http.ResponseWriter, r *http.Request) {
	k, ok := kit.Get(chi.URLParam(r, "name") + "/" + chi.URLParam(r, "version"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such kit")
		return
	}
	c, ok := k.Component(chi.URLParam(r, "component"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such kit component")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kit":       k.ID(),
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
