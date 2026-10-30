package api

import "html/template"

// appHeaderView composes a static app bar. Pages decide which actions to offer;
// route authorization remains independent of the navigation they render.
type appHeaderView struct {
	Title     string
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
	Label, Icon, Href, ID string
	IconOnly, Primary     bool
	SaveEditor            bool
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
