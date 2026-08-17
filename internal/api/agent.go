package api

import (
	"encoding/json"
	"fmt"
	"io"
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

// inlinedArtifactSource reads the artifact body a session opens with, so the
// agent does not spend its first tool call fetching what the handler is
// holding anyway (av-e0yj). The result is untrusted, exactly like the title
// beside it, and the session fences both as data rather than as instructions.
//
// A read failure is not fatal — the agent can still call get_artifact — so
// this returns "" and logs rather than failing the session. An oversized body
// is treated the same way: get_artifact is the fallback for a body too large
// to inline.
const maxInlinedArtifactSourceBytes = 10 << 20 // 10 MiB

func (ro *Router) inlinedArtifactSource(r *http.Request, a *store.Artifact) string {
	rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		slog.WarnContext(r.Context(), "agent session opened without inlined body",
			slog.String("artifact_id", a.ID), slog.String("err", err.Error()))
		return ""
	}
	defer rc.Close()
	body, err := io.ReadAll(io.LimitReader(rc, maxInlinedArtifactSourceBytes+1))
	if err != nil {
		slog.WarnContext(r.Context(), "agent session opened without inlined body",
			slog.String("artifact_id", a.ID), slog.String("err", err.Error()))
		return ""
	}
	if len(body) > maxInlinedArtifactSourceBytes {
		return ""
	}
	return string(body)
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
		a, err := ro.cfg.Store.GetArtifact(r.Context(), opts.OwnerID, req.ArtifactID)
		if err != nil {
			serverError(w, r, "get artifact for agent session", err)
			return
		}
		if a == nil {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		opts.ArtifactTitle = a.Title
		opts.ArtifactBody = ro.inlinedArtifactSource(r, a)
	}

	s, err := ro.cfg.Agent.Create(r.Context(), opts)
	if err != nil {
		serverError(w, r, "create agent session", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          s.ID,
		"artifact_id": s.ArtifactID(),
		"provider":    opts.Provider,
		"model":       opts.Model,
	})
}

// agentPromptRequest keeps the user's words and the untrusted material apart
// on the wire. Message is what the person typed and is the only part that
// reaches the model as an instruction; every Snippets entry is an element
// descriptor captured from inside the artifact (selector, text, outerHTML) and
// is fenced as data by the session. Page JS therefore never composes the
// envelope — the fence id it would need stays server-side (av-e0yj).
type agentPromptRequest struct {
	Message string `json:"message"`
	Images  []struct {
		Data     string `json:"data"`
		MimeType string `json:"mime_type"`
	} `json:"images"`
	Snippets []string `json:"snippets"`
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
	descriptors := make([]string, 0, len(req.Snippets))
	for _, descriptor := range req.Snippets {
		if strings.TrimSpace(descriptor) == "" {
			continue
		}
		descriptors = append(descriptors, descriptor)
	}
	data := make([]agent.DataBlock, 0, len(descriptors))
	for i, descriptor := range descriptors {
		data = append(data, agent.SnippetBlock(i, len(descriptors), descriptor))
	}
	if err := s.Prompt(r.Context(), req.Message, images, data); err != nil {
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

// closeAgentSession ends a session early — the chat page calls it when the
// visitor navigates away, rather than leaving the subprocess to the idle reaper.
//
// It resolves through agentSession like every other session route rather than
// calling Close on the raw route param, which is what it used to do: a DELETE
// is the one verb where an unscoped id costs a subprocess rather than a read.
// The alternative shape — answer 204 unconditionally and close only what the
// caller owns — is equally silent about whose session exists, but it would give
// this route a refusal rule of its own; one rule for all four is worth more than
// an idempotent DELETE to a caller (`resetSession` in agent.js) that discards
// the status either way.
func (ro *Router) closeAgentSession(w http.ResponseWriter, r *http.Request) {
	s := ro.agentSession(w, r)
	if s == nil {
		return
	}
	ro.cfg.Agent.Close(s.OwnerID, s.ID)
	w.WriteHeader(http.StatusNoContent)
}

// agentSession resolves the {sessionID} route param to a live session *of this
// request's owner*, writing the error response itself when it can't. Every
// route that reaches a session by id goes through here, which is the whole
// design: the registry is in memory, so nothing below this line filters by
// owner on the caller's behalf the way the Store's SQL does (av-ep8k).
//
// A session belonging to somebody else answers exactly as an id that was never
// issued — 404, never 403 — the same refusal the store contract (architecture
// §3.3) and adminOnly (admin.go) make, and for the same reason: a permission
// error would confirm the id is live, turning this route into a membership
// oracle over session ids.
//
// The owner comes from ownerIDFromCtx, so the credential asymmetry is already
// resolved upstream and identical to every other API route: a session cookie
// names its user, the static token and a login-free instance fall through
// ownerMiddleware to owner 1 — which is the owner a single-user instance's
// sessions are created under, so nothing changes there — and an unattributed
// request resolves to noOwner, which matches no session at all.
func (ro *Router) agentSession(w http.ResponseWriter, r *http.Request) *agent.Session {
	if ro.cfg.Agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent support is not enabled")
		return nil
	}
	s := ro.cfg.Agent.Get(ownerIDFromCtx(r.Context()), chi.URLParam(r, "sessionID"))
	if s == nil {
		writeError(w, http.StatusNotFound, "session not found (it may have been closed)")
		return nil
	}
	return s
}

// authorizeEventStream authenticates the SSE route and resolves the same
// Principal authMiddleware would, because this route cannot run it:
// EventSource sets no headers, so it is registered outside the group that runs
// authMiddleware and ownerMiddleware and has to resolve their answer itself.
// agentEvents stores the result on the context the same way ownerMiddleware
// would, so agentSession's owner check is one implementation for all four
// session routes rather than two that must be kept in step.
//
// Two credentials, and which one a browser holds depends on the instance
// (av-5imk):
//
//   - A **session cookie**, which the browser attaches to a same-origin
//     EventSource on its own. This is what a page on an instance with an
//     identity provider authenticates with — such a page is handed no bearer
//     token at all, precisely so logout can revoke its access. It names its
//     user, exactly as it does in authMiddleware.
//   - The **static token**, on a single-user instance whose page has no other
//     credential. It travels as `?token=` because there is nowhere else for it
//     to go; narrowing that is av-rgp1's subject, and this function is the one
//     place it would be narrowed. Matched via matchesServiceToken — the same
//     constant-time comparison authMiddleware now uses for its Authorization
//     header, closing what used to be the one place these two paths disagreed
//     (av-o5cf).
//
// With no token configured app auth is off entirely, matching authMiddleware —
// and such an instance still resolves defaultOwnerID rather than "anyone",
// because auth being off is not the same statement as ownership being off.
func (ro *Router) authorizeEventStream(r *http.Request) (Principal, bool) {
	if ownerID, ok := ro.sessionUser(r); ok {
		return Principal{OwnerID: ownerID, Kind: PrincipalSession}, true
	}
	if ro.cfg.AuthToken == "" {
		return Principal{OwnerID: defaultOwnerID, Kind: PrincipalNone}, true
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		token = bearerToken(r)
	}
	if !ro.matchesServiceToken(token) {
		return Principal{}, false
	}
	return Principal{OwnerID: defaultOwnerID, Kind: PrincipalServiceToken}, true
}

// agentEvents streams a session's Pi events to the browser as SSE. It sits
// outside the auth-header middleware because EventSource cannot set headers.
func (ro *Router) agentEvents(w http.ResponseWriter, r *http.Request) {
	p, ok := ro.authorizeEventStream(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Stand in for ownerMiddleware, which this route does not run. Everything
	// downstream — agentSession here, and any later handler that reads the
	// owner — then sees the same Principal an API-group request would carry.
	r = r.WithContext(withPrincipal(r.Context(), p))
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
	id := urlParamID(r, "artifactID")
	ts, err := ro.cfg.Store.ListTranscripts(r.Context(), ownerIDFromCtx(r.Context()), id)
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
