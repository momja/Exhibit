package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/momja/Exhibit/internal/inference"
	"github.com/momja/Exhibit/internal/store"
)

// Artifact inference (Exh-zue3): the HTTP half of window.exhibit.ai.
//
// The sandboxed frame posts a call to the host page, which makes this request
// with the page's own credential; the frame never holds an endpoint or a
// credential. This handler answers the questions only the app can: may this
// principal cause a call at all, may it on this artifact, and has the person
// who will pay agreed to. Everything after that — payer, limits, metering —
// is internal/inference's, which re-checks the principal itself.
//
// Who may call:
//   - the owner, once inference_approved is set (their approval is also their
//     consent to spend their own allowance);
//   - a named recipient, once the owner approved AND the recipient consented
//     to spending their own allowance;
//   - nobody else: public-mode visitors, agent and MCP credentials, and the
//     anonymous share and widget renders, which have no host and no session.

type inferenceRequest struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

// inferenceErrorBody is what the host page receives on a refusal and what it
// hands the frame: a stable code and fixed text, never upstream detail.
type inferenceErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Needs says which consent is missing for not_approved: "owner_approval"
	// (only the owner can approve; a recipient is told so) or "consent" (the
	// viewer may agree to pay).
	Needs        string `json:"needs,omitempty"`
	ResetAt      string `json:"reset_at,omitempty"`
	RetryAfterMS int64  `json:"retry_after_ms,omitempty"`
	Limit        string `json:"limit,omitempty"`
}

func writeInferenceError(w http.ResponseWriter, status int, body inferenceErrorBody) {
	if body.RetryAfterMS > 0 {
		w.Header().Set("Retry-After", itoa(int64(math.Ceil(float64(body.RetryAfterMS)/1000))))
	}
	writeJSON(w, status, map[string]any{"error": body})
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

var inferenceStatus = map[string]int{
	inference.CodeUnavailable:    http.StatusServiceUnavailable,
	inference.CodeNotApproved:    http.StatusForbidden,
	inference.CodeNoKey:          http.StatusForbidden,
	inference.CodeLimitReached:   http.StatusTooManyRequests,
	inference.CodeRateLimited:    http.StatusTooManyRequests,
	inference.CodeInvalidRequest: http.StatusBadRequest,
	inference.CodeProviderError:  http.StatusBadGateway,
	inference.CodeInvalidOutput:  http.StatusBadGateway,
}

// inferencePrincipalAllowed is the endpoint's own principal rule. Only a
// person — a session, or the no-login instance's single owner through its
// page token — may cause a call.
func inferencePrincipalAllowed(ctx context.Context) bool {
	p := principalFromCtx(ctx)
	if p.ReadOnly || p.Grant != nil {
		return false
	}
	switch p.Kind {
	case PrincipalSession, PrincipalServiceToken:
		return p.OwnerID > 0
	}
	return false
}

// fromAppPage reports whether a request that spends or authorizes AI
// spending came from Exhibit's own page. SameSite=Lax still sends the session
// cookie on a POST from a sibling subdomain — including the render origin in a
// deployment that puts both under one site — so a browser request must name
// the app origin, as the relay's does. Requests with neither header are not
// from a browser page and are left to authentication alone.
func (ro *Router) fromAppPage(r *http.Request) bool {
	if o := r.Header.Get("Origin"); o != "" && o != ro.cfg.AppOrigin {
		return false
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}

const inferenceChannelHeader = "X-Exhibit-Inference-Channel"

// inferenceViewerMatches binds a bridge request to the viewer who loaded the
// artifact. The session cookie can change while that document is still open.
// Authenticated API clients without browser headers can use the API directly.
func (ro *Router) inferenceViewerMatches(r *http.Request, artifactID string) bool {
	channel := r.Header.Get(inferenceChannelHeader)
	if channel == "" {
		return r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") == ""
	}
	return ro.artifactChannelMatches(r, artifactID, channel)
}

func (ro *Router) runInference(w http.ResponseWriter, r *http.Request) {
	unavailable := inferenceErrorBody{Code: inference.CodeUnavailable, Message: "AI is not available here."}
	if !inferencePrincipalAllowed(r.Context()) || !ro.cfg.Inference.Enabled() {
		writeInferenceError(w, http.StatusServiceUnavailable, unavailable)
		return
	}
	if !ro.fromAppPage(r) {
		writeInferenceError(w, http.StatusForbidden, inferenceErrorBody{Code: inference.CodeNotApproved,
			Message: "AI requests are accepted only from Exhibit itself."})
		return
	}
	artifactID := urlParamID(r, "artifactID")
	if !ro.inferenceViewerMatches(r, artifactID) {
		writeInferenceError(w, http.StatusForbidden, inferenceErrorBody{Code: inference.CodeUnavailable,
			Message: "Reload the artifact to use AI as the signed-in viewer."})
		return
	}
	var req inferenceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	kind, ok := inference.ParseKind(req.Kind)
	if !ok {
		writeInferenceError(w, http.StatusBadRequest, inferenceErrorBody{Code: inference.CodeInvalidRequest, Message: "Unknown AI method."})
		return
	}
	viewer := stateViewer(r)
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), viewer, artifactID)
	if err != nil {
		serverError(w, r, "inference artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if refusal := ro.inferenceGate(r.Context(), a, viewer); refusal != nil {
		writeInferenceError(w, http.StatusForbidden, *refusal)
		return
	}
	value, err := ro.cfg.Inference.Run(r.Context(), inference.Call{
		ArtifactID: a.ID, ViewerID: int64(viewer), Kind: kind, Payload: req.Payload,
	})
	if err != nil {
		var e *inference.Error
		if !errors.As(err, &e) {
			serverError(w, r, "inference", err)
			return
		}
		body := inferenceErrorBody{Code: e.Code, Message: e.Message, Limit: e.Limit}
		if !e.ResetAt.IsZero() {
			body.ResetAt = e.ResetAt.UTC().Format(time.RFC3339)
		}
		if e.RetryAfter > 0 {
			body.RetryAfterMS = e.RetryAfter.Milliseconds()
		}
		status, ok := inferenceStatus[e.Code]
		if !ok {
			status = http.StatusBadGateway
		}
		writeInferenceError(w, status, body)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]json.RawMessage{"value": value})
}

// inferenceGate applies the two consents. nil is allowed.
func (ro *Router) inferenceGate(ctx context.Context, a *store.Artifact, viewer store.ViewerID) *inferenceErrorBody {
	if !a.InferenceApproved {
		return &inferenceErrorBody{Code: inference.CodeNotApproved, Needs: "owner_approval",
			Message: "This tool has not been allowed to use AI."}
	}
	if int64(viewer) == a.OwnerID {
		return nil
	}
	ok, err := ro.cfg.Store.HasInferenceConsent(ctx, viewer, a.ID)
	if err != nil {
		slog.ErrorContext(ctx, "inference consent lookup failed: refusing", slog.String("err", err.Error()))
		return &inferenceErrorBody{Code: inference.CodeUnavailable, Message: "AI is not available right now."}
	}
	if !ok {
		return &inferenceErrorBody{Code: inference.CodeNotApproved, Needs: "consent",
			Message: "You have not allowed this tool to use your AI allowance."}
	}
	return nil
}

// setInferenceConsent records (PUT) or withdraws (DELETE) a named
// recipient's consent to spend their own allowance on an artifact. The owner
// has no consent row: their decision is inference_approved, set by PATCH.
func (ro *Router) setInferenceConsent(w http.ResponseWriter, r *http.Request) {
	if !inferencePrincipalAllowed(r.Context()) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !ro.fromAppPage(r) {
		writeError(w, http.StatusForbidden, "AI consent is given only in Exhibit itself")
		return
	}
	viewer := stateViewer(r)
	artifactID := urlParamID(r, "artifactID")
	if !ro.inferenceViewerMatches(r, artifactID) {
		writeError(w, http.StatusForbidden, "Reload the page to change AI consent as the signed-in viewer")
		return
	}
	allowed := r.Method == http.MethodPut
	if allowed {
		a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), viewer, artifactID)
		if err != nil {
			serverError(w, r, "inference consent lookup", err)
			return
		}
		if a == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if a.OwnerID == int64(viewer) {
			writeError(w, http.StatusBadRequest, "the owner approves AI use with inference_approved")
			return
		}
	}
	err := ro.cfg.Store.SetInferenceConsent(r.Context(), viewer, artifactID, allowed)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		serverError(w, r, "inference consent", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// inferencePageState is what an artifact page's host needs to answer
// available() and to choose a prompt. It never names a provider or model.
type inferencePageState struct {
	Enabled bool `json:"enabled"`
	// Complete and Decide are the kinds this viewer could use; Images is
	// whether complete() would accept image parts (Exh-v81w).
	Complete bool `json:"complete"`
	Decide   bool `json:"decide"`
	Images   bool `json:"images"`
	// Payer is "platform" or "own_key"; Reason explains no kinds.
	Payer  string `json:"payer,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Owner is whether the viewer owns the artifact; Approved and Consented
	// are the two decisions as they stand.
	Owner     bool `json:"owner"`
	Approved  bool `json:"approved"`
	Consented bool `json:"consented"`
}

// inferenceStateFor builds the page state for one viewer of one artifact.
// Disabled instances get the zero value, which the page reads as "say
// nothing": the artifact's calls reject unavailable and no chrome appears.
func (ro *Router) inferenceStateFor(ctx context.Context, a *store.Artifact, viewer int64) inferencePageState {
	if a == nil || !ro.cfg.Inference.Enabled() || viewer <= 0 {
		return inferencePageState{}
	}
	av := ro.cfg.Inference.Availability(ctx, viewer)
	st := inferencePageState{Enabled: true, Complete: av.Complete, Decide: av.Decide, Images: av.Images, Payer: av.Payer, Reason: av.Reason,
		Owner: a.OwnerID == viewer, Approved: a.InferenceApproved}
	if st.Owner {
		st.Consented = a.InferenceApproved
	} else if ok, err := ro.cfg.Store.HasInferenceConsent(ctx, store.ViewerID(viewer), a.ID); err == nil {
		st.Consented = ok
	}
	return st
}

// getInferenceState is the host page's question "what may this viewer do with
// AI on this artifact": the kinds, who pays, and the two consents. The page
// asks lazily, on the frame's first AI message, so pages that never meet an
// AI-using tool never ask.
func (ro *Router) getInferenceState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !inferencePrincipalAllowed(r.Context()) {
		writeJSON(w, http.StatusOK, inferencePageState{})
		return
	}
	viewer := stateViewer(r)
	artifactID := urlParamID(r, "artifactID")
	if !ro.inferenceViewerMatches(r, artifactID) {
		writeError(w, http.StatusForbidden, "Reload the artifact to use AI as the signed-in viewer")
		return
	}
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), viewer, artifactID)
	if err != nil {
		serverError(w, r, "inference state lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, ro.inferenceStateFor(r.Context(), a, int64(viewer)))
}
