package api

import "encoding/json"

// actionMenuView is the actionMenu partial's complete input. A feature supplies
// items and optional layout classes; the partial and Stimulus controller own
// the shared markup, keyboard interaction, and lifecycle.
type actionMenuView struct {
	ID           string
	Label        string
	Class        string
	TriggerClass string
	PanelClass   string
	Items        []actionMenuItem
}

type actionMenuItem struct {
	Label   string
	Icon    string
	Href    string
	Event   string
	Payload string
}

func menuLink(label, icon, href string) actionMenuItem {
	return actionMenuItem{Label: label, Icon: icon, Href: href}
}

// menuEvent asks the feature to perform an action. Payload is ordinary text
// escaped by html/template in the data attribute and decoded by Stimulus; it
// never contains executable markup. A string map always marshals successfully.
func menuEvent(label, icon, event string, detail map[string]string) actionMenuItem {
	payload, _ := json.Marshal(detail)
	return actionMenuItem{Label: label, Icon: icon, Event: event, Payload: string(payload)}
}

func galleryCardActions(id, title string, readOnly bool) actionMenuView {
	menu := actionMenuView{
		ID: "card-actions-" + id, Label: "Actions for " + title,
		Class: "card-actions", TriggerClass: "card-actions-trigger", PanelClass: "card-actions-menu",
		Items: []actionMenuItem{
			menuLink("Edit", "ph-pencil-simple", "/artifacts/"+id+"/edit"),
			menuEvent("Duplicate", "ph-copy", "exhibit:duplicate", map[string]string{"artifactID": id, "title": title}),
		},
	}
	if !readOnly {
		menu.Items = append(menu.Items, menuEvent("Download artifact", "ph-download-simple", "exhibit:download", map[string]string{"artifactID": id, "title": title}))
	}
	return menu
}
