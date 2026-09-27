// The artifact edit page and the fragments its panels re-render.
package api

import (
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/scanner"
	"github.com/momja/Exhibit/internal/store"
)

func (ro *Router) galleryEdit(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
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

	decisions, err := ro.cfg.Store.ListOriginDecisions(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "gallery edit origin decisions", err)
		return
	}

	library, err := ro.cfg.Store.ListTags(r.Context(), ownerID)
	if err != nil {
		serverError(w, r, "gallery edit list tags", err)
		return
	}

	canGenerate, generateHint := ro.widgetGenerateAvailability(r)
	page, err := renderEditPage(a, decisions, library, string(src), ro.widgetSource(r, a), ro.pageCredentials(r), ro.renderURLs(r), canGenerate, generateHint)
	if err != nil {
		serverError(w, r, "gallery edit render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

// widgetSource reads an artifact's widget body for the edit page's editor, or
// "" when it has none. An unreadable widget blob is treated as absent rather
// than as an error: the edit page's job is to let the user fix the artifact,
// and failing the whole page over its tile would take that away.
func (ro *Router) widgetSource(r *http.Request, a *store.Artifact) string {
	if a.WidgetBlobID == "" {
		return ""
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.WidgetBlobID)
	if err != nil {
		return ""
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		return ""
	}
	return string(body)
}

// cardWidgetPartial re-renders one artifact's tile as a standalone fragment
// (av-fafu). The edit page's widget panel swaps it in after a save so the
// preview updates without a page reload — which would drop the CodeMirror
// buffer beside it — and without page JS assembling markup the cardWidget
// template already owns. Same rule as the agent preview fragment (av-6m3e):
// one definition per component.
//
// The frame URL carries a cache-busting stamp because the browser only
// re-requests a frame whose src changed, no-store or not.
func (ro *Router) cardWidgetPartial(w http.ResponseWriter, r *http.Request) {
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), r.URL.Query().Get("artifact"))
	if err != nil {
		serverError(w, r, "card widget partial lookup", err)
		return
	}
	if a == nil {
		// Plain-text 404: htmx leaves the target untouched on an error
		// response, so the visitor keeps the tile they had.
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}
	view := newWidgetView(a, ro.renderURLs(r))
	if view.URL != "" {
		view.URL = cacheBust(view.URL)
	}
	fragment, err := renderPage("cardWidget", view)
	if err != nil {
		serverError(w, r, "card widget partial render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, fragment)
}

// tagPanelPartial re-renders the edit page's Tags panel body after a tag
// change, so the page's editors keep their unsaved buffers.
func (ro *Router) tagPanelPartial(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, r.URL.Query().Get("artifact"))
	if err != nil {
		serverError(w, r, "tag panel partial lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}
	library, err := ro.cfg.Store.ListTags(r.Context(), ownerID)
	if err != nil {
		serverError(w, r, "tag panel partial list tags", err)
		return
	}
	fragment, err := renderPage("tagPanelBody", newTagPanelView(a, library))
	if err != nil {
		serverError(w, r, "tag panel partial render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, fragment)
}

// tagPanelView feeds tagPanelBody. Available is the owner's tags not yet on
// the artifact.
type tagPanelView struct {
	ArtifactID   string
	Tags         []tagView
	Available    []tagView
	Presets      []string
	DefaultColor string
}

func newTagPanelView(a *store.Artifact, library []*store.Tag) tagPanelView {
	attached := make(map[string]bool, len(a.Tags))
	for _, t := range a.Tags {
		attached[t.ID] = true
	}
	available := []*store.Tag{}
	for _, t := range library {
		if !attached[t.ID] {
			available = append(available, t)
		}
	}
	return tagPanelView{
		ArtifactID:   a.ID,
		Tags:         tagViews(a.Tags),
		Available:    tagViews(available),
		Presets:      color.Presets,
		DefaultColor: color.Normalize(store.DefaultTagColor),
	}
}

type editPageData struct {
	// Favicon is a data: URI (base64 SVG); typed template.URL because
	// html/template rejects the data: scheme in URL contexts by default.
	Favicon template.URL
	ID      string
	Title   string
	Src     string
	pageCredentials
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
	Allowlist          []string
	Blocked            []string
	Unapproved         []string
	DownloadsApproved  bool
	ClipboardApproved  bool
	LinksApproved      bool
	CameraApproved     bool
	MicrophoneApproved bool
	// The gallery widget (av-fafu): its source for the editor, and the same
	// tile view the library renders for the live preview beside it. WidgetSrc
	// is "" when the artifact has no widget, which is also when Widget renders
	// the default tile — so the two always agree without a third flag.
	WidgetSrc string
	Widget    widgetView
	// Whether the "Generate widget" button can run an agent, and the reason it
	// can't. Disabled-with-a-reason rather than hidden: a missing affordance is
	// harder to diagnose than one that says what it needs.
	CanGenerateWidget bool
	GenerateHint      string
	// TagPanel feeds the Tags panel; Presets feeds the edit-tag modal.
	TagPanel tagPanelView
	Presets  []string
}

func renderEditPage(a *store.Artifact, decisions []store.OriginDecision, library []*store.Tag, src, widgetSrc string, creds pageCredentials, urls renderURLs, canGenerate bool, generateHint string) (string, error) {
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
		Favicon:            template.URL(exhibitLogoDataURI),
		ID:                 a.ID,
		Title:              a.Title,
		Src:                src,
		pageCredentials:    creds,
		Allowlist:          allowlist,
		Blocked:            blocked,
		Unapproved:         unapproved,
		WidgetSrc:          widgetSrc,
		Widget:             newWidgetView(a, urls),
		CanGenerateWidget:  canGenerate,
		GenerateHint:       generateHint,
		TagPanel:           newTagPanelView(a, library),
		Presets:            color.Presets,
		DownloadsApproved:  a.DownloadsApproved,
		ClipboardApproved:  a.ClipboardApproved,
		LinksApproved:      a.LinksApproved,
		CameraApproved:     a.CameraApproved,
		MicrophoneApproved: a.MicrophoneApproved,
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
