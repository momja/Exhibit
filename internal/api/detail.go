// The artifact detail (viewer) page, its "open in new tab" door, and the
// owner's share panel.
package api

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/momja/Exhibit/internal/store"
)

// galleryDetail serves the viewer page — the one app-origin page a non-owner
// can reach (av-awr4).
//
// A grant is not a link: it puts no secret in the URL, so a recipient opens the
// artifact at the same /artifacts/:id the owner does, and there is no second
// address and no second template. What differs is what the page *offers*, and
// that is one narrowing (forArtifactOwnedBy) rather than a parallel render
// path — the same shape public mode already uses to suppress edit controls from
// request context.
//
// The read goes through GetArtifactReadableBy, the deny-by-default accessor
// av-lrae added beside the owner-scoped one. It is the only route that calls it
// today, and deliberately: widening the owner predicate itself would have
// handed a recipient the DELETE and the body rewrite along with the read. A
// viewer with no grant gets (nil, nil) — the 404 page, indistinguishable from
// an artifact that never existed.
func (ro *Router) galleryDetail(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	viewerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), store.ViewerID(viewerID), id)
	if err != nil {
		serverError(w, r, "gallery detail lookup", err)
		return
	}
	if a == nil {
		ro.notFound(w, r)
		return
	}

	// The artifact body is deliberately NOT read here (av-02xs): the detail
	// page must never embed the source — for a multi-MB artifact that made
	// this page itself multi-MB and Safari stalls on the response, so the
	// artifact "never loads". The edit page is where the body is viewed and
	// edited. For a recipient there is no such page at all: the source is one
	// of the things a grant does not carry.
	creds := ro.pageCredentials(r).forArtifactOwnedBy(viewerID, a.OwnerID)

	// The share panel is read only for the owner, and the read is skipped
	// entirely for anybody else — a recipient may use the artifact, and who
	// else was given it is not part of what a grant carries.
	var share *sharePanelView
	if !creds.ReadOnly {
		view, err := ro.sharePanel(r, a)
		if err != nil {
			serverError(w, r, "gallery detail shares", err)
			return
		}
		share = &view
	}

	page, err := renderDetailPage(a, ro.renderURLs(r).ownedBy(a.OwnerID), creds, share)
	if err != nil {
		serverError(w, r, "gallery detail render", err)
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
// It reads through the same grant-aware accessor the detail page does
// (av-awr4), because this is the door the detail page's own affordances lead
// to: the toolbar's "Open in new tab", and the capability banner's remedy for
// a frame the sandbox cannot run. A recipient whose page offers those and
// whose /open then 404s has a broken page, and the reason would be invisible.
// It grants nothing extra — a top-level render of an artifact they may already
// read, under that artifact's own unchanged CSP.
func (ro *Router) openArtifact(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	urls := ro.renderURLs(r)
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), store.ViewerID(urls.viewerID), id)
	if err != nil {
		serverError(w, r, "open artifact lookup", err)
		return
	}
	if a == nil {
		ro.notFound(w, r)
		return
	}
	// The token is minted as the artifact's owner because that is who the
	// render surface checks it against; the visitor stays the state principal.
	// For an owner opening their own artifact the two are the same value and
	// this is the token it always was.
	//
	// The old belt-to-the-braces `a.OwnerID != urls.ownerID` check is gone with
	// the owner-scoped read it braced. It cannot be restated here: a recipient
	// legitimately opens an artifact they do not own, so the comparison it made
	// is no longer a statement about authority. The accessor is what authorizes
	// now, and it says so in SQL.
	urls = urls.ownedBy(a.OwnerID)
	// The Location header carries a credential and a deadline; a cached
	// redirect would hand out a token that has already expired.
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, urls.artifact(a.ID), http.StatusFound)
}

// sharePanelPartial re-renders the owner's share panel (av-6xjd) after a
// grant, a revoke, or a change to the public link.
//
// It exists for the reason the widget preview fragment does: the detail page
// holds a live artifact frame, and a reload to show the new guest list would
// restart the tool the owner is looking at. Rendering the same named partial
// the full page render used is what keeps one definition of the list — the
// alternative is page JS rebuilding rows in a second language, over
// attacker-influenced names it would then have to escape by hand.
//
// GetArtifact, not the grant-aware accessor: this panel is the owner's, and a
// recipient asking for it gets the 404 a nonexistent artifact gets.
func (ro *Router) sharePanelPartial(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, r.URL.Query().Get("artifact"))
	if err != nil {
		serverError(w, r, "share panel partial lookup", err)
		return
	}
	if a == nil {
		// Plain-text 404: htmx leaves the target untouched on an error
		// response, so the owner keeps the panel they had.
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}
	view, err := ro.sharePanel(r, a)
	if err != nil {
		serverError(w, r, "share panel partial shares", err)
		return
	}
	fragment, err := renderPage("sharePanelBody", view)
	if err != nil {
		serverError(w, r, "share panel partial render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, fragment)
}

// sharePanel reads one artifact's sharing state for the owner's panel.
func (ro *Router) sharePanel(r *http.Request, a *store.Artifact) (sharePanelView, error) {
	shares, err := ro.cfg.Store.ListArtifactShares(r.Context(), a.OwnerID, a.ID)
	if err != nil {
		return sharePanelView{}, err
	}
	view := sharePanelView{
		ArtifactID:      a.ID,
		Shares:          newArtifactSharesView(shares, ro.shareURL),
		StateModeShared: a.ShareStateMode == store.ShareStateShared,
	}
	if view.Shares.Link != nil {
		view.WidgetEmbed = `<iframe src="` + ro.shareWidgetURL(view.Shares.Link.ID) +
			`" width="320" height="132" style="border:0"></iframe>`
	}
	return view, nil
}

// sharePanelView is the owner's share panel (av-6xjd) — the three independent
// controls that decide who can open an artifact, and the list of who currently
// can.
//
// StateModeShared is a bool rather than the raw mode string because the panel
// offers a choice between exactly two answers and a template comparing strings
// would be a third place the enum's spelling has to be right.
type sharePanelView struct {
	ArtifactID      string
	Shares          artifactSharesView
	StateModeShared bool
	// WidgetEmbed is the ready-to-paste iframe snippet for the artifact's
	// shared tile (av-ei5h). Empty when there is no public link, since the
	// URL inside it would not resolve. An artifact with no widget still gets
	// one: the link serves its default tile (av-cp7j), and a widget added
	// later replaces that tile under the same URL. Panel-only, like
	// everything here: the JSON shares route keeps its own shape.
	WidgetEmbed string
}

// detailPageData feeds the viewer page. It carries two distinct render-origin
// URLs rather than the bare origin, because the two have different lifetimes:
// FrameURL embeds a render token and is consumed immediately (the iframe loads
// with the page), while OpenURL is an app-origin redirect a visitor may click
// long after the page was rendered — so its token is minted at click time
// instead of going stale in the markup (av-c5aq).
type detailPageData struct {
	// Favicon is a data: URI (base64 SVG); typed template.URL because
	// html/template rejects the data: scheme in URL contexts by default.
	Favicon    template.URL
	ID         string
	Title      string
	Created    string
	FrameURL   string
	OpenURL    string
	SourceURL  string
	Capability capabilityView
	// SharedState is share_state_mode == 'shared': every viewer of this
	// artifact is on the owner's rows, so somebody else may be writing them
	// while this page is open. It drives the page's state resync (av-v991) and
	// nothing else — the frame's document already resolved whose rows to
	// inline, server-side, and this is only the page learning that it is worth
	// asking again.
	SharedState bool
	pageCredentials
	// Share is the owner's share panel (av-6xjd), and it is nil for anybody
	// else. Nil rather than an empty struct, because "this visitor has no
	// share panel" and "this artifact is shared with nobody" are different
	// facts and the template must not be able to confuse them: a recipient
	// must not learn the guest list, and an owner with no shares yet must
	// still get the controls.
	//
	// The gate is creds.ReadOnly, narrowed per artifact by
	// forArtifactOwnedBy — the same flag every other owner-only control on
	// this page hangs off, and deliberately not a second one, because two
	// flags are two things to keep in agreement and the disagreement is the
	// bug.
	Share *sharePanelView
}

// renderDetailPage builds the viewer page for whoever this request is.
//
// creds.ReadOnly is the single switch between the owner's page and a
// recipient's (av-awr4). Everything the template withholds — the edit link, the
// source, the agent, the export, refetch, the allowlist and capability
// controls — hangs off it, and so does the popover's Manage link and the page
// script's willingness to approve anything. The capability *cluster* itself
// stays: it reports what this tool may reach, which is a fact about the
// artifact running in the recipient's browser and exactly what the CSP-block
// explanation refers back to. What goes is the link to change it.
func renderDetailPage(a *store.Artifact, urls renderURLs, creds pageCredentials, share *sharePanelView) (string, error) {
	allowlist := a.NetworkAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}
	return renderPage("detail", detailPageData{
		Favicon:   template.URL(exhibitLogoDataURI),
		Share:     share,
		ID:        a.ID,
		Title:     a.Title,
		Created:   a.CreatedAt.Format("Jan 2, 2006 15:04"),
		FrameURL:  urls.artifact(a.ID),
		OpenURL:   openURL(a.ID),
		SourceURL: a.SourceURL,
		// Read through the same constant the store resolves the mode with, so
		// the page and the render agree about what 'shared' is spelled as.
		SharedState: a.ShareStateMode == store.ShareStateShared,
		Capability: capabilityView{
			ArtifactID:         a.ID,
			NetworkAllowlist:   allowlist,
			DownloadsApproved:  a.DownloadsApproved,
			ClipboardApproved:  a.ClipboardApproved,
			LinksApproved:      a.LinksApproved,
			CameraApproved:     a.CameraApproved,
			MicrophoneApproved: a.MicrophoneApproved,
			// The popover's Manage link points at the edit page, which a
			// recipient cannot reach: it is a link to a 404 rather than a
			// control they might have used.
			ShowManage: !creds.ReadOnly,
		},
		pageCredentials: creds,
	})
}
