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
	"golang.org/x/net/html"
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
	Title            string     `json:"title"`
	Body             string     `json:"body"`
	URL              string     `json:"url"`
	Tier             store.Tier `json:"tier"`
	NetworkAllowlist []string   `json:"network_allowlist"`
}

// extractTitle pulls the text content of the first <title> element from HTML.
func extractTitle(body string) string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return ""
	}
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = n.FirstChild.Data
			return
		}
		for c := n.FirstChild; c != nil && title == ""; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.TrimSpace(title)
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

	// Fetch from URL if no body provided
	if req.URL != "" && req.Body == "" {
		resp, err := http.Get(req.URL) //nolint:noctx
		if err != nil {
			http.Error(w, "failed to fetch URL: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer resp.Body.Close()
		fetched, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if err != nil {
			http.Error(w, "failed to read URL content: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.Body = string(fetched)
		if req.Title == "" {
			if t := extractTitle(req.Body); t != "" {
				req.Title = t
			} else {
				req.Title = req.URL
			}
		}
	}

	if req.Body == "" {
		http.Error(w, "body or url is required", http.StatusBadRequest)
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

	// The allowlist holds only origins the user has explicitly approved; it is
	// NEVER seeded from the scan. The scanned footprint is returned to the
	// caller (network_footprint) as transparency so the user can review and
	// approve origins before any network access is granted. Until an origin is
	// approved the render CSP stays connect-src 'none' and the artifact is
	// network-inert. See spec §6.2 ("Nothing is rendered with network access
	// until they decide").
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

	// Handle body update: overwrite the blob and re-scan network footprint.
	if bodyVal, ok := updates["body"]; ok {
		bodyStr, _ := bodyVal.(string)
		if bodyStr != "" {
			if err := ro.cfg.Blob.Put(r.Context(), a.SourceBlobID, bytes.NewReader([]byte(bodyStr))); err != nil {
				http.Error(w, "failed to update artifact body: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// Do NOT auto-add newly scanned origins to the allowlist — approval
			// is an explicit user action. Existing approved origins are kept as
			// they are; origins introduced by the edited body surface via the
			// footprint / runtime prompt and must be approved before they gain
			// network access. See spec §6.2.
		}
		delete(updates, "body")
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
