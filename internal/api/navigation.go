package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
)

func (ro *Router) connectionPolicy() *connectors.Service {
	if ro.cfg.Connections != nil {
		return ro.cfg.Connections
	}
	return ro.cfg.ConnectionPolicy
}

// Retained provenance keeps saved-data sharing and private-frame controls
// available after a credential is removed. It does not restrict network access.
func (ro *Router) hasConnectionDataPolicies(ctx context.Context, viewer int64, artifact string) (bool, error) {
	policy := ro.connectionPolicy()
	if policy == nil {
		return false, nil
	}
	policies, err := policy.DataPolicies(ctx, viewer, artifact)
	return len(policies) > 0, err
}

func (ro *Router) renderDocumentPolicy(ctx context.Context, viewer int64, artifact string) (string, bool, error) {
	protected, err := ro.hasConnectionDataPolicies(ctx, viewer, artifact)
	if err != nil || !protected {
		return "", protected, err
	}
	a, err := ro.cfg.Store.GetArtifactReadableBy(ctx, store.ViewerID(viewer), artifact)
	if err != nil {
		return "", true, err
	}
	if a == nil {
		return "", true, store.ErrNotFound
	}
	// Code edits retain the same data boundary. Keep their path stable so an
	// agent preview can update under its already-enforced exact frame policy.
	// The app's separate API authority still includes the actual source blob.
	dataBoundary := *a
	dataBoundary.SourceBlobID = ""
	policy, err := ro.relayPolicy(ctx, viewer, &dataBoundary)
	if err != nil {
		return "", true, err
	}
	// This one-way fingerprint can reach the frame; the app's authority
	// accepted by relay, navigation and state endpoints must never do so.
	digest := sha256.Sum256([]byte("exhibit/render-policy/" + policy))
	return hex.EncodeToString(digest[:]), true, nil
}

func trustedFrameRefresh(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Dest") == "iframe" && r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Reopen the artifact through Exhibit to refresh its access.", http.StatusForbidden)
		return false
	}
	return true
}

func (ro *Router) connectionDocumentProtected(ctx context.Context, viewer int64, artifact string, publicShare bool) (bool, error) {
	if publicShare {
		// Publishing intentionally discloses the owner's data. Verify that
		// every current owner grant carries the required sharing consent.
		if policy := ro.connectionPolicy(); policy != nil {
			return false, policy.ValidateArtifactSharing(ctx, viewer, artifact)
		}
		return false, nil
	}
	return ro.hasConnectionDataPolicies(ctx, viewer, artifact)
}

// Link capability consent is independent of connection grants. Check the
// current host authority and validate the destination before each activation.
func (ro *Router) checkArtifactNavigation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !ro.relayHumanRequest(w, r) {
		return
	}
	viewer := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), store.ViewerID(viewer), urlParamID(r, "artifactID"))
	if err != nil {
		serverError(w, r, "navigation artifact", err)
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if !ro.requireRelayPolicy(w, r, a) {
		return
	}
	var input struct {
		URL string `json:"url"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	target, err := url.Parse(input.URL)
	if err != nil || len(input.URL) > 8192 || strings.TrimSpace(input.URL) != input.URL ||
		target.User != nil || target.Host == "" || (target.Scheme != "https" && target.Scheme != "http") {
		writeError(w, http.StatusBadRequest, "Choose an HTTP or HTTPS link without credentials.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": target.String()})
}
