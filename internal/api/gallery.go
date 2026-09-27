// The library index page: the artifact grid and its cards.
package api

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

func (ro *Router) galleryIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	// The gallery pages sit outside the API's auth group — their credential
	// lives in the page bootstrap — but they are still owner-scoped: the page
	// group runs ownerMiddleware under sessionGate, so this is the session's
	// owner on an instance with a login and the single-user default without
	// one (av-syug). It is never a guess; ownerIDFromCtx fails closed.
	ownerID := ownerIDFromCtx(r.Context())
	arts, err := ro.cfg.Store.ListArtifacts(r.Context(), store.ListOptions{OwnerID: ownerID, Query: q, Limit: 100})
	if err != nil {
		serverError(w, r, "gallery index list artifacts", err)
		return
	}

	page, err := renderGalleryPage(arts, q, ro.pageCredentials(r), ro.renderURLs(r), ro.adminRequest(r))
	if err != nil {
		serverError(w, r, "gallery index render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}

// shareBadgeView is the gallery card's sharing marker (av-6xjd, designed on
// av-v991's 2026-08-06 note).
//
// **One badge, naming the strongest thing true**, because the states are a
// ladder ordered by how much authority has left the owner's hands, not four
// independent flags:
//
//	private          no share row at all
//	shared with N    named accounts can run it, each on their own data
//	public link      anyone holding a URL can run it, and sees the owner's data
//	shared data      somebody named can CHANGE the owner's data
//
// **Private gets no marker.** The absence is the signal: a library of forty
// cards must not render forty badges, and marking the default trains people to
// ignore the marker. Level is "" for that case and the partial renders
// nothing.
//
// It is ambient — always visible on the card, never hover-only — because the
// failure it exists for is the share made eight months ago that nobody has
// thought about since. A marker you have to go looking for does not help
// somebody who has forgotten.
type shareBadgeView struct {
	// Level is "", "granted", "public" or "shared-data"; it drives the class
	// and is what a test asserts on.
	Level string
	Icon  string
	Label string
	// Detail is the full sentence, on the badge's title. Unlike Label it names
	// *every* fact that is true, since the label can only carry the strongest
	// one and "public link" would otherwise hide the three people who also
	// hold grants.
	Detail string
}

func newShareBadgeView(a *store.Artifact) shareBadgeView {
	grants, link := a.ShareGrantCount, a.SharePublicLink
	if grants == 0 && !link {
		return shareBadgeView{}
	}

	people := strconv.Itoa(grants) + " person"
	if grants != 1 {
		people = strconv.Itoa(grants) + " people"
	}
	detail := []string{}
	if grants > 0 {
		if a.ShareStateMode == store.ShareStateShared {
			detail = append(detail, people+" can open this artifact, and everyone using it reads and writes one shared copy of its data.")
		} else {
			detail = append(detail, people+" can open this artifact. Each keeps their own data.")
		}
	}
	if link {
		detail = append(detail, "Anyone with its public link can open it and read the owner's saved data.")
	}

	badge := shareBadgeView{Detail: strings.Join(detail, " ")}
	switch {
	// Write beats read: a named person changing the owner's saved data is
	// more authority given away than a stranger reading it, which is the
	// order av-v991's ladder puts them in. The public link, when there is
	// also one, is still stated in Detail.
	case grants > 0 && a.ShareStateMode == store.ShareStateShared:
		badge.Level, badge.Icon, badge.Label = "shared-data", "ph-users-three", "Shared data"
	case link:
		badge.Level, badge.Icon, badge.Label = "public", "ph-globe", "Public link"
	default:
		badge.Level, badge.Icon, badge.Label = "granted", "ph-user-circle", "Shared with "+strconv.Itoa(grants)
	}
	return badge
}

// galleryCard is one artifact card on the index page. The tagPills partial
// reads Tags from it directly; the capabilityCluster
// partial reads Capability to render the card-footer posture badge + popover
// (av-isb3, av-41se); Widget renders the card's tile (av-fafu); Share renders
// the sharing marker (av-6xjd), and renders nothing at all when the artifact
// is private.
type galleryCard struct {
	ArtifactID string
	Title      string
	Created    string
	Tags       []tagView
	Capability capabilityView
	Widget     widgetView
	Share      shareBadgeView
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
	LogoSVG template.HTML
	Query   string
	Cards   []galleryCard
	pageCredentials
	// IsAdmin reveals the header link to /admin/users (av-utap). It decides
	// what the page *offers* and nothing else: the route carries its own
	// adminOnly guard, so a visitor who reaches it by typing the URL is
	// refused there rather than admitted by a template that forgot to hide a
	// link. Hiding a control is a courtesy; the guard is the control.
	IsAdmin bool
}

func renderGalleryPage(arts []*store.Artifact, query string, creds pageCredentials, urls renderURLs, isAdmin bool) (string, error) {
	cards := make([]galleryCard, len(arts))
	for i, a := range arts {
		cards[i] = galleryCard{
			ArtifactID: a.ID,
			Title:      a.Title,
			Created:    a.CreatedAt.Format("Jan 2, 2006"),
			Tags:       tagViews(a.Tags),
			Capability: capabilityView{
				ArtifactID:         a.ID,
				NetworkAllowlist:   a.NetworkAllowlist,
				DownloadsApproved:  a.DownloadsApproved,
				ClipboardApproved:  a.ClipboardApproved,
				LinksApproved:      a.LinksApproved,
				CameraApproved:     a.CameraApproved,
				MicrophoneApproved: a.MicrophoneApproved,
				ShowManage:         true,
			},
			Widget: newWidgetView(a, urls),
			Share:  newShareBadgeView(a),
		}
	}
	return renderPage("gallery", galleryPageData{
		Favicon:         template.URL(exhibitLogoDataURI),
		LogoSVG:         template.HTML(exhibitLogoSVG),
		Query:           query,
		Cards:           cards,
		pageCredentials: creds,
		IsAdmin:         isAdmin,
	})
}
