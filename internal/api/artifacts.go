package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/artifact-viewer/artifact-viewer/internal/scanner"
	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (ro *Router) listArtifacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := store.ListOptions{
		Query:  q.Get("q"),
		Offset: 0,
		Limit:  50,
	}
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			opts.Limit = n
		}
	}
	if o := q.Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil {
			opts.Offset = n
		}
	}
	if tags := q.Get("tags"); tags != "" {
		opts.Tags = strings.Split(tags, ",")
	}
	if cols := q.Get("collections"); cols != "" {
		opts.Collections = strings.Split(cols, ",")
	}

	artifacts, err := ro.cfg.Store.ListArtifacts(r.Context(), opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if artifacts == nil {
		artifacts = []*store.Artifact{}
	}
	writeJSON(w, http.StatusOK, artifacts)
}

type createArtifactRequest struct {
	Title            string   `json:"title"`
	Body             string   `json:"body"`
	Tier             store.Tier `json:"tier"`
	NetworkAllowlist []string `json:"network_allowlist"`
}

type createArtifactResponse struct {
	Artifact         *store.Artifact `json:"artifact"`
	NetworkFootprint []string        `json:"network_footprint"`
	RenderURL        string          `json:"render_url"`
}

func (ro *Router) createArtifact(w http.ResponseWriter, r *http.Request) {
	var req createArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Body == "" {
		http.Error(w, "body is required", http.StatusBadRequest)
		return
	}
	if req.Tier == 0 {
		req.Tier = store.Tier1
	}

	// Scan for network footprint
	footprint := scanner.Scan(req.Body)

	ownerID := ownerIDFromCtx(r.Context())
	id := uuid.New().String()
	blobID := uuid.New().String()

	// Store the artifact body
	if err := ro.cfg.Blob.Put(r.Context(), blobID, bytes.NewReader([]byte(req.Body))); err != nil {
		http.Error(w, "failed to store artifact body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	allowlist := req.NetworkAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}

	now := time.Now().UTC()
	a := &store.Artifact{
		ID:               id,
		OwnerID:          ownerID,
		Title:            req.Title,
		SourceBlobID:     blobID,
		Tier:             req.Tier,
		NetworkAllowlist: allowlist,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := ro.cfg.Store.PutArtifact(r.Context(), a); err != nil {
		http.Error(w, "failed to store artifact: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := createArtifactResponse{
		Artifact:         a,
		NetworkFootprint: footprint,
		RenderURL:        ro.cfg.RenderOrigin + "/a/" + id,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (ro *Router) getArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Attach the artifact body if requested
	if r.URL.Query().Get("body") == "true" {
		rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
		if err == nil {
			body, _ := io.ReadAll(rc)
			rc.Close()
			type artifactWithBody struct {
				*store.Artifact
				Body string `json:"body"`
			}
			writeJSON(w, http.StatusOK, artifactWithBody{Artifact: a, Body: string(body)})
			return
		}
	}

	writeJSON(w, http.StatusOK, a)
}

func (ro *Router) updateArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Verify artifact exists
	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := ro.cfg.Store.UpdateArtifact(r.Context(), id, updates); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a, _ = ro.cfg.Store.GetArtifact(r.Context(), id)
	writeJSON(w, http.StatusOK, a)
}

func (ro *Router) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")

	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := ro.cfg.Store.DeleteArtifact(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
