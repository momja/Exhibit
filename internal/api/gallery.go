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
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/scanner"
	"github.com/momja/Exhibit/internal/store"
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
		ro.notFound(w, r)
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
		ro.notFound(w, r)
		return
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		serverError(w, r, "gallery edit blob", err)
		return
	}
	defer rc.Close()
	src, _ := io.ReadAll(rc)

	decisions, err := ro.cfg.Store.ListOriginDecisions(r.Context(), id)
	if err != nil {
		serverError(w, r, "gallery edit origin decisions", err)
		return
	}

	page, err := renderEditPage(a, decisions, string(src), ro.cfg.AuthToken)
	if err != nil {
		serverError(w, r, "gallery edit render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

// notFound serves the app's HTML 404 (av-at2v). It is both the mux's fallback
// for unrouted paths and what the gallery pages call for a missing artifact,
// so a 404 looks the same however it was arrived at.
//
// /api/* is deliberately excluded: chi propagates this handler into every
// subrouter, and those routes have JSON/text clients that must keep getting
// the plain error they always got, not a page.
func (ro *Router) notFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	page, err := renderNotFoundPage(r.URL.Path)
	if err != nil {
		serverError(w, r, "not found render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, page)
}

// notFoundPageData feeds the 404 page. RequestedPath is attacker-controlled
// URL text — html/template's contextual escaping is the whole defence, so it
// must reach the page as a template value and never as concatenated markup.
// LogoImage is the brand mark as an image source: the page inlines the SVG
// once in its header and draws the hero frame from the data URI, so one
// artwork serves both without duplicating its element ids (logo.go).
type notFoundPageData struct {
	Favicon       template.URL
	LogoSVG       template.HTML
	LogoImage     template.URL
	RequestedPath string
}

func renderNotFoundPage(requestedPath string) (string, error) {
	return renderPage("notfound", notFoundPageData{
		Favicon:       template.URL(exhibitLogoDataURI),
		LogoSVG:       template.HTML(exhibitLogoSVG),
		LogoImage:     template.URL(exhibitLogoDataURI),
		RequestedPath: requestedPath,
	})
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

// capabilityView is the data the capabilityCluster (badge, av-isb3) and
// capabilityPopover (av-41se) partials render. It's shared verbatim by the
// gallery card and the artifact detail/viewer page so the popover looks and
// behaves identically in both places. ShowManage gates the popover's footer
// "Manage in allowlist settings" link: true for both app-origin pages here.
// The render surface (internal/render) — which serves /s/:shareID — never
// composes gallery templates at all, so no caller there needs ShowManage;
// the field exists so a caller without an owner session can render the same
// partial without the link, and TestCapabilityPopoverManageLinkGatedByShowManage
// exercises exactly that.
type capabilityView struct {
	ArtifactID        string
	NetworkAllowlist  []string
	DownloadsApproved bool
	ClipboardApproved bool
	ShowManage        bool
}

// galleryCard is one artifact card on the index page. The tagRow/tagPills
// partials read ArtifactID and Tags from it directly; the capabilityCluster
// partial reads Capability to render the card-footer posture badge + popover
// (av-isb3, av-41se).
type galleryCard struct {
	ArtifactID string
	Title      string
	Created    string
	Tags       []tagView
	Capability capabilityView
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
			Capability: capabilityView{
				ArtifactID:        a.ID,
				NetworkAllowlist:  a.NetworkAllowlist,
				DownloadsApproved: a.DownloadsApproved,
				ClipboardApproved: a.ClipboardApproved,
				ShowManage:        true,
			},
		}
	}
	return renderPage("gallery", galleryPageData{
		Favicon:         template.URL(exhibitLogoDataURI),
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
	Capability   capabilityView
	Token        string
}

func renderDetailPage(a *store.Artifact, src, renderOrigin, token string) (string, error) {
	allowlist := a.NetworkAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}
	return renderPage("detail", detailPageData{
		ID:           a.ID,
		Title:        a.Title,
		Created:      a.CreatedAt.Format("Jan 2, 2006 15:04"),
		RenderOrigin: renderOrigin,
		SourceURL:    a.SourceURL,
		Src:          src,
		Capability: capabilityView{
			ArtifactID:        a.ID,
			NetworkAllowlist:  allowlist,
			DownloadsApproved: a.DownloadsApproved,
			ClipboardApproved: a.ClipboardApproved,
			ShowManage:        true,
		},
		Token: token,
	})
}

type editPageData struct {
	ID    string
	Title string
	Src   string
	Token string
	// An origin has three states here, not two (exhibit-x87): Allowlist holds
	// the decision='allow' origins (the ones the render CSP is built from);
	// Blocked holds the decision='block' origins — explicit "don't ask again"
	// answers from the runtime prompt, which never widen the CSP but must stay
	// visible and overridable rather than silently reading as undecided;
	// Unapproved holds the origins the current body references (per
	// scanner.Scan) that carry no decision at all, surfaced as one-click
	// "Allow" rows. Unapproved is never merged into Allowlist server-side;
	// that would auto-seed the allowlist from the scan, which spec §6.2
	// forbids.
	Allowlist         []string
	Blocked           []string
	Unapproved        []string
	DownloadsApproved bool
	ClipboardApproved bool
}

func renderEditPage(a *store.Artifact, decisions []store.OriginDecision, src, token string) (string, error) {
	allowlist, blocked := []string{}, []string{}
	for _, d := range decisions {
		switch d.Decision {
		case store.DecisionAllow:
			allowlist = append(allowlist, d.Origin)
		case store.DecisionBlock:
			blocked = append(blocked, d.Origin)
		}
	}
	// Only origins with no decision at all are "referenced, not approved" —
	// a blocked origin is a decision already made and belongs in Blocked.
	unapproved := diffOrigins(scanner.Scan(src), allowlist, blocked)
	return renderPage("edit", editPageData{
		ID:                a.ID,
		Title:             a.Title,
		Src:               src,
		Token:             token,
		Allowlist:         allowlist,
		Blocked:           blocked,
		Unapproved:        unapproved,
		DownloadsApproved: a.DownloadsApproved,
		ClipboardApproved: a.ClipboardApproved,
	})
}

// diffOrigins returns the origins in footprint that appear in none of the
// decided sets, preserving footprint's order. Used to surface "referenced, not
// approved" rows on the edit page without ever writing them to the allowlist.
func diffOrigins(footprint []string, decided ...[]string) []string {
	have := make(map[string]bool)
	for _, set := range decided {
		for _, o := range set {
			have[o] = true
		}
	}
	out := []string{}
	for _, o := range footprint {
		if !have[o] {
			out = append(out, o)
		}
	}
	return out
}
