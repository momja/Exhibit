package api

import (
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/momja/Exhibit/internal/store"
)

var activityGallery = galleryDefinition{
	ID: "activity-gallery", Title: "Activity", PagePath: "/activity", FragmentPath: "/partials/gallery-grid/activity",
	NoFilters: true, HideSearch: true, Activity: true,
}

type activityFeed struct{ Running, Items []activityItem }
type activityItem struct {
	Title, Description, Label, Icon, URL, When, WhenISO string
	Version                                             int
	Conversation                                        bool
}

func conversationURL(artifact, session, parameter string) string {
	return "/agent?" + url.Values{"artifact": {artifact}, parameter: {session}}.Encode()
}

func (ro *Router) loadActivity(r *http.Request, grid galleryGridData, offset int) (galleryGridData, error) {
	grid.Activity = &activityFeed{}
	grid.Parameters = url.Values{}
	owner := ownerIDFromCtx(r.Context())
	if offset == 0 && ro.cfg.Agent != nil {
		running := ro.cfg.Agent.RunningSessions(owner)
		sort.Slice(running, func(i, j int) bool {
			if running[i].UpdatedAt.Equal(running[j].UpdatedAt) {
				return running[i].ID < running[j].ID
			}
			return running[i].UpdatedAt.After(running[j].UpdatedAt)
		})
		for _, session := range running {
			title := "New artifact"
			version := 0
			if session.ArtifactID != "" {
				a, err := ro.cfg.Store.GetArtifact(r.Context(), owner, session.ArtifactID)
				if err != nil {
					return grid, err
				}
				if a == nil {
					continue
				}
				title, version = a.Title, a.VersionSeq
			}
			grid.Activity.Running = append(grid.Activity.Running, activityItem{Title: title, Description: session.Title, Label: "Agent running", Icon: "ph-robot", Version: version, URL: conversationURL(session.ArtifactID, session.ID, "live"), Conversation: true})
		}
	}
	records, err := ro.cfg.Store.ListArtifactActivity(r.Context(), owner, galleryPageSize+1, offset)
	if err != nil {
		return grid, err
	}
	if len(records) > galleryPageSize {
		records = records[:galleryPageSize]
		grid.NextURL = grid.url(activityGallery.FragmentPath, offset+galleryPageSize)
	}
	for _, record := range records {
		item := activityItem{Title: record.Title, Description: record.Message, Label: versionOriginLabels[record.Origin], Icon: "ph-pencil-simple", Version: record.Seq, URL: "/artifacts/" + record.ArtifactID + "/edit#versions-panel", When: record.CreatedAt.UTC().Format("Jan 2, 2006 · 15:04 UTC"), WhenISO: record.CreatedAt.UTC().Format(time.RFC3339)}
		if item.Description == "" {
			item.Description = "Artifact version saved."
		}
		if record.Origin == store.VersionAgent {
			item.Label, item.Icon = "Agent change", "ph-robot"
		}
		if record.Origin == store.VersionInitial {
			item.Icon = "ph-plus-circle"
		}
		if record.HasConversation {
			item.URL, item.Conversation = conversationURL(record.ArtifactID, record.SessionID, "conversation"), true
		} else if ro.cfg.Agent != nil && record.SessionID != "" {
			if session := ro.cfg.Agent.Get(owner, record.SessionID); session != nil && session.ArtifactID() == record.ArtifactID {
				item.URL, item.Conversation = conversationURL(record.ArtifactID, record.SessionID, "live"), true
			}
		}
		grid.Activity.Items = append(grid.Activity.Items, item)
	}
	return grid, nil
}
