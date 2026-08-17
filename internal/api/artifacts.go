package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/origin"
	"github.com/momja/Exhibit/internal/scanner"
	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/momja/Exhibit/internal/store"
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

// writeArtifactError maps a store error from an artifact mutation to a
// response. ErrNotFound becomes 404, never 403: the store reports an id
// outside the caller's library exactly as it reports one that doesn't exist,
// and the handler must not undo that by answering differently (av-ep8k).
func writeArtifactError(w http.ResponseWriter, r *http.Request, label string, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrNotUpdatable) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	serverError(w, r, label, err)
}

func (ro *Router) listArtifacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := store.ListOptions{
		OwnerID: ownerIDFromCtx(r.Context()),
		Query:   q.Get("q"),
		Offset:  0,
		Limit:   50,
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
// Vendored payloads come back separately, to be stored as blobs of their own
// once the artifact row exists rather than folded into the body. They arrive by
// two routes, because the two passes cannot substitute the same way: the markup
// walker rewrites its references through `sink` as it goes (an <img src> is not
// fetch-loaded, so there is nothing to intercept), while the runtime pass leaves
// the document untouched and is redirected at render.
func snapshotBody(ctx context.Context, pageURL, body string, sink snapshot.AssetSink) (string, []snapshot.RuntimeAsset, *snapshotReport) {
	var assets []snapshot.RuntimeAsset
	report := &snapshotReport{VendoredURLs: []string{}}
	f, err := newSnapshotFetcher(pageURL)
	if err != nil {
		report.Error = err.Error()
		return body, nil, report
	}
	out, fetchErrs, err := snapshot.InlineHTMLAssets(ctx, f, body, sink)
	if err != nil {
		report.Error = err.Error()
		return body, nil, report
	}
	// Second pass, on the same fetcher so both share one budget, one dedupe
	// cache and one Vendored() total: collect the binary payloads the page
	// fetches from JavaScript, which the markup walker above cannot see.
	//
	// Unlike the markup pass this one does not touch the document (av-20fk).
	// The payloads become blobs of their own and the render surface injects
	// the manifest that redirects the fetch, so the stored body keeps the
	// literals it was ingested with. A transform error here is not fatal
	// either — keep the markup-vendored document and report the failures.
	if collected, runtimeErrs, rerr := snapshot.CollectRuntimeAssets(ctx, f, out); rerr == nil {
		assets = collected
		fetchErrs = append(fetchErrs, runtimeErrs...)
	} else {
		report.Error = rerr.Error()
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
	return out, assets, report
}

// persistRuntimeAssets stores each collected payload as its own blob and makes
// them the artifact's current asset generation, draining whatever the previous
// generation left behind.
//
// It runs after the artifact row exists, because an asset row references it.
// A failure here is reported but never fails the ingest: the artifact is
// already stored and usable, and an asset that did not land shows up the same
// way an un-vendored one always has — the page's own fetch reaches the network
// and the allowlist governs it.
func (ro *Router) persistRuntimeAssets(ctx context.Context, ownerID int64, artifactID string, collected []snapshot.RuntimeAsset) error {
	if len(collected) == 0 {
		return nil
	}
	generationID, err := store.NewGenerationID()
	if err != nil {
		return err
	}

	rows := make([]store.ArtifactAsset, 0, len(collected))
	for _, c := range collected {
		// The markup pass already minted one, because the URL it wrote into
		// the document contains it; the runtime pass takes one here.
		assetID := c.AssetID
		if assetID == "" {
			minted, err := store.NewAssetID()
			if err != nil {
				return err
			}
			assetID = minted
		}
		// Content-addressed per owner, so one library that loads the same
		// wasm from two artifacts stores it once — and so deleting one
		// owner's account can never reach another's bytes.
		blobID := store.AssetBlobID(ownerID, c.Body)
		if err := ro.cfg.Blob.Put(ctx, blobID, bytes.NewReader(c.Body)); err != nil {
			return fmt.Errorf("store asset %s: %w", c.SourceURL, err)
		}
		rows = append(rows, store.ArtifactAsset{
			ID:          assetID,
			SourceURL:   c.SourceURL,
			BlobID:      blobID,
			ContentType: c.ContentType,
			SizeBytes:   int64(len(c.Body)),
		})
	}

	queued, err := ro.cfg.Store.ReplaceArtifactAssets(ctx, ownerID, artifactID, generationID, rows)
	if err != nil {
		return err
	}
	ro.reclaimBlobs(ctx, queued)
	return nil
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

	// The artifact id is minted here rather than after the transform, because
	// the markup pass rewrites references to their final asset URLs as it
	// walks — and those URLs contain this id. Nothing is persisted under it
	// until PutArtifact below, so an ingest that fails leaves nothing behind.
	ownerID := ownerIDFromCtx(r.Context())
	id := uuid.New().String()

	var snapReport *snapshotReport
	var runtimeAssets []snapshot.RuntimeAsset
	collector := newAssetCollector(ro.cfg.RenderOrigin, id)
	if req.Snapshot {
		req.Body, runtimeAssets, snapReport = snapshotBody(r.Context(), req.URL, req.Body, collector.sink)
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

	blobID := uuid.New().String()

	// Store the artifact body
	if err := putBlob(r.Context(), ro.cfg.Store, ro.cfg.Blob, blobID, bytes.NewReader([]byte(req.Body))); err != nil {
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
	// Whatever the caller approved must be an origin before it is stored: the
	// allowlist is pasted verbatim into the render CSP, where a path-bearing or
	// oddly-spelled entry means something other than what the approval UI showed
	// (av-i7hd). Rejecting names the value so the client can point at it.
	allowlist, err := origin.NormalizeOrigins(req.NetworkAllowlist)
	if err != nil {
		// JSON, like every other client-facing error the gallery pages read:
		// the page shows data.error, so the named entry reaches the user.
		writeError(w, http.StatusBadRequest, err.Error())
		return
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
		SourceText:       store.ExtractSearchText(req.Body),
	}

	if err := ro.cfg.Store.PutArtifact(r.Context(), a); err != nil {
		serverError(w, r, "store artifact", err)
		return
	}

	// Assets go after the row they reference. Never fatal (av-20fk): a
	// payload that did not land leaves the page's own fetch to reach the
	// network, which is exactly where it was before any of this existed.
	if err := ro.persistRuntimeAssets(r.Context(), ownerID, id, collector.merge(runtimeAssets)); err != nil {
		slog.WarnContext(r.Context(), "store runtime assets",
			slog.String("artifact_id", id), slog.String("err", err.Error()))
		if snapReport != nil {
			snapReport.Failures = append(snapReport.Failures, snapshotFailure{
				Kind: "asset_store", Detail: err.Error(),
			})
		}
	}

	// A create-mode agent session binds here, to the id this handler just
	// wrote (av-e0yj). Binding from the row rather than from the tool result
	// the model sees is what makes the scope unforgeable; from here the
	// session's credential reaches this artifact and nothing else.
	if g := agentGrantFromCtx(r.Context()); g != nil {
		g.BindArtifact(id)
		slog.InfoContext(r.Context(), "agent session bound to the artifact it created",
			slog.String("artifact_id", id))
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
	id := urlParamID(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
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
	id := urlParamID(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// The capability-bridge approval flags (downloads_approved,
	// clipboard_approved, links_approved) are strict booleans; reject anything
	// else up front so a bad PATCH is a 400, not a stored value that later
	// fails to scan.
	for _, field := range []string{"downloads_approved", "clipboard_approved", "links_approved"} {
		if v, ok := updates[field]; ok {
			if _, isBool := v.(bool); !isBool {
				http.Error(w, field+" must be a boolean", http.StatusBadRequest)
				return
			}
		}
	}

	// The allowlist goes through the same origin normalization as ingest, for
	// the same reason: this is the single write path, and the values land in a
	// CSP header (av-i7hd). A rejected entry is named in the 400 so the edit
	// page can point at the row the user typed.
	if v, ok := updates["network_allowlist"]; ok {
		raw, ok := allowlistStrings(v)
		if !ok {
			writeError(w, http.StatusBadRequest, "network_allowlist must be an array of strings")
			return
		}
		normalized, err := origin.NormalizeOrigins(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		updates["network_allowlist"] = normalized
	}

	// Verify the artifact exists *in this owner's library*. Another owner's
	// id reads back as nil here, exactly like an unknown one, so the 404
	// below reveals nothing about which ids exist (av-ep8k).
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
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
			if err := putBlob(r.Context(), ro.cfg.Store, ro.cfg.Blob, a.SourceBlobID, bytes.NewReader([]byte(newBody))); err != nil {
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
			updates["source_text"] = store.ExtractSearchText(newBody)
		}
		delete(updates, "body")
	}

	if err := ro.cfg.Store.UpdateArtifact(r.Context(), ownerID, id, updates); err != nil {
		writeArtifactError(w, r, "update artifact", err)
		return
	}

	a, err = ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "reload artifact after update", err)
		return
	}
	if a == nil {
		serverError(w, r, "reload artifact after update", errors.New("artifact vanished after update"))
		return
	}

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
		Artifact:         a,
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
	Artifact         *store.Artifact `json:"artifact"`
	NetworkFootprint []string        `json:"network_footprint"`
	FootprintChanged bool            `json:"footprint_changed"`
}

// allowlistStrings reads the network_allowlist value out of a decoded PATCH
// body, which arrives as []interface{} from JSON (or []string from Go code in
// tests). A non-list — JSON null included — or a list with a non-string in it
// is the caller's error: clearing the allowlist takes an explicit empty array,
// so a null can never silently wipe it.
func allowlistStrings(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		return list, true
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
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
	id := urlParamID(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())

	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
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
	if err := putBlob(r.Context(), ro.cfg.Store, ro.cfg.Blob, a.SourceBlobID, bytes.NewReader(fetched)); err != nil {
		serverError(w, r, "refetch update body", err)
		return
	}

	// Re-scan the network footprint and bump updated_at. Title is preserved.
	updates := map[string]any{
		"network_allowlist": scanner.Scan(string(fetched)),
		"source_text":       store.ExtractSearchText(string(fetched)),
	}
	if err := ro.cfg.Store.UpdateArtifact(r.Context(), ownerID, id, updates); err != nil {
		writeArtifactError(w, r, "refetch update artifact", err)
		return
	}

	slog.InfoContext(r.Context(), "artifact refetched", slog.String("id", id), slog.Int("body_bytes", len(fetched)))

	a, err = ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "reload artifact after refetch", err)
		return
	}
	if a == nil {
		serverError(w, r, "reload artifact after refetch", errors.New("artifact vanished after refetch"))
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (ro *Router) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())

	// No pre-read to establish that the artifact exists: the delete itself
	// answers ErrNotFound (as 404) for an id this owner cannot see, and the
	// blob ids the handler needs come back from the same call rather than from
	// a row it looked up beforehand — which is also what keeps them consistent
	// with what the transaction actually removed.
	queued, err := ro.cfg.Store.DeleteArtifact(r.Context(), ownerID, id)
	if err != nil {
		writeArtifactError(w, r, "delete artifact", err)
		return
	}

	slog.InfoContext(r.Context(), "artifact deleted", slog.String("id", id))

	// The rows are gone and the bytes are condemned in writing; now remove
	// them (av-8gyd). A 500 here reports a delete that only half happened —
	// the artifact has left the library but its file is still on disk — and
	// that is the honest status even though retrying the request now 404s.
	// What it no longer reports is a *permanent* leak: the queue row survives
	// the failure, so the next startup finishes the job whether or not anybody
	// reads the log.
	if err := ro.reclaimBlobs(r.Context(), queued); err != nil {
		serverError(w, r, "delete artifact blobs", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// reclaimBlobs removes the bytes of the ids a delete operation just queued,
// and retires their queue rows. Call it *after* the transaction that condemned
// them committed.
//
// That ordering is the decision, and it turns on an asymmetry between the two
// failure modes:
//
//   - Bytes first, and a failing row delete leaves a live artifact pointing at
//     a body that no longer exists. It renders an error forever and nothing on
//     the instance can repair it, because the blob was the only copy.
//   - Rows first, and a failing blob delete leaves bytes nobody references.
//     That breaks no row, and since av-8gyd it is not even a leak: the ids sit
//     in pending_blob_deletions until a drain succeeds.
//
// Prefer the recoverable failure: the rows go first. The blob failure is still
// returned rather than swallowed — a deletion that left the bytes on disk must
// not report success.
//
// Only this operation's own ids are passed, never the whole queue, so no
// request ever walks a backlog; the backlog is the startup drain's job
// (cmd/server/main.go).
func (ro *Router) reclaimBlobs(ctx context.Context, queued []string) error {
	if len(queued) == 0 {
		return nil
	}
	_, err := ro.cfg.Store.DrainBlobDeletions(ctx, ro.cfg.Blob, queued)
	return err
}
