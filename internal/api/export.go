package api

// Owner-only portable downloads: a complete ZIP by default, or an explicit
// legacy HTML document with out-of-line assets folded back in (av-vnkt).

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

// exportArtifact validates the format before loading any content. Both formats
// use the owner-scoped lookup, never the viewer/shared-state read path.
func (ro *Router) exportArtifact(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format != "" && format != "zip" && format != "html" {
		writeError(w, http.StatusBadRequest, "unsupported export format: choose zip or html")
		return
	}
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

	content, err := ro.loadArtifactBundleContent(r.Context(), ownerID, a, format != "html")
	if err != nil {
		serverError(w, r, "export: read content", err)
		return
	}
	if format != "html" {
		ro.exportArtifactZIP(w, r, a, content)
		return
	}
	doc, err := content.portableHTML(content.source)
	if err != nil {
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
