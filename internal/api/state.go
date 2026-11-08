package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/momja/Exhibit/internal/store"
)

// stateViewer is the one principal every state route needs: whoever is looking
// (av-v991).
//
// It used to be two — an owner authorizing the reach and a viewer selecting the
// rows (av-q0ub) — and on this path that was always the same id twice, because
// only an owner could reach an artifact at all. A grant ended that, and the
// answer is not to resolve a second principal here but to stop having one: the
// store's ...AsViewer methods take the viewer alone and resolve whose rows that
// means from the artifact's share_state_mode. A caller writing on somebody
// else's behalf is then not something these handlers can express.
func stateViewer(r *http.Request) store.ViewerID {
	return store.ViewerID(ownerIDFromCtx(r.Context()))
}

func (ro *Router) getState(w http.ResponseWriter, r *http.Request) {
	artifactID := urlParamID(r, "artifactID")
	w.Header().Set("Cache-Control", "no-store")

	if !ro.artifactExists(w, r, artifactID, "get state") {
		return
	}
	if !ro.stateFramePolicy(w, r, artifactID) {
		return
	}

	state, err := ro.cfg.Store.GetStateAsViewer(r.Context(), stateViewer(r), artifactID)
	if err != nil {
		serverError(w, r, "get state", err)
		return
	}
	if !ro.stateFramePolicy(w, r, artifactID) {
		return
	}
	slog.DebugContext(r.Context(), "state read",
		slog.String("artifact_id", artifactID), slog.Int("keys", len(state)))
	writeJSON(w, http.StatusOK, state)
}

// App-owned frame resyncs carry their policy token. A source or grant change
// must stop new state flowing into a document whose CSP is now out of date.
// Ordinary API/editor state reads are not a frame bridge and need no token.
func (ro *Router) stateFramePolicy(w http.ResponseWriter, r *http.Request, artifactID string) bool {
	if len(r.Header.Values("X-Exhibit-Relay-Policy")) == 0 {
		return true
	}
	a, err := ro.cfg.Store.GetArtifactReadableBy(r.Context(), stateViewer(r), artifactID)
	if err != nil {
		serverError(w, r, "state frame policy", err)
		return false
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return false
	}
	return ro.requireRelayPolicy(w, r, a)
}

func (ro *Router) lockStateFramePolicy(w http.ResponseWriter, r *http.Request, artifactID string) (func(), bool) {
	if len(r.Header.Values("X-Exhibit-Relay-Policy")) == 0 {
		return func() {}, true
	}
	ro.connectionDataMu.Lock()
	if !ro.stateFramePolicy(w, r, artifactID) {
		ro.connectionDataMu.Unlock()
		return nil, false
	}
	return ro.connectionDataMu.Unlock, true
}

// Key is a pointer so an omitted key (nil — a malformed request) stays
// distinct from an empty one (""), which is a legitimate Web Storage key:
// localStorage.setItem("", v) is valid and must round-trip like any other.
// Testing the string for "" would conflate the two, which is the same mistake
// the delete route avoids by keying on query-parameter presence (av-hh1o).
type setStateRequest struct {
	Key   *string `json:"key"`
	Value string  `json:"value"`
}

func (ro *Router) setState(w http.ResponseWriter, r *http.Request) {
	artifactID := urlParamID(r, "artifactID")

	var req setStateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Key == nil {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	key := *req.Key
	version, ok := stateWriteVersion(w, r)
	if !ok {
		return
	}

	if !ro.artifactExists(w, r, artifactID, "set state") {
		return
	}
	unlock, allowed := ro.lockStateFramePolicy(w, r, artifactID)
	if !allowed {
		return
	}
	defer unlock()

	if err := ro.cfg.Store.SetStateAsViewer(r.Context(), stateViewer(r), artifactID, key, req.Value, version...); err != nil {
		writeArtifactError(w, r, "set state", err)
		return
	}

	slog.DebugContext(r.Context(), "state written",
		slog.String("artifact_id", artifactID),
		slog.String("key", key),
		slog.Int("value_bytes", len(req.Value)),
	)

	w.WriteHeader(http.StatusNoContent)
}

// deleteState removes one state row, or every row for the artifact.
//
// The key travels as the `key` QUERY parameter, deliberately not as a path
// segment (av-hh1o). State keys are arbitrary artifact-chosen text, and a key
// of ".." in a path is resolved by the browser's URL parser *before the
// request is sent* — turning DELETE /api/artifacts/:id/state/.. into
// DELETE /api/artifacts/:id/, which deletes the artifact. That made untrusted
// artifact code able to destroy the artifact through the host frame's token
// by calling localStorage.removeItem(".."). A query value has no segment
// structure, so there is nothing to normalize.
//
// Two more things fall out of the same choice: the empty-string key becomes
// representable (there is no empty path segment, but there is an empty query
// value), and the key stops being bounded by the request line, which a long
// key could otherwise overflow on delete despite being settable via the PUT
// body.
//
// Query().Has("key") is what separates the two operations — present-but-empty
// deletes the empty-string key, absent erases everything. Testing the value
// for "" would conflate them.
func (ro *Router) deleteState(w http.ResponseWriter, r *http.Request) {
	artifactID := urlParamID(r, "artifactID")

	if !ro.artifactExists(w, r, artifactID, "delete state") {
		return
	}
	unlock, allowed := ro.lockStateFramePolicy(w, r, artifactID)
	if !allowed {
		return
	}
	defer unlock()

	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, "invalid query encoding", http.StatusBadRequest)
		return
	}

	viewer := stateViewer(r)
	version, ok := stateWriteVersion(w, r)
	if !ok {
		return
	}

	// Absent key: erase everything *this viewer* holds on the artifact.
	// Destructive and irreversible — there is no version history for state —
	// but bounded twice over: it touches nothing else the artifact owns (body,
	// origin decisions, capability approvals all survive), and no other
	// viewer's rows.
	if !query.Has("key") {
		if err := ro.cfg.Store.ClearStateAsViewer(r.Context(), viewer, artifactID, version...); err != nil {
			writeArtifactError(w, r, "clear state", err)
			return
		}
		slog.InfoContext(r.Context(), "state cleared", slog.String("artifact_id", artifactID))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	key := query.Get("key")
	if err := ro.cfg.Store.DeleteStateAsViewer(r.Context(), viewer, artifactID, key, version...); err != nil {
		writeArtifactError(w, r, "delete state", err)
		return
	}

	slog.InfoContext(r.Context(), "state key deleted",
		slog.String("artifact_id", artifactID), slog.String("key", key))
	w.WriteHeader(http.StatusNoContent)
}

// artifactExists authorizes access and writes a 403/404/500 when it fails.
// Human viewers may reach shared artifacts, just as the store's AsViewer
// methods allow. Agent principals must own the artifact: this check is the
// additional security boundary preventing a library grant from spending
// incoming shares. Artifact ownership is immutable, so the subsequent AsViewer
// call cannot switch to a different owner between this check and the state
// operation.
//
// An agent principal must also have the owner's permission to use this
// artifact's saved data (Exh-dabd). Every state route an agent can reach comes
// through here, which makes this the boundary: the Pi extension leaving its
// state tools unregistered, and the prompt saying so, are only what keeps the
// model from asking.
func (ro *Router) artifactExists(w http.ResponseWriter, r *http.Request, artifactID, op string) bool {
	var a *store.Artifact
	var err error
	agentRequest := agentGrantFromCtx(r.Context()) != nil
	if agentRequest {
		// Sharing with a person does not delegate that artifact to their agent.
		a, err = ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), artifactID)
	} else {
		a, err = ro.cfg.Store.GetArtifactReadableBy(r.Context(), stateViewer(r), artifactID)
	}
	if err != nil {
		serverError(w, r, op+" artifact lookup", err)
		return false
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return false
	}
	if agentRequest {
		allowed, err := ro.agentStateAllowed(r.Context(), a)
		if err != nil {
			serverError(w, r, op+" agent state access", err)
			return false
		}
		if !allowed {
			writeError(w, http.StatusForbidden, agentStateRefusal)
			return false
		}
	}
	return true
}

// agentStateRefusal is what an agent is told when the owner has not let agents
// use an artifact's saved data. It is written for the model reading it as a
// tool error: what happened, that retrying will not help, and where the person
// can change it.
const agentStateRefusal = "the owner has not allowed agents to read or change this artifact's saved data. " +
	"Do not retry. If the request needs it, tell the person they can allow it on the artifact's edit page " +
	"(Stored state) or for every artifact in Settings."

// agentStateAllowed resolves whether agents may use a's saved data: the
// artifact's own override, else its owner's account default. a must be read
// owner-scoped, so a.OwnerID is whose default applies.
func (ro *Router) agentStateAllowed(ctx context.Context, a *store.Artifact) (bool, error) {
	accountDefault, err := ro.cfg.Store.AgentStateAccessDefault(ctx, a.OwnerID)
	if err != nil {
		return false, err
	}
	return a.AgentStateAllowed(accountDefault), nil
}

// Unversioned API edits are deliberate owner/agent actions. Frame writes carry
// the version embedded with their code and are conditional on it still being current.
func stateWriteVersion(w http.ResponseWriter, r *http.Request) ([]int, bool) {
	if r.Header.Get("X-Exhibit-Version") == "" {
		return nil, true
	}
	version, err := strconv.Atoi(r.Header.Get("X-Exhibit-Version"))
	if err != nil || version < 1 {
		writeError(w, http.StatusBadRequest, "invalid state version")
		return nil, false
	}
	return []int{version}, true
}
