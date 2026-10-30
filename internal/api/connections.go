package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
)

func (ro *Router) connectionService(w http.ResponseWriter, r *http.Request) *connectors.Service {
	if ro.cfg.Connections == nil {
		writeError(w, 404, "experimental connectors are disabled")
		return nil
	}
	if !ro.relayHumanRequest(w, r) {
		return nil
	}
	w.Header().Set("Cache-Control", "no-store")
	return ro.cfg.Connections
}
func connectionError(w http.ResponseWriter, err error) {
	status := connectors.ErrorStatus(err)
	message := err.Error()
	if errors.Is(err, store.ErrNotFound) {
		message = "connection or artifact not found"
	}
	if code := managedErrorCode(err); code != "" {
		writeJSON(w, status, map[string]string{"error": vendorErrorMessage(code), "code": code})
		return
	}
	writeError(w, status, message)
}

func managedErrorCode(err error) string {
	var vendorError *connectors.VendorError
	if errors.As(err, &vendorError) {
		return connectors.ErrorCode(err)
	}
	return ""
}

func vendorErrorMessage(code string) string {
	switch code {
	case "needs_reconnect":
		return "This account needs to be connected again in Connections."
	case "rate_limited":
		return "The managed provider is rate limited. Try again later."
	case "refused":
		return "The managed provider refused this request. Review the connection permissions."
	default:
		return "The managed provider is unavailable. Try again later."
	}
}

func (ro *Router) listConnections(w http.ResponseWriter, r *http.Request) {
	s := ro.connectionService(w, r)
	if s == nil {
		return
	}
	owner := ownerIDFromCtx(r.Context())
	list, err := s.List(r.Context(), owner)
	if err != nil {
		connectionError(w, err)
		return
	}
	limits, err := ro.resolveAllowance(r.Context(), owner)
	if err != nil {
		writeError(w, 503, "connection allowance could not be resolved")
		return
	}
	writeJSON(w, 200, map[string]any{"connections": list, "providers": s.Providers(), "vendor_providers": s.VendorProviders(), "limits": limits.Connections})
}
func (ro *Router) createConnection(w http.ResponseWriter, r *http.Request) {
	s := ro.connectionService(w, r)
	if s == nil {
		return
	}
	var in struct {
		connectors.CreateRequest
		ArtifactID string `json:"artifact_id,omitempty"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	owner := ownerIDFromCtx(r.Context())
	limits, err := ro.resolveAllowance(r.Context(), owner)
	if err != nil {
		writeError(w, 503, "connection allowance could not be resolved")
		return
	}
	if !ro.validConnectionSetup(w, r, owner, in.ArtifactID) {
		return
	}
	c, err := s.Create(r.Context(), owner, in.CreateRequest, limits.Connections)
	if err != nil {
		connectionError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, c)
}

// reconnectConnection repairs the same saved identity; only replacement
// credential fields are accepted so granted destinations cannot change here.
func (ro *Router) reconnectConnection(w http.ResponseWriter, r *http.Request) {
	s := ro.connectionService(w, r)
	if s == nil {
		return
	}
	var in connectors.ReconnectRequest
	if !decodeStrictJSON(w, r, &in) {
		return
	}
	c, err := s.Reconnect(r.Context(), ownerIDFromCtx(r.Context()), urlParamID(r, "connectionID"), in)
	if err != nil {
		connectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (ro *Router) validConnectionSetup(w http.ResponseWriter, r *http.Request, owner int64, artifactID string) bool {
	if artifactID == "" {
		return true
	}
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), store.ViewerID(owner), artifactID)
	if err != nil {
		connectionError(w, err)
		return false
	}
	if a == nil {
		connectionError(w, store.ErrNotFound)
		return false
	}
	return true
}

func connectionSetupURL(setup connectors.SetupContext, connectionID, outcome string) string {
	q := url.Values{}
	if setup.ArtifactID != "" {
		q.Set("artifact", setup.ArtifactID)
	}
	if connectionID == "" {
		connectionID = setup.ConnectionID
	}
	if connectionID != "" {
		q.Set("connection", connectionID)
	}
	if outcome != "" {
		q.Set("connection_error", outcome)
	}
	if len(q) == 0 {
		return "/connections"
	}
	return "/connections?" + q.Encode()
}

func (ro *Router) deleteConnection(w http.ResponseWriter, r *http.Request) {
	s := ro.connectionService(w, r)
	if s == nil {
		return
	}
	if err := s.Revoke(r.Context(), ownerIDFromCtx(r.Context()), urlParamID(r, "connectionID")); err != nil {
		connectionError(w, err)
		return
	}
	w.WriteHeader(204)
}

func (ro *Router) connectionArtifact(w http.ResponseWriter, r *http.Request) (*connectors.Service, *store.Artifact) {
	s := ro.connectionService(w, r)
	if s == nil {
		return nil, nil
	}
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), store.ViewerID(ownerIDFromCtx(r.Context())), urlParamID(r, "artifactID"))
	if err != nil {
		serverError(w, r, "connection artifact", err)
		return nil, nil
	}
	if a == nil {
		writeError(w, 404, "artifact not found")
		return nil, nil
	}
	return s, a
}
func (ro *Router) listArtifactConnections(w http.ResponseWriter, r *http.Request) {
	s, a := ro.connectionArtifact(w, r)
	if a == nil {
		return
	}
	if ro.cfg.Relay.Enabled && !ro.requireRelayPolicy(w, r, a) {
		return
	}
	grants, err := s.Grants(r.Context(), ownerIDFromCtx(r.Context()), a.ID)
	if err != nil {
		connectionError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"grants": grants})
}
func (ro *Router) grantArtifactConnection(w http.ResponseWriter, r *http.Request) {
	s, a := ro.connectionArtifact(w, r)
	if a == nil {
		return
	}
	var in connectors.GrantRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	g, err := s.Grant(r.Context(), ownerIDFromCtx(r.Context()), a.ID, in)
	if err != nil {
		connectionError(w, err)
		return
	}
	writeJSON(w, 201, g)
}
func (ro *Router) deleteArtifactConnection(w http.ResponseWriter, r *http.Request) {
	s, a := ro.connectionArtifact(w, r)
	if a == nil {
		return
	}
	if err := s.Ungrant(r.Context(), ownerIDFromCtx(r.Context()), a.ID, urlParamID(r, "connectionID")); err != nil {
		connectionError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (ro *Router) connectionStandin(w http.ResponseWriter, r *http.Request) {
	s, a := ro.connectionArtifact(w, r)
	if a == nil {
		return
	}
	if ro.cfg.Relay.Enabled && !ro.requireRelayPolicy(w, r, a) {
		return
	}
	token, err := s.Standin(r.Context(), ownerIDFromCtx(r.Context()), a.ID, urlParamID(r, "connectionID"))
	if err != nil {
		connectionError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, token)
}

const connectionFlowCookie = "exhibit_connection_oauth"
const connectionVendorFlowCookie = "exhibit_connection_vendor"

func (ro *Router) connectionOAuthBinding(r *http.Request, flowCookie string) string {
	if ro.cfg.Clerk != nil {
		// Clerk rotates session-token bytes during the provider round trip.
		// Its verified subject plus the private flow cookie pins the account
		// and browser without depending on Exhibit's self-hosted cookie.
		subject := ro.cfg.Clerk.Subject(r)
		if subject == "" {
			return ""
		}
		return subject + ":" + flowCookie
	}
	if ro.loginEnabled() {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			return ""
		}
		return cookie.Value + ":" + flowCookie
	}
	return flowCookie
}
func (ro *Router) beginConnectionOAuth(w http.ResponseWriter, r *http.Request) {
	s := ro.connectionService(w, r)
	if s == nil {
		return
	}
	var in struct {
		Name string `json:"name"`
		connectors.SetupContext
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		writeError(w, 500, "could not begin OAuth connection")
		return
	}
	flow := base64.RawURLEncoding.EncodeToString(raw[:])
	binding := ro.connectionOAuthBinding(r, flow)
	destination, err := s.BeginOAuthFor(r.Context(), ownerIDFromCtx(r.Context()), binding, in.Name, strings.TrimSuffix(ro.cfg.AppOrigin, "/")+"/api/connections/oauth/github/callback", in.SetupContext)
	if err != nil {
		connectionError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: connectionFlowCookie, Value: flow, Path: "/api/connections/oauth/github", MaxAge: 600, HttpOnly: true, Secure: ro.cookieSecure(), SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{"url": destination})
}

// This callback is intentionally outside the API bearer gate: navigations in
// a service-token instance carry no Authorization header. The unpredictable
// one-shot state and HttpOnly browser binding provide the round-trip authority;
// a login-enabled instance additionally requires the original live session.
func (ro *Router) connectionOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if ro.cfg.Connections == nil {
		writeError(w, 404, "experimental connectors are disabled")
		return
	}
	owner := defaultOwnerID
	if ro.loginEnabled() {
		var ok bool
		owner, ok = ro.sessionUser(r)
		if !ok {
			writeError(w, 401, "sign in again before completing this connection")
			return
		}
	}
	cookie, err := r.Cookie(connectionFlowCookie)
	if err != nil || cookie.Value == "" {
		writeError(w, 400, "OAuth connection did not match this browser")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: connectionFlowCookie, Value: "", Path: "/api/connections/oauth/github", MaxAge: -1, HttpOnly: true, Secure: ro.cookieSecure(), SameSite: http.SameSiteLaxMode})
	limits, err := ro.resolveAllowance(r.Context(), owner)
	if err != nil {
		writeError(w, 503, "connection allowance could not be resolved")
		return
	}
	c, setup, err := ro.cfg.Connections.CompleteOAuthFor(r.Context(), owner, ro.connectionOAuthBinding(r, cookie.Value), r.URL.Query().Get("state"), r.URL.Query().Get("code"), limits.Connections)
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		if setup == nil {
			connectionError(w, err)
			return
		}
		outcome := "failed"
		if r.URL.Query().Get("error") == "access_denied" {
			outcome = "cancelled"
		}
		http.Redirect(w, r, connectionSetupURL(*setup, "", outcome), http.StatusSeeOther)
		return
	}
	destination := connectionSetupURL(*setup, c.ID, "")
	separator := "?"
	if strings.Contains(destination, "?") {
		separator = "&"
	}
	http.Redirect(w, r, destination+separator+"connected=github", http.StatusSeeOther)
}
func (ro *Router) beginConnectionVendor(w http.ResponseWriter, r *http.Request) {
	s := ro.connectionService(w, r)
	if s == nil {
		return
	}
	var in struct {
		Provider string `json:"provider"`
		connectors.SetupContext
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	limits, err := ro.resolveAllowance(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil {
		writeError(w, 503, "connection allowance could not be resolved")
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		writeError(w, 500, "could not begin managed connection")
		return
	}
	flow := base64.RawURLEncoding.EncodeToString(random[:])
	session, err := s.BeginVendorFor(r.Context(), ownerIDFromCtx(r.Context()), in.Provider, limits.Connections, ro.connectionOAuthBinding(r, flow), in.SetupContext)
	if err != nil {
		connectionError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: connectionVendorFlowCookie, Value: flow, Path: "/api/connections/vendor", MaxAge: 1800, HttpOnly: true, Secure: ro.cookieSecure(), SameSite: http.SameSiteStrictMode})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, session)
}

func (ro *Router) completeConnectionVendor(w http.ResponseWriter, r *http.Request) {
	s := ro.connectionService(w, r)
	if s == nil {
		return
	}
	var in struct {
		SessionID string `json:"session_id"`
		Name      string `json:"name"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	cookie, err := r.Cookie(connectionVendorFlowCookie)
	if err != nil || cookie.Value == "" {
		writeError(w, 400, "managed OAuth session did not match this browser")
		return
	}
	allowance, err := ro.resolveAllowance(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil {
		writeError(w, 503, "connection allowance could not be resolved")
		return
	}
	c, ready, setup, err := s.CompleteVendorFor(r.Context(), ownerIDFromCtx(r.Context()), ro.connectionOAuthBinding(r, cookie.Value), in.SessionID, in.Name, allowance.Connections)
	if err != nil {
		connectionError(w, err)
		return
	}
	if !ready {
		writeJSON(w, 202, map[string]string{"status": "pending"})
		return
	}
	writeJSON(w, 201, map[string]any{"status": "connected", "connection": c, "return_url": connectionSetupURL(*setup, c.ID, "")})
}
