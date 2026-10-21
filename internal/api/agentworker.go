package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/momja/Exhibit/internal/agentcontrol"
)

const AgentWorkerAuthorityPath = "/internal/agent-worker/v1"
const (
	AgentCheckpointRevisionHeader = agentcontrol.CheckpointRevisionHeader
	AgentCheckpointKindHeader     = agentcontrol.CheckpointKindHeader
	AgentConversationTitleHeader  = agentcontrol.ConversationTitleHeader
	AgentArtifactVersionHeader    = agentcontrol.ArtifactVersionHeader
)

// NewAgentWorkerHandler is an own-auth supervisor API, independent of Router's
// browser/service-token/auth-disabled paths. Mount the entire prefix outside
// the general API auth AND generic 32 MiB body limiter; this handler bounds raw
// checkpoint bodies at 64 MiB before decoding/allocating them. Never mount it on
// the render origin or include a supervisor token in browser bootstrap.
//
// GET snapshot returns raw JSONL with metadata headers. PUT accepts complete
// JSONL plus revision/kind/title headers; its JSON acknowledgement omits the
// file. POST usage accepts agentcontrol.Usage and no owner/funding fields.
// An unknown protocol version/path is 404; unsupported methods are 405.
func NewAgentWorkerHandler(authority *agentcontrol.Control) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+AgentWorkerAuthorityPath+"/snapshot", func(w http.ResponseWriter, r *http.Request) {
		s, ok := agentWorkerSupervisor(w, r, authority)
		if !ok {
			return
		}
		snapshot, err := authority.ReadSnapshot(r.Context(), s.Identity)
		if err != nil {
			agentWorkerError(w, err)
			return
		}
		agentSnapshotHeaders(w, snapshot)
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, snapshot.SessionFile)
	})
	mux.HandleFunc("PUT "+AgentWorkerAuthorityPath+"/snapshot", func(w http.ResponseWriter, r *http.Request) {
		s, ok := agentWorkerSupervisor(w, r, authority)
		if !ok {
			return
		}
		// Reject stale authority before reading an expensive body, then revalidate
		// inside WriteCheckpoint's fence/transaction before committing it.
		release, err := authority.Guard(r.Context(), s.Identity)
		if err != nil {
			agentWorkerError(w, err)
			return
		}
		release()
		p, err := agentcontrol.ReadCheckpointRequest(w, r)
		if err != nil {
			agentWorkerError(w, err)
			return
		}
		s, ok = agentWorkerSupervisor(w, r, authority)
		if !ok {
			return
		}
		snapshot, err := authority.WriteCheckpoint(r.Context(), s.Identity, p)
		if err != nil {
			agentWorkerError(w, err)
			return
		}
		agentSnapshotHeaders(w, snapshot)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snapshot)
	})
	mux.HandleFunc("POST "+AgentWorkerAuthorityPath+"/usage", func(w http.ResponseWriter, r *http.Request) {
		s, ok := agentWorkerSupervisor(w, r, authority)
		if !ok {
			return
		}
		u, err := agentcontrol.ReadUsageRequest(w, r)
		if err != nil {
			agentWorkerError(w, err)
			return
		}
		s, ok = agentWorkerSupervisor(w, r, authority)
		if !ok {
			return
		}
		inserted, err := authority.ReportUsage(r.Context(), s, u)
		if err != nil {
			agentWorkerError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Recorded bool `json:"recorded"`
		}{inserted})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Vary", "Authorization")
		mux.ServeHTTP(w, r)
	})
}
func agentSnapshotHeaders(w http.ResponseWriter, s agentcontrol.Snapshot) {
	w.Header().Set(AgentCheckpointRevisionHeader, strconv.FormatInt(s.Revision, 10))
	w.Header().Set(AgentCheckpointKindHeader, s.Kind)
	w.Header().Set(AgentConversationTitleHeader, s.Title)
	w.Header().Set(AgentArtifactVersionHeader, strconv.Itoa(s.VersionSeq))
	w.Header().Set(agentcontrol.PreviousExecutionStatusHeader, s.PreviousStatus)
}
func agentWorkerSupervisor(w http.ResponseWriter, r *http.Request, c *agentcontrol.Control) (agentcontrol.Supervisor, bool) {
	if c == nil {
		agentWorkerError(w, errors.New("authority unavailable"))
		return agentcontrol.Supervisor{}, false
	}
	// A cookie or operator credential buys no authority here, on any instance.
	auth := r.Header.Get("Authorization")
	if len(auth) < 7 || !strings.EqualFold(auth[:7], "Bearer ") {
		agentWorkerError(w, agentcontrol.ErrCredential)
		return agentcontrol.Supervisor{}, false
	}
	s, err := c.AuthenticateSupervisor(r.Context(), auth[7:])
	if err != nil {
		agentWorkerError(w, err)
		return s, false
	}
	return s, true
}
func agentWorkerError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	switch {
	case errors.Is(err, agentcontrol.ErrCredential):
		status = http.StatusUnauthorized
	case errors.Is(err, agentcontrol.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, agentcontrol.ErrStale), errors.Is(err, agentcontrol.ErrAssigned), errors.Is(err, agentcontrol.ErrConflict), errors.Is(err, agentcontrol.ErrNotResumable):
		status = http.StatusConflict
	case errors.Is(err, agentcontrol.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, agentcontrol.ErrTooLarge), errors.Is(err, agentcontrol.ErrReportTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, agentcontrol.ErrOwnerLimit), errors.Is(err, agentcontrol.ErrFleetLimit):
		status = http.StatusTooManyRequests
	}
	// Never echo repository errors, request payloads or credential material.
	message := "worker authority unavailable"
	if status != http.StatusServiceUnavailable {
		message = err.Error()
	}
	http.Error(w, message, status)
}
