package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/momja/Exhibit/internal/kit"
	"github.com/momja/Exhibit/internal/store"
)

// The component kit routes (Exh-61ix). /api/kit… describes what an artifact
// can build with: read-only and instance-wide, authored by Exhibit, shipped in
// the binary and the same for every caller, so reading it reveals nothing
// about anyone's library. That is why an agent credential may read it
// (kitPathReadable) even though every other route outside its one artifact is
// refused. /api/settings/kit is the owner's library choice, which is library
// data, so it lives outside that prefix and an agent cannot reach it.

// kitView is one kit as the API lists it: what it looks like and which
// contract it draws. What to write is the contract's business, listed once.
type kitView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Contract    string `json:"contract"`
}

// contractView is a contract with the marker an artifact writes to use it.
type contractView struct {
	*kit.Contract
	Marker string `json:"marker"`
}

// listKit answers GET /api/kit: the default kit, every released kit, and the
// contracts they implement.
func (ro *Router) listKit(w http.ResponseWriter, r *http.Request) {
	kits := []kitView{}
	for _, id := range kit.IDs() {
		k, _ := kit.Get(id)
		kits = append(kits, kitView{
			ID:          k.ID(),
			Name:        k.Name,
			Version:     k.Version,
			Description: k.Description,
			Contract:    k.Contract.Version,
		})
	}
	contracts := []contractView{}
	for _, v := range kit.Contracts() {
		c, _ := kit.GetContract(v)
		contracts = append(contracts, contractView{Contract: c, Marker: c.Marker()})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"default":   kit.Default(),
		"kits":      kits,
		"contracts": contracts,
	})
}

// getLibraryKit answers GET /api/settings/kit: the kit the caller chose for
// their library, "" for none, and the default kit that "" means.
func (ro *Router) getLibraryKit(w http.ResponseWriter, r *http.Request) {
	id, err := ro.cfg.Store.LibraryKit(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil {
		serverError(w, r, "library kit: read", err)
		return
	}
	writeLibraryKit(w, id)
}

// setLibraryKit answers PUT /api/settings/kit with {"kit": "paper/v1"}, or
// {"kit": ""} to go back to the default. Every artifact that has no kit of its
// own restyles on its next render; none of their sources change.
func (ro *Router) setLibraryKit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kit *string `json:"kit"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Kit == nil || !store.ValidKitChoice(*body.Kit) {
		writeError(w, http.StatusBadRequest, "kit must be a released kit ID, or empty for the default")
		return
	}
	if err := ro.cfg.Store.SetLibraryKit(r.Context(), ownerIDFromCtx(r.Context()), *body.Kit); err != nil {
		serverError(w, r, "library kit: write", err)
		return
	}
	writeLibraryKit(w, *body.Kit)
}

func writeLibraryKit(w http.ResponseWriter, id string) {
	writeJSON(w, http.StatusOK, map[string]any{"kit": id, "default": kit.Default()})
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

// kitPickerView feeds the component kit select on the profile page (the
// library's kit) and the edit page (one artifact's kit). The empty option is
// "no choice at this level", and Inherit says what that falls back to.
type kitPickerView struct {
	SelectID string
	Inherit  string
	Options  []kitOption
}

type kitOption struct {
	ID          string
	Description string
	Selected    bool
}

func newKitPicker(selectID, selected, inherit string) kitPickerView {
	v := kitPickerView{SelectID: selectID, Inherit: inherit}
	for _, id := range kit.IDs() {
		k, _ := kit.Get(id)
		v.Options = append(v.Options, kitOption{ID: id, Description: k.Description, Selected: id == selected})
	}
	return v
}

// libraryKitLabel names what an artifact with no kit of its own falls back to.
func libraryKitLabel(libraryKit string) string {
	if libraryKit == "" {
		return "Library kit (" + kit.Default() + ", the default)"
	}
	return "Library kit (" + libraryKit + ")"
}
