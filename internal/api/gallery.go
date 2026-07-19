// The gallery pages (index, artifact detail, artifact edit) are html/template
// files under templates/ (epi-q0u2); this file holds their handlers and the
// view models the templates consume. The pages' stylesheets and scripts are
// static assets built from web/gallery/ and served under /assets/gallery/;
// per-request values reach the scripts through a small inline bootstrap
// <script> each template renders.
package api

import (
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/scanner"
	"github.com/momja/Exhibit/internal/store"
	"github.com/go-chi/chi/v5"
)

func (ro *Router) galleryIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	arts, err := ro.cfg.Store.ListArtifacts(r.Context(), store.ListOptions{Query: q, Limit: 100})
	if err != nil {
		serverError(w, r, "gallery index list artifacts", err)
		return
	}

	tags, _ := ro.cfg.Store.ListTags(r.Context(), 1)

	page, err := renderGalleryPage(arts, tags, q, ro.cfg.AuthToken)
	if err != nil {
		serverError(w, r, "gallery index render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

func (ro *Router) galleryDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		serverError(w, r, "gallery detail lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		serverError(w, r, "gallery detail blob", err)
		return
	}
	defer rc.Close()
	src, _ := io.ReadAll(rc)

	page, err := renderDetailPage(a, string(src), ro.cfg.RenderOrigin, ro.cfg.AuthToken)
	if err != nil {
		serverError(w, r, "gallery detail render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

func (ro *Router) galleryEdit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		serverError(w, r, "gallery edit lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		serverError(w, r, "gallery edit blob", err)
		return
	}
	defer rc.Close()
	src, _ := io.ReadAll(rc)

	page, err := renderEditPage(a, string(src), ro.cfg.AuthToken)
	if err != nil {
		serverError(w, r, "gallery edit render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

// tagView is a tag as the templates consume it: color already normalized to
// a well-formed #rrggbb (tag colors are user-authored free text; Normalize
// falls back to the default for anything malformed).
type tagView struct {
	ID    string
	Name  string
	Color string
}

func tagViews(tags []*store.Tag) []tagView {
	views := make([]tagView, len(tags))
	for i, t := range tags {
		views[i] = tagView{ID: t.ID, Name: t.Name, Color: color.Normalize(t.Color)}
	}
	return views
}

// galleryCard is one artifact card on the index page. The tagRow/tagPills
// partials read ArtifactID and Tags from it directly.
type galleryCard struct {
	ArtifactID string
	Title      string
	Created    string
	Tags       []tagView
}

// addTagModalData feeds the addTagModal partial: every existing tag for the
// dropdown, plus the preset palette for the create-new fields.
type addTagModalData struct {
	Tags    []tagView
	Presets []string
}

// The brand palette lives in web/gallery/tokens.css (av-xgik): pages link it
// instead of the old per-template inline :root injection. tokens.css mirrors
// internal/color/brand.go — keep the two in sync (color.BrandBlue still
// colors the server-rendered SVG logo).

type galleryPageData struct {
	// Favicon is a data: URI (base64 SVG); typed template.URL because
	// html/template rejects the data: scheme in URL contexts by default.
	Favicon template.URL
	// LogoSVG is the compiled-in brand mark (logo.go), trusted markup.
	LogoSVG         template.HTML
	Query           string
	Cards           []galleryCard
	Presets         []string
	AddTagModal     addTagModalData
	Token           string
	DefaultTagColor string
}

func renderGalleryPage(arts []*store.Artifact, tags []*store.Tag, query, token string) (string, error) {
	cards := make([]galleryCard, len(arts))
	for i, a := range arts {
		cards[i] = galleryCard{
			ArtifactID: a.ID,
			Title:      a.Title,
			Created:    a.CreatedAt.Format("Jan 2, 2006"),
			Tags:       tagViews(a.Tags),
		}
	}
	return renderPage("gallery", galleryPageData{
		Favicon:         template.URL(exhibitFaviconDataURI),
		LogoSVG:         template.HTML(exhibitLogoSVG),
		Query:           query,
		Cards:           cards,
		Presets:         color.Presets,
		AddTagModal:     addTagModalData{Tags: tagViews(tags), Presets: color.Presets},
		Token:           token,
		DefaultTagColor: store.DefaultTagColor,
	})
}

type detailPageData struct {
	ID           string
	Title        string
	Created      string
	RenderOrigin string
	SourceURL    string
	Src          string
	// Allowlist is rendered twice: as toolbar badges and, JSON-encoded by
	// the JS bootstrap, as the page script's mutable working copy. Never
	// nil — nil would encode as null and break allowlist.length.
	Allowlist         []string
	DownloadsApproved bool
	ClipboardApproved bool
	Token             string
}

func renderDetailPage(a *store.Artifact, src, renderOrigin, token string) (string, error) {
	allowlist := a.NetworkAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}
	return renderPage("detail", detailPageData{
		ID:                a.ID,
		Title:             a.Title,
		Created:           a.CreatedAt.Format("Jan 2, 2006 15:04"),
		RenderOrigin:      renderOrigin,
		SourceURL:         a.SourceURL,
		Src:               src,
		Allowlist:         allowlist,
		DownloadsApproved: a.DownloadsApproved,
		ClipboardApproved: a.ClipboardApproved,
		Token:             token,
	})
}

type editPageData struct {
	ID    string
	Title string
	Src   string
	Token string
	// Allowlist is the artifact's approved network origins. Unapproved holds
	// origins the current body references (per scanner.Scan) that are not yet
	// on the allowlist — surfaced as one-click "Allow" rows. Unapproved is
	// never merged into Allowlist server-side; that would auto-seed the
	// allowlist from the scan, which spec §6.2 forbids.
	Allowlist         []string
	Unapproved        []string
	DownloadsApproved bool
	ClipboardApproved bool
}

func renderEditPage(a *store.Artifact, src, token string) (string, error) {
	allowlist := a.NetworkAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}
	unapproved := diffOrigins(scanner.Scan(src), allowlist)
	return renderPage("edit", editPageData{
		ID:                a.ID,
		Title:             a.Title,
		Src:               src,
		Token:             token,
		Allowlist:         allowlist,
		Unapproved:        unapproved,
		DownloadsApproved: a.DownloadsApproved,
		ClipboardApproved: a.ClipboardApproved,
	})
}

// diffOrigins returns the origins in footprint not already present in
// approved, preserving footprint's order. Used to surface "referenced, not
// approved" rows on the edit page without ever writing them to the allowlist.
func diffOrigins(footprint, approved []string) []string {
	have := make(map[string]bool, len(approved))
	for _, o := range approved {
		have[o] = true
	}
	out := []string{}
	for _, o := range footprint {
		if !have[o] {
			out = append(out, o)
		}
	}
	return out
}
