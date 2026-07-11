package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/artifact-viewer/artifact-viewer/internal/scanner"
	"github.com/artifact-viewer/artifact-viewer/internal/snapshot"
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

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverError logs err at error level with the operation label and request
// context, then responds 500. The label makes the log line greppable without
// a stack; the response body keeps the raw error to preserve existing client
// behavior. This is the seam that turns a bare 500 into diagnosable feedback
// in test environments (the request middleware already records the status).
func serverError(w http.ResponseWriter, r *http.Request, label string, err error) {
	slog.ErrorContext(r.Context(), label,
		slog.String("err", err.Error()),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
	)
	http.Error(w, err.Error(), http.StatusInternalServerError)
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
		serverError(w, r, "list artifacts", err)
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
	Snapshot         bool       `json:"snapshot"`
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
	Snapshot         *snapshotReport `json:"snapshot,omitempty"`
	RenderURL        string          `json:"render_url"`
}

// snapshotReport tells the caller what the snapshot transform did with their
// URL ingest: which assets were vendored into the document, which references
// still point at the network, and which assets could not be inlined. Partial
// failure is data here, never an ingest error — the user always gets a usable
// artifact plus this report (exhibit-lwb.6). ResidualOrigins duplicates the
// response's network_footprint so the report is self-contained; those origins
// feed the same explicit-approval flow and never seed the allowlist.
type snapshotReport struct {
	Applied         bool              `json:"applied"`
	Error           string            `json:"error,omitempty"` // why Applied is false
	VendoredURLs    []string          `json:"vendored_urls"`
	VendoredBytes   int64             `json:"vendored_bytes"`
	ResidualOrigins []string          `json:"residual_origins"`
	Failures        []snapshotFailure `json:"failures,omitempty"`
}

// snapshotFailure is one asset the snapshot could not inline. The reference
// survives verbatim in the stored document, so with the injected <base href>
// it still resolves to its real origin — reachable once that origin is
// approved onto the allowlist.
type snapshotFailure struct {
	Ref    string `json:"ref"`
	URL    string `json:"url,omitempty"`
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}

// newSnapshotFetcher builds the bounded asset fetcher for one snapshot run.
// It is a seam for ingest tests, which swap in snapshot.NewFetcherForTests:
// the production fetcher's SSRF dial guard refuses non-public addresses, and
// httptest fixture servers live on loopback.
var newSnapshotFetcher = func(pageURL string) (*snapshot.Fetcher, error) {
	return snapshot.NewFetcher(pageURL, snapshot.DefaultLimits())
}

// snapshotBody vendors body's external assets into the document so the stored
// artifact is self-contained (exhibit-lwb). It never fails the ingest: on a
// transform-level error the original body comes back with Applied=false and
// the caller's <base href> fallback keeps relative references resolving, while
// per-asset failures are recorded in the report with the rest of the page
// still vendored. ResidualOrigins is left for the caller, which computes it
// from the final document.
func snapshotBody(ctx context.Context, pageURL, body string) (string, *snapshotReport) {
	report := &snapshotReport{VendoredURLs: []string{}}
	f, err := newSnapshotFetcher(pageURL)
	if err != nil {
		report.Error = err.Error()
		return body, report
	}
	out, fetchErrs, err := snapshot.InlineHTMLAssets(ctx, f, body)
	if err != nil {
		report.Error = err.Error()
		return body, report
	}
	report.Applied = true
	report.VendoredURLs, report.VendoredBytes = f.Vendored()
	for _, fe := range fetchErrs {
		fail := snapshotFailure{Ref: fe.Ref, URL: fe.URL, Kind: string(fe.Kind)}
		if fe.Err != nil {
			fail.Detail = fe.Err.Error()
		}
		report.Failures = append(report.Failures, fail)
	}
	return out, report
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
			slog.WarnContext(r.Context(), "ingest fetch failed", slog.String("url", req.URL), slog.String("err", err.Error()))
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
	if req.Snapshot && req.URL == "" {
		http.Error(w, "snapshot requires a source url", http.StatusBadRequest)
		return
	}
	if req.Tier == 0 {
		req.Tier = store.Tier1
	}

	var snapReport *snapshotReport
	if req.Snapshot {
		req.Body, snapReport = snapshotBody(r.Context(), req.URL, req.Body)
	}

	// Scan for network footprint. A URL ingest resolves relative references
	// against the source page so residual origins surface for approval. The
	// scan runs before the <base href> injection below, so the fallback tag
	// itself is never reported as network egress — a fully vendored artifact
	// keeps its empty footprint.
	var footprint []string
	if req.URL != "" {
		footprint = scanner.ScanWithBase(req.Body, req.URL)
		// Option A fallback (exhibit-lwb.6): relative references that survive
		// ingest — snapshot off, failed, or partial — would otherwise resolve
		// against the render origin and 404. The injected base points them
		// back at the source site; whether that origin is reachable stays the
		// allowlist's decision.
		req.Body = snapshot.InjectBaseHref(req.Body, req.URL)
	} else {
		footprint = scanner.Scan(req.Body)
	}
	if snapReport != nil {
		snapReport.ResidualOrigins = footprint
	}

	ownerID := ownerIDFromCtx(r.Context())
	id := uuid.New().String()
	blobID := uuid.New().String()

	// Store the artifact body
	if err := ro.cfg.Blob.Put(r.Context(), blobID, bytes.NewReader([]byte(req.Body))); err != nil {
		serverError(w, r, "store artifact body", err)
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
		SourceURL:        req.URL,
		Tier:             req.Tier,
		NetworkAllowlist: allowlist,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := ro.cfg.Store.PutArtifact(r.Context(), a); err != nil {
		serverError(w, r, "store artifact", err)
		return
	}

	slog.DebugContext(r.Context(), "artifact created",
		slog.String("id", id),
		slog.String("title", req.Title),
		slog.Int("body_bytes", len(req.Body)),
		slog.Any("footprint", footprint),
		slog.Any("allowlist", allowlist),
		slog.Int("tier", int(req.Tier)),
	)

	resp := createArtifactResponse{
		Artifact:         a,
		NetworkFootprint: footprint,
		Snapshot:         snapReport,
		RenderURL:        ro.cfg.RenderOrigin + "/a/" + id,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (ro *Router) getArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		serverError(w, r, "get artifact", err)
		return
	}
	if a == nil {
		slog.DebugContext(r.Context(), "artifact not found", slog.String("id", id))
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
		serverError(w, r, "get artifact for update", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Handle body update: capture the previous body before overwriting (so the
	// post-edit can be diffed against it), write the new blob, and re-scan.
	var newBody, oldBody string
	bodySet := false
	if bodyVal, ok := updates["body"]; ok {
		if bodyStr, ok := bodyVal.(string); ok && bodyStr != "" {
			newBody = bodyStr
			bodySet = true
			// Read the previous body before it is overwritten so the edit
			// dialog can tell whether the network footprint actually changed.
			if rc, gerr := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID); gerr == nil {
				if prev, perr := io.ReadAll(rc); perr == nil {
					oldBody = string(prev)
				}
				rc.Close()
			}
			if err := ro.cfg.Blob.Put(r.Context(), a.SourceBlobID, bytes.NewReader([]byte(newBody))); err != nil {
				serverError(w, r, "update artifact body", err)
				return
			}
			slog.DebugContext(r.Context(), "artifact body rewritten",
				slog.String("id", id), slog.Int("body_bytes", len(newBody)))
			// Do NOT auto-add newly scanned origins to the allowlist — approval
			// is an explicit user action. Existing approved origins are kept as
			// they are; origins introduced by the edited body surface via the
			// footprint / runtime prompt and must be approved before they gain
			// network access. See spec §6.2.
		}
		delete(updates, "body")
	}

	if err := ro.cfg.Store.UpdateArtifact(r.Context(), id, updates); err != nil {
		serverError(w, r, "update artifact", err)
		return
	}

	a, _ = ro.cfg.Store.GetArtifact(r.Context(), id)

	// Re-execute the network scan when the body actually changed (a diff
	// against the previous version), and surface the footprint — and whether
	// it differs from before — so the edit dialog can re-run the explicit
	// approval flow the way ingest does. Edits that don't touch the body, or
	// that leave the network footprint unchanged, report no change and stay on
	// the existing allowlist. The allowlist itself is never seeded from here.
	var footprint []string
	footprintChanged := false
	if bodySet && newBody != oldBody {
		footprint = scanner.Scan(newBody)
		footprintChanged = !sameOrigins(footprint, scanner.Scan(oldBody))
	}
	if footprint == nil {
		footprint = []string{}
	}

	writeJSON(w, http.StatusOK, updateArtifactResponse{
		Artifact:          a,
		NetworkFootprint: footprint,
		FootprintChanged: footprintChanged,
	})
}

// updateArtifactResponse is the PATCH /api/artifacts/:id body. The artifact
// is always present; network_footprint/footprint_changed are only meaningful
// when the request rewrote the body. They let the edit dialog re-run the
// explicit-origin approval flow when an edit changes the network footprint,
// mirroring the two-step ingest scan→approval gate (spec §6.2) without ever
// seeding the allowlist from a scan.
type updateArtifactResponse struct {
	Artifact          *store.Artifact `json:"artifact"`
	NetworkFootprint  []string        `json:"network_footprint"`
	FootprintChanged  bool            `json:"footprint_changed"`
}

// sameOrigins reports whether two origin lists describe the same set,
// disregarding order and duplicates — used to tell whether an edit changed the
// network footprint at all, so an unchanged origin set doesn't re-prompt.
func sameOrigins(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, o := range a {
		seen[o] = struct{}{}
	}
	for _, o := range b {
		if _, ok := seen[o]; !ok {
			return false
		}
	}
	return true
}

// refetchArtifact re-fetches the current HTML/CSS/JS from an artifact's source
// URL and overwrites the stored body with that fresh snapshot. This is a
// destructive snapshot replace — not versioned, no history. The network
// allowlist is re-scanned from the new content; the title is left untouched.
func (ro *Router) refetchArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")

	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		serverError(w, r, "get artifact for refetch", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if a.SourceURL == "" {
		http.Error(w, "artifact has no source URL to re-fetch from", http.StatusBadRequest)
		return
	}

	// Fetch the latest content, mirroring the createArtifact fetch pattern.
	resp, err := http.Get(a.SourceURL) //nolint:noctx
	if err != nil {
		slog.WarnContext(r.Context(), "refetch failed", slog.String("id", id), slog.String("url", a.SourceURL), slog.String("err", err.Error()))
		http.Error(w, "failed to fetch URL: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()
	fetched, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		http.Error(w, "failed to read URL content: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Overwrite the existing blob with the fresh snapshot.
	if err := ro.cfg.Blob.Put(r.Context(), a.SourceBlobID, bytes.NewReader(fetched)); err != nil {
		serverError(w, r, "refetch update body", err)
		return
	}

	// Re-scan the network footprint and bump updated_at. Title is preserved.
	updates := map[string]any{"network_allowlist": scanner.Scan(string(fetched))}
	if err := ro.cfg.Store.UpdateArtifact(r.Context(), id, updates); err != nil {
		serverError(w, r, "refetch update artifact", err)
		return
	}

	slog.InfoContext(r.Context(), "artifact refetched", slog.String("id", id), slog.Int("body_bytes", len(fetched)))

	a, _ = ro.cfg.Store.GetArtifact(r.Context(), id)
	writeJSON(w, http.StatusOK, a)
}

func (ro *Router) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")

	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		serverError(w, r, "get artifact for delete", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := ro.cfg.Store.DeleteArtifact(r.Context(), id); err != nil {
		serverError(w, r, "delete artifact", err)
		return
	}

	slog.InfoContext(r.Context(), "artifact deleted", slog.String("id", id))

	w.WriteHeader(http.StatusNoContent)
}
