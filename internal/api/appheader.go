package api

import (
	"github.com/momja/Exhibit/internal/store"
	"html/template"
)

// appHeaderView composes a static app bar. Pages decide which actions to offer;
// route authorization remains independent of the navigation they render.
type appHeaderView struct {
	Floating  *floatingHeaderView
	Title     string
	TitleIcon string
	HideTitle bool
	Logo      template.HTML
	Back      *appHeaderBack
	Search    *galleryGridData
	Actions   []appHeaderAction
	Account   bool
	Clerk     *clerkView
}

type appHeaderBack struct {
	URL, Label    string
	CollapseLabel bool
}

// An action is a link when Href is set, otherwise a native button. SaveEditor
// connects the editor's existing save handler without adding inline script.
type appHeaderAction struct {
	Label, Icon, Href, ID, LabelID string
	IconOnly, Primary              bool
	SaveEditor                     bool
}

func galleryHeader(grid galleryGridData, isAdmin bool, clerk *clerkView) appHeaderView {
	title := grid.Title
	if title == "" {
		title = "Gallery"
	}
	header := appHeaderView{
		Title: title, HideTitle: true, Logo: template.HTML(exhibitLogoSVG),
		Search: &grid, Account: true, Clerk: clerk,
	}
	if isAdmin {
		header.Actions = append(header.Actions, appHeaderAction{
			Label: "Accounts", Icon: "ph-users-three", Href: "/admin/users",
		})
	}
	header.Actions = append(header.Actions,
		appHeaderAction{Label: "Add artifact", Icon: "ph-plus", Href: "/new", IconOnly: true},
	)
	return header
}

func subpageHeader(title string) appHeaderView {
	return appHeaderView{Title: title, Back: &appHeaderBack{URL: "/", Label: "Gallery", CollapseLabel: true}}
}

func editorHeader(id string, navigation editNavigation) appHeaderView {
	return appHeaderView{
		Back: &appHeaderBack{URL: navigation.URL, Label: navigation.Label},
		Actions: []appHeaderAction{
			{Label: "View artifact", Icon: "ph-eye", Href: "/artifacts/" + id},
			{Label: "Save changes", Icon: "ph-check", ID: "save-button", Primary: true, SaveEditor: true},
		},
	}
}

func profileHeader(clerk *clerkView) appHeaderView {
	header := subpageHeader("Settings")
	header.Account = clerk != nil
	header.Clerk = clerk
	return header
}

func agentHeader(backURL string, byok bool) appHeaderView {
	header := appHeaderView{
		Title: "Agent", TitleIcon: "ph-robot",
		Back: &appHeaderBack{URL: backURL, Label: "Back", CollapseLabel: true},
	}
	if byok {
		header.Actions = []appHeaderAction{{Label: "API key", Icon: "ph-key", ID: "key-btn", LabelID: "key-btn-label"}}
	}
	return header
}

func notFoundHeader() appHeaderView {
	return appHeaderView{
		Title: "Exhibit", Logo: template.HTML(exhibitLogoSVG),
		Actions: []appHeaderAction{{Label: "Add artifact", Icon: "ph-plus", Href: "/new", Primary: true}},
	}
}

func authPageHeader(title string) appHeaderView {
	return appHeaderView{Title: title, Logo: template.HTML(exhibitLogoSVG)}
}

// The viewer keeps its draggable controls as a header variant. Its facts and
// permissions are composed by the page, while the library owns the markup,
// styling and controller lifecycle for the entire floating header.
type floatingHeaderView struct {
	ID, Title, Created, SourceURL string
	ReadOnly, CanShare            bool
	// CanConnectAgents offers the External agents panel (Exh-l2xj), on the
	// same terms as CanShare: only where the page rendered it.
	CanConnectAgents bool
	Capability       capabilityView
}

func artifactHeader(a *store.Artifact, readOnly, canShare, canConnectAgents bool) appHeaderView {
	return appHeaderView{Floating: &floatingHeaderView{
		ID: a.ID, Title: a.Title, Created: a.CreatedAt.Format("Jan 2, 2006 15:04"), SourceURL: a.SourceURL,
		ReadOnly: readOnly, CanShare: canShare, CanConnectAgents: canConnectAgents,
		Capability: newCapabilityView(a, !readOnly),
	}}
}
