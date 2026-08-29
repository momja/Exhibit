package api

// The self-contained export endpoint (av-vnkt).
//
// PRD §7 promises a one-file `.html` an artifact's owner can email, archive, or
// open years later with no service in the loop. That promise became load-bearing
// when av-20fk and av-oz40 moved artifact payloads out of the body: inside the
// service a URL is the better representation, but a file that needs this
// instance alive is not a file. This is where the two are reconciled.

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/export"
	"github.com/momja/Exhibit/internal/store"
)

// exportArtifact serves one artifact as a single self-contained document.
func (ro *Router) exportArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())

	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "export: load artifact", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	body, err := ro.readBlobString(r.Context(), a.SourceBlobID)
	if err != nil {
		serverError(w, r, "export: read body", err)
		return
	}
	assets, err := ro.cfg.Store.ListArtifactAssets(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "export: list assets", err)
		return
	}

	doc, err := export.Materialize(body, assets,
		func(as store.ArtifactAsset) string {
			return ro.cfg.RenderOrigin + "/a/" + id + "/assets/" + as.ID
		},
		func(blobID string) ([]byte, error) {
			s, err := ro.readBlobString(r.Context(), blobID)
			return []byte(s), err
		})
	if err != nil {
		// Better a failed export than one that claims to be self-contained
		// while pointing at a URL that dies with the instance.
		serverError(w, r, "export: materialize", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+exportFilename(a.Title)+`"`)
	// The export reflects the artifact as it is right now; a cached copy would
	// quietly hand back a version the owner has already edited past.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, doc)
}

// readBlobString reads a blob fully into memory.
func (ro *Router) readBlobString(ctx context.Context, blobID string) (string, error) {
	rc, err := ro.cfg.Blob.Get(ctx, blobID)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// exportFilename turns an artifact title into a filename a browser will accept
// and a person will recognise. Titles are arbitrary user text — they arrive
// from a page's <title> on URL ingest — so everything outside a conservative
// set is collapsed rather than escaped: this value lands in a
// Content-Disposition header, where a stray quote or newline is a header
// injection rather than a cosmetic problem.
func exportFilename(title string) string {
	name := unsafeFilenameChars.ReplaceAllString(strings.TrimSpace(title), "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		name = "artifact"
	}
	if len(name) > 80 {
		name = strings.Trim(name[:80], "-.")
	}
	return name + ".html"
}
