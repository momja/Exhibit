package api

import (
	"fmt"
	"net/http"
)

type assetPanelView struct {
	Loaded     bool
	ArtifactID string
	assetListResponse
}

func (ro *Router) assetPanelPartial(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("artifact")
	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		fragmentServerError(w, r, "asset panel lookup", err)
		return
	}
	if a == nil {
		fragmentNotFound(w)
		return
	}
	assets, err := ro.cfg.Store.ListArtifactAssets(r.Context(), ownerID, id)
	if err != nil {
		fragmentServerError(w, r, "asset panel list", err)
		return
	}
	fragment, err := renderPage("assetPanelRows", assetPanelView{Loaded: true, ArtifactID: id, assetListResponse: toAssetViews(assets)})
	if err != nil {
		fragmentServerError(w, r, "asset panel render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, fragment)
}
