package api

import (
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

func (ro *Router) createAgentSession(w http.ResponseWriter, r *http.Request) {
	if ro.cfg.Agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent support is not enabled (pi binary not found)")
		return
	}
	var req createAgentSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ownerID := ownerIDFromCtx(r.Context())

	k, err := ro.cfg.Store.GetAgentKey(r.Context(), ownerID)
	if err != nil {
		serverError(w, r, "get agent key", err)
		return
	}
	if k == nil {
		writeError(w, http.StatusPreconditionFailed, "no API key configured — add one in agent settings")
		return
	}
	apiKey, err := ro.cfg.Secrets.Decrypt(k.KeyCiphertext)
	if err != nil {
		writeError(w, http.StatusPreconditionFailed, "stored API key is unreadable — re-enter it in agent settings")
		return
	}

	opts := agent.CreateOpts{
		OwnerID:    ownerID,
		Provider:   k.Provider,
		Model:      k.Model,
		APIKey:     apiKey,
		ArtifactID: req.ArtifactID,
	}
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
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          s.ID,
		"artifact_id": s.ArtifactID,
		"provider":    k.Provider,
		"model":       k.Model,
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
	ro.cfg.Agent.Close(chi.URLParam(r, "sessionID"))
	w.WriteHeader(http.StatusNoContent)
}

// agentSession resolves the {sessionID} route param to a live session,
// writing the error response itself when it can't.
func (ro *Router) agentSession(w http.ResponseWriter, r *http.Request) *agent.Session {
	if ro.cfg.Agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent support is not enabled")
		return nil
	}
	s := ro.cfg.Agent.Get(chi.URLParam(r, "sessionID"))
	if s == nil {
		writeError(w, http.StatusNotFound, "session not found (it may have been closed)")
		return nil
	}
	return s
}

// agentEvents streams a session's Pi events to the browser as SSE. It sits
// outside the auth-header middleware because EventSource cannot set headers;
// it accepts the same bearer token via the ?token query parameter instead.
func (ro *Router) agentEvents(w http.ResponseWriter, r *http.Request) {
	if ro.cfg.AuthToken != "" {
		token := r.URL.Query().Get("token")
		if auth := r.Header.Get("Authorization"); token == "" && strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
		if token != ro.cfg.AuthToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
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
