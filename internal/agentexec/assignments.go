package agentexec

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// The optional local service multiplexes the SAME single-assignment supervisor;
// it does not implement another agent manager or prompt adapter. Completed
// assignments retain snapshots until an authenticated release after core saves.
func (s *Service) serveAssignments(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/ready" && r.Method == http.MethodGet {
		writeJSON(w, Readiness{Version: ProtocolVersion, PiVersion: PiVersion, Unassigned: true, Capabilities: []string{"commands", "idle-disconnect", "snapshot", "selection", "deduplication", "leases", "idle-ack", "final-ack"}})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/executions/")
	id, operation, ok := strings.Cut(path, "/")
	if !ok || id == "" || path == r.URL.Path || strings.ContainsAny(id, "?#\r\n") {
		writeError(w, 404, "not_found", "unknown worker operation")
		return
	}
	s.mu.Lock()
	child, retired, closed := s.children[id], s.retired[id], s.closed
	s.mu.Unlock()
	if closed {
		writeError(w, 503, "shutting_down", "local worker service is shutting down")
		return
	}
	if retired {
		if operation == "release" && r.Method == http.MethodPost {
			// An authenticated retry may follow a lost successful response.
			writeJSON(w, map[string]bool{"released": true})
			return
		}
		writeError(w, 409, "execution_retired", "execution was released; reserve a new attempt")
		return
	}
	if child == nil && operation == "ready" && r.Method == http.MethodGet {
		writeJSON(w, Readiness{Version: ProtocolVersion, PiVersion: PiVersion, ExecutionID: id, Unassigned: true})
		return
	}
	if child == nil && operation == "start" && r.Method == http.MethodPost {
		var config StartConfig
		if !decodeJSON(w, r, &config, 2*MaxSessionBytes+(1<<20)) {
			return
		}
		if config.ExecutionID != id || config.SessionID == "" {
			writeError(w, 400, "invalid_assignment", "matching execution and session identities are required")
			return
		}
		s.mu.Lock()
		child = s.children[id]
		if child == nil {
			if s.closed || len(s.children) >= s.cfg.MaxAssignments || len(s.retired) >= MaxCommands {
				s.mu.Unlock()
				writeError(w, 429, "assignment_limit", "local worker assignment capacity reached; close and release executions")
				return
			}
			cfg := s.cfg
			cfg.LocalAssignments = false
			cfg.ExecutionID = id
			cfg.SessionID = config.SessionID
			var err error
			child, err = NewService(cfg)
			if err != nil {
				s.mu.Unlock()
				writeError(w, 400, "invalid_assignment", "invalid execution identity")
				return
			}
			s.children[id] = child
		}
		s.mu.Unlock()
		data, _ := json.Marshal(config)
		r.Body = io.NopCloser(bytes.NewReader(data))
	}
	if child == nil {
		writeError(w, 404, "assignment_not_found", "execution is not assigned to this service")
		return
	}
	if operation == "release" && r.Method == http.MethodPost {
		child.mu.Lock()
		done := child.closed
		child.mu.Unlock()
		if !done {
			writeError(w, 409, "execution_active", "close and acknowledge the final snapshot before release")
			return
		}
		if err := child.Reclaim(r.Context()); err != nil {
			writeError(w, 503, "reclaim_failed", "worker scratch reclamation failed")
			return
		}
		s.mu.Lock()
		delete(s.children, id)
		s.retired[id] = true
		s.mu.Unlock()
		writeJSON(w, map[string]bool{"released": true})
		return
	}
	child.ServeHTTP(w, r)
}
