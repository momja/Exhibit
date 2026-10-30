package api

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
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
}
type connectionSetupOption struct {
	ID   string
	Name string
}
type connectionRequestView struct {
	store.ArtifactConnectionRequest
	Action string
	URL    string
}
type connectionPageView struct {
	Advanced         bool
	CustomEnabled    bool
	BaseURL          string
	AdvancedURL      string
	WorkspaceURL     string
	SelectedProvider string
	SetupProviders   []connectionSetupOption
	Reconnect        *connectors.Connection
	Notice           string
	SetupFailed      bool
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
	Requests         []connectionRequestView
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
	v.Advanced = r.URL.Query().Get("advanced") == "1"
	v.SelectedProvider = r.URL.Query().Get("provider")
	q := url.Values{"artifact": {r.URL.Query().Get("artifact")}}
	v.BaseURL = "/connections?" + q.Encode()
	q.Set("advanced", "1")
	v.AdvancedURL = "/connections?" + q.Encode()
	v.WorkspaceURL = "/partials/connections?artifact=" + url.QueryEscape(r.URL.Query().Get("artifact"))
	if v.Advanced {
		v.WorkspaceURL += "&advanced=1"
	}
	if r.URL.Query().Get("connected") != "" {
		v.Notice = "Account connected. Existing tool permissions are unchanged."
		if r.URL.Query().Get("artifact") != "" {
			v.Notice += " Review access for this tool below."
		}
	}
	if r.URL.Query().Get("connection_error") != "" {
		v.SetupFailed = true
		v.Notice = "The account was not connected. Your existing access has not changed. You can try again."
	}
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
		v.CustomEnabled = ro.cfg.Connections.CustomEnabled()
		v.Providers = ro.cfg.Connections.Providers()
		for _, p := range v.Providers {
			if p.Enabled && p.Kind != "vendor" && (p.Kind != "native" || v.Advanced) {
				v.SetupProviders = append(v.SetupProviders, connectionSetupOption{p.ID, p.Name})
			}
		}
		v.VendorProviders = ro.cfg.Connections.VendorProviders()
		for _, p := range v.VendorProviders {
			v.SetupProviders = append(v.SetupProviders, connectionSetupOption{"vendor:" + p.ID, p.Name})
		}
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
			if c.ID == r.URL.Query().Get("reconnect") {
				copy := c
				v.Reconnect = &copy
			}
		}
	}
	if r.URL.Query().Get("reconnect") != "" && v.Reconnect == nil {
		return v, store.ErrNotFound
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
			requests, requestErr := s.ListArtifactConnectionRequests(r.Context(), viewer, id)
			if requestErr != nil {
				return v, requestErr
			}
			for _, request := range requests {
				v.Requests = append(v.Requests, v.requestView(request))
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
				v.Preview = &connectionGrantPreview{Connection: c.Connection}
			}
		}
	}
	return v, nil
}

func (v connectionPageView) requestView(request store.ArtifactConnectionRequest) connectionRequestView {
	result := connectionRequestView{ArtifactConnectionRequest: request, Action: "Review connection options", URL: v.BaseURL + "#connection-setup"}
	for _, c := range v.Connections {
		if c.Provider == request.Provider || (request.Provider == "vendor" && c.Kind == "vendor") {
			if c.Status == "active" {
				result.Action = "Allow this tool"
				result.URL = v.BaseURL + "&connection=" + url.QueryEscape(c.ID) + "#connection-access"
			} else {
				result.Action = "Reconnect account"
				base := v.BaseURL
				if c.Kind == "native" {
					base = v.AdvancedURL
				}
				result.URL = base + "&reconnect=" + url.QueryEscape(c.ID) + "#connection-setup"
			}
			return result
		}
	}
	for _, p := range v.Providers {
		if p.ID == request.Provider && p.Enabled {
			result.Action = "Connect account"
			base := v.BaseURL
			if p.Kind == "native" {
				base = v.AdvancedURL
				result.Action = "Set up custom connection"
			}
			result.URL = base + "&provider=" + url.QueryEscape(p.ID) + "#connection-setup"
			return result
		}
	}
	for _, p := range v.VendorProviders {
		if p.ID == request.Provider {
			result.Action = "Connect " + p.Name
			result.URL = v.BaseURL + "&provider=" + url.QueryEscape("vendor:"+p.ID) + "#connection-setup"
			return result
		}
	}
	return result
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
