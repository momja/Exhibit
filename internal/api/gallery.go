// The gallery pages (index, artifact detail, artifact edit) are html/template
// files under templates/ (epi-q0u2); this file holds their handlers and the
// view models the templates consume. The pages' stylesheets and scripts are
// static assets built from web/gallery/ and served under /assets/gallery/;
// per-request values reach the scripts through a small inline bootstrap
// <script> each template renders.
package api

import (
	"fmt"
	"hash/fnv"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/scanner"
	"github.com/momja/Exhibit/internal/store"
)

// renderURLs mints the render-origin URLs one page render points its frames at.
// It carries the signing key and the owner the page is being rendered for, so
// every URL on the page is minted in memory during that render: a gallery of
// forty cards costs forty HMACs and zero round trips (av-c5aq AC#6).
//
// It is deliberately per-request rather than a Router field, because the owner
// is a property of the request, not of the process — which is what keeps this
// correct once sessions replace the fixed owner id.
type renderURLs struct {
	origin  string
	signer  *rendertoken.Signer
	ownerID int64
	// anonymous mints tokens that render the owner's artifact for nobody: no
	// state inlined, no write-through (av-wmp6). It is set when the request
	// being served is a public instance's unauthenticated visitor.
	//
	// The choice lives here, at the one place render URLs are minted, so that
	// "a public visitor's frames carry no state" follows from the request
	// rather than from every call site remembering to ask. A page that learns
	// to serve public visitors (av-eu3v, av-epnt) inherits the property by
	// marking the request; it cannot get the token flavour wrong separately.
	anonymous bool
}

func (ro *Router) renderURLs(r *http.Request) renderURLs {
	return renderURLs{
		origin:    ro.cfg.RenderOrigin,
		signer:    ro.tokens,
		ownerID:   ownerIDFromCtx(r.Context()),
		anonymous: publicVisitor(r.Context()),
	}
}

// mint signs one render-origin credential for id, as whoever this page is being
// rendered for.
func (u renderURLs) mint(id string) string {
	if u.anonymous {
		return u.signer.MintAnonymous(id, u.ownerID)
	}
	return u.signer.Mint(id, u.ownerID)
}

// artifact returns the tokened URL of an artifact's render document, for an
// iframe src. Links a visitor might click minutes later must NOT use this —
// they go through openArtifact, which mints at click time (a token embedded in
// a link goes stale while the page sits open).
func (u renderURLs) artifact(id string) string {
	return u.origin + "/a/" + id + "?" + rendertoken.Param + "=" + u.mint(id)
}

// widget returns the tokened URL of an artifact's widget document.
func (u renderURLs) widget(id string) string {
	return u.origin + "/w/" + id + "?" + rendertoken.Param + "=" + u.mint(id)
}

// cacheBust appends the per-render stamp that makes a browser actually refetch
// a frame after a save. It is a second parameter, not the first, because the
// token is already there — the render URLs above always carry a query string.
func cacheBust(url string) string {
	return url + "&r=" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func (ro *Router) galleryIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	// The gallery pages sit outside the API's auth group (their token lives
	// in the page bootstrap), so ownerIDFromCtx returns the default owner
	// here rather than a middleware-supplied one. That is the same owner the
	// API resolves today; when identity becomes real the pages inherit it
	// from the same seam instead of from a literal 1.
	ownerID := ownerIDFromCtx(r.Context())
	arts, err := ro.cfg.Store.ListArtifacts(r.Context(), store.ListOptions{OwnerID: ownerID, Query: q, Limit: 100})
	if err != nil {
		serverError(w, r, "gallery index list artifacts", err)
		return
	}

	tags, _ := ro.cfg.Store.ListTags(r.Context(), ownerID)

	page, err := renderGalleryPage(arts, tags, q, ro.cfg.AuthToken, ro.renderURLs(r))
	if err != nil {
		serverError(w, r, "gallery index render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

// galleryNew serves the add-artifact page (av-qo0j). It reads nothing from the
// store: ingest is entirely a client-side conversation with POST
// /api/artifacts, so the page needs only the API token its script posts with.
func (ro *Router) galleryNew(w http.ResponseWriter, r *http.Request) {
	page, err := renderNewPage(ro.cfg.AuthToken)
	if err != nil {
		serverError(w, r, "gallery new render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

func (ro *Router) galleryDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), id)
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

	page, err := renderDetailPage(a, string(src), ro.renderURLs(r), ro.cfg.AuthToken)
	if err != nil {
		serverError(w, r, "gallery detail render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

func (ro *Router) galleryEdit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
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

	canGenerate, generateHint := ro.widgetGenerateAvailability(r)
	page, err := renderEditPage(a, decisions, string(src), ro.widgetSource(r, a), ro.cfg.AuthToken, ro.renderURLs(r), canGenerate, generateHint)
	if err != nil {
		serverError(w, r, "gallery edit render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

// openURL is the app-origin path that opens an artifact top-level on the render
// origin. Pages link here instead of linking to RENDER_ORIGIN directly.
func openURL(artifactID string) string {
	return "/artifacts/" + artifactID + "/open"
}

// openArtifact is the "Open in new tab" door: it mints a fresh render token and
// redirects to the render origin (av-c5aq).
//
// A link is not a frame. A frame's src is fetched the moment the page renders,
// so a token baked into the markup is always fresh; a link sits in an open tab
// until someone clicks it, which may be an hour later — long past any TTL short
// enough to be worth having. Minting on the redirect makes the token's lifetime
// start at the click, so the TTL can stay minutes without the affordance
// breaking. It also keeps the token out of the page source, where a "copy link
// address" would spread a credential.
func (ro *Router) openArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	urls := ro.renderURLs(r)
	a, err := ro.cfg.Store.GetArtifact(r.Context(), urls.ownerID, id)
	if err != nil {
		serverError(w, r, "open artifact lookup", err)
		return
	}
	if a == nil || a.OwnerID != urls.ownerID {
		ro.notFound(w, r)
		return
	}
	// The Location header carries a credential and a deadline; a cached
	// redirect would hand out a token that has already expired.
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, urls.artifact(a.ID), http.StatusFound)
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

// widgetView is a card's tile (av-fafu). Exactly one of its two states
// renders: a live widget frame when the artifact has a widget document, or the
// server-rendered default tile when it doesn't.
//
// The default is deliberately not an iframe or an image. An artifact without a
// widget is the common case, and a gallery of forty cards must not pay forty
// frame loads (or a thumbnail pipeline) to say "nothing to show here" — a
// monogram on a tint derived from the artifact's own id costs one <div> and is
// stable for the life of the artifact, so a card keeps the same face every
// visit and stays recognizable at a glance.
type widgetView struct {
	// URL is the render-origin widget document, empty when there is no widget.
	URL string
	// Monogram and Hue drive the default tile. Hue is a plain 0–359 number the
	// stylesheet feeds to hsl(), so the tint stays a presentation decision.
	Monogram string
	Hue      int
	// Title is the owning artifact's title, used for the frame's accessible
	// name — an iframe needs one, and "<artifact> widget" is what it is.
	Title string
}

// galleryCard is one artifact card on the index page. The tagRow/tagPills
// partials read ArtifactID and Tags from it directly; the capabilityCluster
// partial reads Capability to render the card-footer posture badge + popover
// (av-isb3, av-41se); Widget renders the card's tile (av-fafu).
type galleryCard struct {
	ArtifactID string
	Title      string
	Created    string
	Tags       []tagView
	Capability capabilityView
	Widget     widgetView
}

// newWidgetView builds a card's tile view model, minting the tile frame's
// render token as it goes.
func newWidgetView(a *store.Artifact, urls renderURLs) widgetView {
	v := widgetView{
		Monogram: monogram(a.Title),
		Hue:      titleHue(a.ID),
		Title:    a.Title,
	}
	if a.WidgetBlobID != "" {
		v.URL = urls.widget(a.ID)
	}
	return v
}

// monogram reduces a title to the one or two letters the default tile shows.
// It walks runes rather than bytes so a non-ASCII title yields a real letter
// instead of half a UTF-8 sequence, and falls back to a dash for a title with
// no letters at all (an untitled artifact, an emoji-only name).
func monogram(title string) string {
	var letters []rune
	takeNext := true
	for _, r := range title {
		if unicode.IsSpace(r) || r == '-' || r == '_' {
			takeNext = true
			continue
		}
		if takeNext && unicode.IsLetter(r) {
			letters = append(letters, unicode.ToUpper(r))
			takeNext = false
			if len(letters) == 2 {
				break
			}
		}
	}
	if len(letters) == 0 {
		return "—"
	}
	return string(letters)
}

// titleHue derives a stable 0–359 hue from an artifact id, so every card gets a
// distinct-looking but unchanging tile. FNV-1a because the requirement is
// "spread ids across the wheel", not secrecy.
func titleHue(id string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return int(h.Sum32() % 360)
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

func renderGalleryPage(arts []*store.Artifact, tags []*store.Tag, query, token string, urls renderURLs) (string, error) {
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
			Widget: newWidgetView(a, urls),
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

// newPageData feeds the add-artifact page. It is deliberately thin: the page
// creates artifacts through the API like any other client, so the only
// per-request value it needs is the bearer token its script posts with.
type newPageData struct {
	Favicon template.URL
	Token   string
}

func renderNewPage(token string) (string, error) {
	return renderPage("new", newPageData{
		Favicon: template.URL(exhibitLogoDataURI),
		Token:   token,
	})
}

// detailPageData feeds the viewer page. It carries two distinct render-origin
// URLs rather than the bare origin, because the two have different lifetimes:
// FrameURL embeds a render token and is consumed immediately (the iframe loads
// with the page), while OpenURL is an app-origin redirect a visitor may click
// long after the page was rendered — so its token is minted at click time
// instead of going stale in the markup (av-c5aq).
type detailPageData struct {
	ID         string
	Title      string
	Created    string
	FrameURL   string
	OpenURL    string
	SourceURL  string
	Src        string
	Capability capabilityView
	Token      string
}

func renderDetailPage(a *store.Artifact, src string, urls renderURLs, token string) (string, error) {
	allowlist := a.NetworkAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}
	return renderPage("detail", detailPageData{
		ID:        a.ID,
		Title:     a.Title,
		Created:   a.CreatedAt.Format("Jan 2, 2006 15:04"),
		FrameURL:  urls.artifact(a.ID),
		OpenURL:   openURL(a.ID),
		SourceURL: a.SourceURL,
		Src:       src,
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
}

func renderEditPage(a *store.Artifact, decisions []store.OriginDecision, src, widgetSrc, token string, urls renderURLs, canGenerate bool, generateHint string) (string, error) {
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
		WidgetSrc:         widgetSrc,
		Widget:            newWidgetView(a, urls),
		CanGenerateWidget: canGenerate,
		GenerateHint:      generateHint,
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
