// Shared pieces of the server-rendered pages (html/template files under
// templates/, epi-q0u2): render-URL minting and the view models more than one
// page renders. Each page's handler and view model lives in its own file
// (gallery.go, newpage.go, detail.go, edit.go, notfound.go). Page stylesheets
// and scripts are static assets built from web/gallery/ and served under
// /assets/gallery/; per-request values reach the scripts through a small
// inline bootstrap <script> each template renders.
package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/store"
	"github.com/momja/Exhibit/internal/tile"
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
	origin string
	// signer must never be nil: mint dereferences it unconditionally on every
	// call. NewRouter is the only production constructor and always populates
	// it via renderSigner, which itself never returns nil (an ephemeral key
	// stands in when no secrets Box is configured). A Router built any other
	// way — direct struct literal, a future constructor — must preserve that.
	signer *rendertoken.Signer
	// ownerID is the owner of the artifacts these URLs address — the principal
	// that authorizes the read at the render surface, which refuses a token
	// whose owner does not own the artifact. viewerID is whose state rows the
	// document inlines (av-6axy).
	//
	// They hold the same value for every URL a page mints about its own
	// library, and differ in exactly one place: a recipient opening an artifact
	// a grant names them on (av-awr4), where the owner still authorizes and the
	// recipient's own rows are what renders. ownedBy is the only way to make
	// them differ, so "these URLs point at somebody else's artifact" is a
	// statement a call site has to make out loud.
	ownerID  int64
	viewerID int64
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

// renderURLs relies on ro.tokens being non-nil (see the signer field's
// comment); NewRouter guarantees that for every Router this package
// constructs.
func (ro *Router) renderURLs(r *http.Request) renderURLs {
	viewer := ownerIDFromCtx(r.Context())
	return renderURLs{
		origin:    ro.cfg.RenderOrigin,
		signer:    ro.tokens,
		ownerID:   viewer,
		viewerID:  viewer,
		anonymous: publicVisitor(r.Context()),
	}
}

// ownedBy re-points these URLs at an artifact somebody else owns, keeping this
// request's visitor as the state principal (av-awr4).
//
// It exists because a grant separates two things the rest of the page has no
// reason to tell apart: the artifact is read under its *owner's* authority —
// the render surface verifies the token's owner against the artifact's row —
// while the state inlined into it belongs to whoever is looking. Calling it
// with the request's own owner is a no-op, which is why the detail page can
// call it unconditionally and stay correct for the owner.
func (u renderURLs) ownedBy(ownerID int64) renderURLs {
	u.ownerID = ownerID
	return u
}

// mint signs one render-origin credential for id, as whoever this page is being
// rendered for.
func (u renderURLs) mint(id string) string {
	if u.anonymous {
		return u.signer.MintAnonymous(id, u.ownerID)
	}
	// MintViewer with the two principals equal encodes no principal at all, so
	// this is byte-identical to Mint for every page that renders its own
	// library — there is no owner/recipient branch here to get wrong.
	return u.signer.MintViewer(id, u.ownerID, u.viewerID)
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
// "Manage security settings" link: true for both app-origin pages here.
// The render surface (internal/render) — which serves /s/:shareID — never
// composes gallery templates at all, so no caller there needs ShowManage;
// the field exists so a caller without an owner session can render the same
// partial without the link, and TestCapabilityPopoverManageLinkGatedByShowManage
// exercises exactly that.
type capabilityView struct {
	ArtifactID         string
	NetworkAllowlist   []string
	DownloadsApproved  bool
	ClipboardApproved  bool
	LinksApproved      bool
	CameraApproved     bool
	MicrophoneApproved bool
	ShowManage         bool
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

// newWidgetView builds a card's tile view model, minting the tile frame's
// render token as it goes.
func newWidgetView(a *store.Artifact, urls renderURLs) widgetView {
	v := widgetView{
		Monogram: tile.Monogram(a.Title),
		Hue:      tile.Hue(a.ID),
		Title:    a.Title,
	}
	if a.WidgetBlobID != "" {
		v.URL = urls.widget(a.ID)
	}
	return v
}
