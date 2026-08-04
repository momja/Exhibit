package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/store"
)

// --- BYO API key (Exh-ky6e) ---------------------------------------------
// The key crosses the wire exactly once, on PUT. It is sealed with the
// server secret before it touches the datastore, and reads return only a
// short hint — page JS never sees the key again after entry.

type putAgentKeyRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
}

func (ro *Router) putAgentKey(w http.ResponseWriter, r *http.Request) {
	var req putAgentKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Provider = strings.TrimSpace(req.Provider)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if !agent.KnownProvider(req.Provider) {
		writeError(w, http.StatusBadRequest, "unsupported provider")
		return
	}
	ownerID := ownerIDFromCtx(r.Context())

	// An empty api_key means "keep the currently stored key" — lets the UI
	// save a model/provider-label change without forcing re-entry of the
	// secret (Exh-454g). Only valid when a key for this provider already
	// exists; switching providers still needs a fresh key.
	if req.APIKey == "" {
		existing, err := ro.cfg.Store.GetAgentKey(r.Context(), ownerID)
		if err != nil {
			serverError(w, r, "get agent key", err)
			return
		}
		if existing == nil {
			writeError(w, http.StatusBadRequest, "api_key is required")
			return
		}
		if existing.Provider != req.Provider {
			writeError(w, http.StatusBadRequest, "api_key is required when changing provider")
			return
		}
		plain, err := ro.cfg.Secrets.Decrypt(existing.KeyCiphertext)
		if err != nil {
			writeError(w, http.StatusBadRequest, "api_key is required")
			return
		}
		row := &store.AgentKey{OwnerID: ownerID, Provider: req.Provider, Model: req.Model, KeyCiphertext: existing.KeyCiphertext}
		if err := ro.cfg.Store.SetAgentKey(r.Context(), row); err != nil {
			serverError(w, r, "store agent key", err)
			return
		}
		slog.InfoContext(r.Context(), "agent key model updated",
			slog.String("provider", req.Provider), slog.String("model", req.Model))
		writeJSON(w, http.StatusOK, agentKeyStatus(req.Provider, req.Model, plain))
		return
	}

	sealed, err := ro.cfg.Secrets.Encrypt(req.APIKey)
	if err != nil {
		serverError(w, r, "seal agent key", err)
		return
	}
	row := &store.AgentKey{OwnerID: ownerID, Provider: req.Provider, Model: req.Model, KeyCiphertext: sealed}
	if err := ro.cfg.Store.SetAgentKey(r.Context(), row); err != nil {
		serverError(w, r, "store agent key", err)
		return
	}
	slog.InfoContext(r.Context(), "agent key configured",
		slog.String("provider", req.Provider), slog.String("model", req.Model))
	writeJSON(w, http.StatusOK, agentKeyStatus(req.Provider, req.Model, req.APIKey))
}

func (ro *Router) getAgentKey(w http.ResponseWriter, r *http.Request) {
	k, err := ro.cfg.Store.GetAgentKey(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil {
		serverError(w, r, "get agent key", err)
		return
	}
	if k == nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	plain, err := ro.cfg.Secrets.Decrypt(k.KeyCiphertext)
	if err != nil {
		// Server secret changed since the key was stored: report unconfigured
		// so the user re-enters it, rather than 500ing the settings UI.
		slog.WarnContext(r.Context(), "agent key undecryptable, treating as unset", slog.String("err", err.Error()))
		writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	writeJSON(w, http.StatusOK, agentKeyStatus(k.Provider, k.Model, plain))
}

func (ro *Router) deleteAgentKey(w http.ResponseWriter, r *http.Request) {
	if err := ro.cfg.Store.DeleteAgentKey(r.Context(), ownerIDFromCtx(r.Context())); err != nil {
		serverError(w, r, "delete agent key", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// agentKeyStatus is the masked shape both GET and PUT return.
func agentKeyStatus(provider, model, plainKey string) map[string]any {
	return map[string]any{
		"configured": true,
		"provider":   provider,
		"model":      model,
		"key_hint":   maskKey(plainKey),
	}
}

func maskKey(k string) string {
	if len(k) <= 7 {
		return "•••"
	}
	return k[:3] + "…" + k[len(k)-4:]
}

// --- Sessions (Exh-m4ym / Exh-jlbt) --------------------------------------

type createAgentSessionRequest struct {
	ArtifactID string `json:"artifact_id"`
}

// agentSessionOpts builds the CreateOpts for a new session from the owner's
// stored provider key, writing the error response itself when the agent is
// unavailable or no usable key is configured. Both session creators — the chat
// surface and the widget generator (av-fafu) — go through it, so the key is
// decrypted in exactly one place.
func (ro *Router) agentSessionOpts(w http.ResponseWriter, r *http.Request) (agent.CreateOpts, bool) {
	if ro.cfg.Agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent support is not enabled (pi binary not found)")
		return agent.CreateOpts{}, false
	}
	ownerID := ownerIDFromCtx(r.Context())
	k, err := ro.cfg.Store.GetAgentKey(r.Context(), ownerID)
	if err != nil {
		serverError(w, r, "get agent key", err)
		return agent.CreateOpts{}, false
	}
	if k == nil {
		writeError(w, http.StatusPreconditionFailed, "no API key configured — add one in agent settings")
		return agent.CreateOpts{}, false
	}
	apiKey, err := ro.cfg.Secrets.Decrypt(k.KeyCiphertext)
	if err != nil {
		writeError(w, http.StatusPreconditionFailed, "stored API key is unreadable — re-enter it in agent settings")
		return agent.CreateOpts{}, false
	}
	return agent.CreateOpts{OwnerID: ownerID, Provider: k.Provider, Model: k.Model, APIKey: apiKey}, true
}

func (ro *Router) createAgentSession(w http.ResponseWriter, r *http.Request) {
	var req createAgentSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	opts, ok := ro.agentSessionOpts(w, r)
	if !ok {
		return
	}
	opts.ArtifactID = req.ArtifactID
	if req.ArtifactID != "" {
		a, err := ro.cfg.Store.GetArtifact(r.Context(), req.ArtifactID)
		if err != nil {
			serverError(w, r, "get artifact for agent session", err)
			return
		}
		if a == nil {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		opts.ArtifactTitle = a.Title
	}

	s, err := ro.cfg.Agent.Create(r.Context(), opts)
	if err != nil {
		serverError(w, r, "create agent session", err)
		return
	}
	// The ticket rides along with the id: creating a session and connecting
	// to its stream is one user action, so it should be one round trip.
	ticket, err := ro.sseTickets.Issue(s.ID, opts.OwnerID)
	if err != nil {
		serverError(w, r, "issue sse ticket", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          s.ID,
		"artifact_id": s.ArtifactID,
		"provider":    opts.Provider,
		"model":       opts.Model,
		"sse_ticket":  ticket,
	})
}

// agentSessionTicket mints a fresh SSE ticket for an existing session. A ticket
// is single-use, so every (re)connect needs one; this is the route a client
// calls when EventSource's automatic retry would otherwise replay a spent one.
func (ro *Router) agentSessionTicket(w http.ResponseWriter, r *http.Request) {
	s := ro.agentSession(w, r)
	if s == nil {
		return
	}
	ticket, err := ro.sseTickets.Issue(s.ID, s.OwnerID)
	if err != nil {
		serverError(w, r, "issue sse ticket", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"sse_ticket":     ticket,
		"expires_in_sec": int(sseTicketTTL / time.Second),
	})
}

type agentPromptRequest struct {
	Message string `json:"message"`
	Images  []struct {
		Data     string `json:"data"`
		MimeType string `json:"mime_type"`
	} `json:"images"`
}

func (ro *Router) agentPrompt(w http.ResponseWriter, r *http.Request) {
	s := ro.agentSession(w, r)
	if s == nil {
		return
	}
	var req agentPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	images := make([]agent.ImageContent, 0, len(req.Images))
	for _, im := range req.Images {
		if im.Data == "" {
			continue
		}
		mt := im.MimeType
		if mt == "" {
			mt = "image/png"
		}
		images = append(images, agent.ImageContent{Type: "image", Data: im.Data, MimeType: mt})
	}
	if err := s.Prompt(r.Context(), req.Message, images); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func (ro *Router) agentAbort(w http.ResponseWriter, r *http.Request) {
	s := ro.agentSession(w, r)
	if s == nil {
		return
	}
	if err := s.Abort(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) closeAgentSession(w http.ResponseWriter, r *http.Request) {
	if ro.cfg.Agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent support is not enabled")
		return
	}
	// Closing is idempotent — an already-closed or reaped session is a 204,
	// not a 404 — so this resolves the session itself rather than going
	// through agentSession. Another owner's session is left alone for the
	// same reason agentSession hides it: a session id is not a capability.
	sessionID := chi.URLParam(r, "sessionID")
	if s := ro.cfg.Agent.Get(sessionID); s != nil && !sessionOwnedBy(s, ownerIDFromCtx(r.Context())) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ro.cfg.Agent.Close(sessionID)
	// Tickets must not outlive the stream they name.
	ro.sseTickets.Forget(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

// agentSession resolves the {sessionID} route param to a live session the
// request's owner may use, writing the error response itself when it can't.
func (ro *Router) agentSession(w http.ResponseWriter, r *http.Request) *agent.Session {
	if ro.cfg.Agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent support is not enabled")
		return nil
	}
	s := ro.cfg.Agent.Get(chi.URLParam(r, "sessionID"))
	// Another owner's session is reported as absent rather than forbidden:
	// a session id alone must not be a capability, and "not found" leaks
	// nothing about which ids exist. Pinned to owner 1 today; this is the
	// multi-user seam the architecture claims is additive.
	if !sessionOwnedBy(s, ownerIDFromCtx(r.Context())) {
		writeError(w, http.StatusNotFound, "session not found (it may have been closed)")
		return nil
	}
	return s
}

// sessionOwnedBy reports whether a resolved session belongs to ownerID. A nil
// session (never created, or already reaped) belongs to nobody.
func sessionOwnedBy(s *agent.Session, ownerID int64) bool {
	return s != nil && s.OwnerID == ownerID
}

// agentEvents streams a session's Pi events to the browser as SSE. It sits
// outside the auth-header middleware because EventSource cannot set headers.
// Its credential is a single-use, seconds-lived, session-bound ticket (?ticket=,
// see ssetickets.go) rather than the service token, so nothing the chat page
// requests puts the master credential into a log or browser history (av-rgp1).
func (ro *Router) agentEvents(w http.ResponseWriter, r *http.Request) {
	// AuthToken == "" is the unauthenticated dev mode authMiddleware also
	// waves through; the owner is then the default owner.
	ownerID := defaultOwnerID
	if ro.cfg.AuthToken != "" {
		id, ok := ro.sseTickets.Redeem(chi.URLParam(r, "sessionID"), r.URL.Query().Get("ticket"))
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ownerID = id
	}
	r = r.WithContext(context.WithValue(r.Context(), ownerIDKey, ownerID))

	s := ro.agentSession(w, r)
	if s == nil {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, unsubscribe := s.Subscribe()
	defer unsubscribe()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case ev := <-events:
			fmt.Fprintf(w, "data: %s\n\n", ev)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-s.Done():
			// Drain anything already queued (incl. exhibit_session_closed).
			for {
				select {
				case ev := <-events:
					fmt.Fprintf(w, "data: %s\n\n", ev)
				default:
					flusher.Flush()
					return
				}
			}
		}
	}
}

// listTranscripts returns the agent conversations persisted with an artifact
// (colophon provenance, av-q3wo).
func (ro *Router) listTranscripts(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	ts, err := ro.cfg.Store.ListTranscripts(r.Context(), id)
	if err != nil {
		serverError(w, r, "list transcripts", err)
		return
	}
	out := make([]map[string]any, 0, len(ts))
	for sid, msgs := range ts {
		out = append(out, map[string]any{"session_id": sid, "messages": json.RawMessage(msgs)})
	}
	writeJSON(w, http.StatusOK, out)
}
