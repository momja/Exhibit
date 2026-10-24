package api

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/bundle"
	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/export"
	"github.com/momja/Exhibit/internal/scanner"
	"github.com/momja/Exhibit/internal/store"
)

type connectionCard struct {
	connectors.Connection
	LastUsed string
	Usage    []store.ConnectionUsageArtifact
}
type connectionGrantPreview struct {
	Connection connectors.Connection
	ReadText   string
	Sharing    bool
}
type connectionPageView struct {
	Favicon          template.URL
	Enabled          bool
	Artifact         *store.Artifact
	IsOwner          bool
	Connections      []connectionCard
	Providers        []connectors.Provider
	VendorProviders  []connectors.VendorProviderInfo
	Grants           []connectors.Grant
	CanClearData     bool
	Relay            store.RelayGrant
	Activity         []store.RelayActivity
	Schedules        []store.RelaySchedule
	SchedulesEnabled bool
	Allowance        Allowance
	Requests         []store.ArtifactConnectionRequest
	Warnings         []scanner.Warning
	Preview          *connectionGrantPreview
	Operator         bool
	VendorUsage      []store.VendorUsage
	UsagePeriod      string
	pageCredentials
}

func (ro *Router) connectionOrigins(ctx context.Context, viewer int64, artifact string, base []string) ([]string, error) {
	policy := ro.cfg.Connections
	if policy == nil {
		policy = ro.cfg.ConnectionPolicy
	}
	if policy == nil {
		return base, nil
	}
	return policy.AllowedEgress(ctx, viewer, artifact, base)
}

func (ro *Router) connectionPageView(r *http.Request) (connectionPageView, error) {
	v := connectionPageView{Favicon: template.URL(exhibitLogoDataURI), Enabled: ro.cfg.Relay.Enabled, Connections: []connectionCard{}, Grants: []connectors.Grant{}, pageCredentials: ro.pageCredentials(r), SchedulesEnabled: ro.cfg.Relay.SchedulesEnabled}
	viewer := ownerIDFromCtx(r.Context())
	var err error
	v.Allowance, err = ro.resolveAllowance(r.Context(), viewer)
	if err != nil {
		return v, err
	}
	if ro.adminRequest(r) {
		v.Operator = true
		if s, ok := ro.cfg.Store.(vendorUsageStore); ok {
			now := time.Now().UTC()
			v.UsagePeriod = now.Format("January 2006") + " · UTC"
			v.VendorUsage, err = s.VendorUsage(r.Context(), now)
			if err != nil {
				return v, err
			}
		}
	}
	if v.Enabled && ro.cfg.Connections != nil {
		v.Providers = ro.cfg.Connections.Providers()
		v.VendorProviders = ro.cfg.Connections.VendorProviders()
		connections, err := ro.cfg.Connections.List(r.Context(), viewer)
		if err != nil {
			return v, err
		}
		for _, c := range connections {
			used := "Never used"
			if c.LastUsedAt != nil {
				used = c.LastUsedAt.Local().Format("2 Jan, 15:04")
			}
			usage, err := ro.cfg.Connections.Usage(r.Context(), viewer, c.ID)
			if err != nil {
				return v, err
			}
			v.Connections = append(v.Connections, connectionCard{Connection: c, LastUsed: used, Usage: usage})
		}
	}
	if id := r.URL.Query().Get("artifact"); id != "" {
		v.Artifact, err = ro.cfg.Store.GetArtifactReadableBy(r.Context(), store.ViewerID(viewer), id)
		if err != nil {
			return v, err
		}
		if v.Artifact == nil {
			return v, store.ErrNotFound
		}
		v.IsOwner = v.Artifact.OwnerID == viewer
		v.CanClearData = v.IsOwner || v.Artifact.ShareStateMode != store.ShareStateShared
		// Optional saved-data cleanup remains available with the engines off.
		if !v.Enabled {
			return v, nil
		}
		s, ok := ro.relayStore()
		if !ok {
			return v, fmt.Errorf("relay storage unavailable")
		}
		v.Relay, err = s.GetRelayGrant(r.Context(), viewer, id)
		if err != nil {
			return v, err
		}
		v.Activity, err = s.ListRelayActivity(r.Context(), viewer, id)
		if err != nil {
			return v, err
		}
		v.Schedules, err = s.ListRelaySchedules(r.Context(), viewer, id)
		if err != nil {
			return v, err
		}
		if ro.cfg.Connections != nil {
			v.Grants, err = ro.cfg.Connections.Grants(r.Context(), viewer, id)
			if err != nil {
				return v, err
			}
		}
		if s, ok := ro.cfg.Store.(store.ConnectionRequestStore); ok && v.IsOwner {
			v.Requests, err = s.ListArtifactConnectionRequests(r.Context(), viewer, id)
			if err != nil {
				return v, err
			}
		}
		if v.IsOwner {
			body, err := ro.readBlobString(r.Context(), v.Artifact.SourceBlobID)
			if err != nil {
				return v, err
			}
			v.Warnings = scanner.Warnings(body, v.Artifact.SourceURL)
		}
		selected := r.URL.Query().Get("connection")
		for _, c := range v.Connections {
			if c.ID == selected {
				v.Preview = &connectionGrantPreview{Connection: c.Connection, ReadText: strings.Join(c.Capabilities, ", "), Sharing: r.URL.Query().Get("sharing") == "on"}
			}
		}
	}
	return v, nil
}

func (ro *Router) connectionsPage(w http.ResponseWriter, r *http.Request) {
	v, err := ro.connectionPageView(r)
	if err != nil {
		writeArtifactError(w, r, "connections page", err)
		return
	}
	page, err := renderPage("connections", v)
	if err != nil {
		serverError(w, r, "connections render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, page)
}

func (ro *Router) connectionsPartial(w http.ResponseWriter, r *http.Request) {
	v, err := ro.connectionPageView(r)
	if err != nil {
		writeArtifactError(w, r, "connections panel", err)
		return
	}
	name := "connectionsBody"
	if r.URL.Query().Get("preview") == "true" {
		name = "connectionGrantPreview"
	}
	page, err := renderPage(name, v)
	if err != nil {
		serverError(w, r, "connections partial", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, page)
}

func (ro *Router) exportRelayBundle(w http.ResponseWriter, r *http.Request) {
	if !ro.cfg.Relay.Enabled {
		writeError(w, 404, "experimental relay is disabled")
		return
	}
	viewer := ownerIDFromCtx(r.Context())
	id := urlParamID(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), viewer, id)
	if err != nil {
		serverError(w, r, "bundle artifact", err)
		return
	}
	if a == nil {
		writeError(w, 404, "artifact not found")
		return
	}
	body, err := ro.readBlobString(r.Context(), a.SourceBlobID)
	if err != nil {
		serverError(w, r, "bundle source", err)
		return
	}
	assets, err := ro.cfg.Store.ListArtifactAssets(r.Context(), viewer, id)
	if err != nil {
		serverError(w, r, "bundle assets", err)
		return
	}
	page, err := export.Materialize(body, assets, func(as store.ArtifactAsset) string { return ro.cfg.RenderOrigin + "/a/" + id + "/assets/" + as.ID }, func(blobID string) ([]byte, error) {
		s, err := ro.readBlobString(r.Context(), blobID)
		return []byte(s), err
	})
	if err != nil {
		serverError(w, r, "bundle materialize", err)
		return
	}
	state, err := ro.cfg.Store.GetStateAsViewer(r.Context(), store.ViewerID(viewer), id)
	if err != nil {
		serverError(w, r, "bundle state", err)
		return
	}
	requirements := []bundle.ConnectionRequirement{}
	allowed, err := ro.connectionOrigins(r.Context(), viewer, id, a.NetworkAllowlist)
	if err != nil {
		serverError(w, r, "bundle destinations", err)
		return
	}
	allowedOrigins := make(map[string]bool, len(allowed))
	for _, origin := range allowed {
		allowedOrigins[origin] = true
	}
	policy := ro.cfg.Connections
	if policy == nil {
		policy = ro.cfg.ConnectionPolicy
	}
	if policy != nil {
		grants, err := policy.Grants(r.Context(), viewer, id)
		if err != nil {
			serverError(w, r, "bundle requirements", err)
			return
		}
		for _, g := range grants {
			kind := g.Connection.Provider
			if g.Connection.Kind == "oauth" || g.Connection.Kind == "vendor" {
				kind = "bearer"
			}
			for _, o := range g.Connection.Origins {
				if allowedOrigins[o] {
					requirements = append(requirements, bundle.ConnectionRequirement{
						Name: g.Connection.Name, Kind: kind, Origin: o, Header: g.Connection.Header,
						DailyLimit: g.Connection.DailyLimit, CacheMinutes: g.Connection.CacheMinutes,
					})
				}
			}
		}
	}
	app := bundle.New(a.Title, page, state, allowed, requirements)
	data, err := app.Bytes(strings.TrimSuffix(exportFilename(a.Title), ".html"))
	if err != nil {
		serverError(w, r, "bundle export", err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.TrimSuffix(exportFilename(a.Title), ".html")+`-app.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
