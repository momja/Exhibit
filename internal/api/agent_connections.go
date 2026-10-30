package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/momja/Exhibit/internal/store"
)

// These DTOs deliberately select metadata instead of serializing an engine's
// connection or persistence record. No future credential field can silently
// become part of the model's context when a connector implementation changes.
type agentConnectionMetadata struct {
	Name     string   `json:"name"`
	Provider string   `json:"provider"`
	Origins  []string `json:"origins"`
	Status   string   `json:"status"`
	Endpoint string   `json:"endpoint,omitempty"`
}

type agentConnectionProvider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func (ro *Router) connectionRequestStore(w http.ResponseWriter) store.ConnectionRequestStore {
	requests, ok := ro.cfg.Store.(store.ConnectionRequestStore)
	if ro.cfg.Connections == nil || !ro.cfg.Relay.Enabled || !ok {
		writeError(w, http.StatusNotFound, "experimental connections are not enabled")
		return nil
	}
	return requests
}

// agentConnections and agentRequestConnection are the only connector routes
// admitted by an agent grant. Their artifact id still passes the ordinary
// owner-scoped lookup; listing a connection never attaches it to an artifact.
func (ro *Router) agentConnections(w http.ResponseWriter, r *http.Request) {
	requests := ro.connectionRequestStore(w)
	if requests == nil {
		return
	}
	artifactID := urlParamID(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())
	artifact, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, artifactID)
	if err != nil {
		serverError(w, r, "load connection request artifact", err)
		return
	}
	if artifact == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	connections, err := ro.cfg.Connections.List(r.Context(), ownerID)
	if err != nil {
		serverError(w, r, "list agent connection metadata", err)
		return
	}
	metadata := make([]agentConnectionMetadata, 0, len(connections))
	for _, connection := range connections {
		metadata = append(metadata, agentConnectionMetadata{
			Name: connection.Name, Provider: connection.Provider, Origins: connection.Origins,
			Status:   connection.Status,
			Endpoint: connection.Endpoint,
		})
	}
	providers := make([]agentConnectionProvider, 0)
	for _, provider := range ro.connectionRequestProviders() {
		providers = append(providers, agentConnectionProvider{
			ID: provider.ID, Name: provider.Name, Enabled: provider.Enabled,
		})
	}
	pending, err := requests.ListArtifactConnectionRequests(r.Context(), ownerID, artifactID)
	if err != nil {
		serverError(w, r, "list connection requests", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"connections": metadata, "providers": providers, "pending_requests": pending})
}

func (ro *Router) agentRequestConnection(w http.ResponseWriter, r *http.Request) {
	requests := ro.connectionRequestStore(w)
	if requests == nil {
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Reason   string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Provider, req.Reason = strings.TrimSpace(req.Provider), strings.TrimSpace(req.Reason)
	if req.Reason == "" || utf8.RuneCountInString(req.Reason) > 1000 {
		writeError(w, http.StatusBadRequest, "a reason of at most 1000 characters is required; never include credentials")
		return
	}
	providerEnabled := false
	for _, provider := range ro.connectionRequestProviders() {
		if provider.ID == req.Provider && provider.Enabled {
			providerEnabled = true
			break
		}
	}
	if !providerEnabled {
		writeError(w, http.StatusBadRequest, "choose an enabled provider from list_connections")
		return
	}
	artifactID := urlParamID(r, "artifactID")
	request, err := requests.RequestArtifactConnection(r.Context(), ownerIDFromCtx(r.Context()), artifactID, req.Provider, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "artifact not found")
		case errors.Is(err, store.ErrConnectionRequestLimit):
			writeError(w, http.StatusTooManyRequests, err.Error())
		default:
			serverError(w, r, "queue connection request", err)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, map[string]any{
		"request": request, "status": "pending", "manage_url": ro.connectionRequestSetupURL(artifactID, req.Provider),
	})
}

// Dismissal belongs to the app UI, never the agent credential's allowlist.
func (ro *Router) dismissConnectionRequest(w http.ResponseWriter, r *http.Request) {
	requests := ro.connectionRequestStore(w)
	if requests == nil {
		return
	}
	if err := requests.DeleteArtifactConnectionRequest(r.Context(), ownerIDFromCtx(r.Context()), urlParamID(r, "artifactID"), urlParamID(r, "requestID")); err != nil {
		serverError(w, r, "dismiss connection request", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Agent requests name only configured services; custom mechanisms stay opt-in.
func (ro *Router) connectionRequestProviders() []agentConnectionProvider {
	providers := []agentConnectionProvider{}
	if ro.cfg.Connections == nil {
		return providers
	}
	for _, p := range ro.cfg.Connections.Providers() {
		if p.ID != "vendor" {
			providers = append(providers, agentConnectionProvider{p.ID, p.Name, p.Enabled})
		}
	}
	for _, p := range ro.cfg.Connections.VendorProviders() {
		providers = append(providers, agentConnectionProvider{p.ID, p.Name, true})
	}
	return providers
}

func (ro *Router) connectionRequestSetupURL(artifactID, provider string) string {
	q := url.Values{"artifact": {artifactID}, "provider": {provider}}
	for _, p := range ro.cfg.Connections.Providers() {
		if p.ID == provider && p.Kind == "native" {
			q.Set("advanced", "1")
		}
	}
	for _, p := range ro.cfg.Connections.VendorProviders() {
		if p.ID == provider {
			q.Set("provider", "vendor:"+provider)
		}
	}
	return "/connections?" + q.Encode() + "#connection-setup"
}
