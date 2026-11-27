package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/connectors"
)

type agentInteractionData struct {
	agent.Interaction
	SessionID string
	Proposal  *connectors.APIProposal
}

// Both routes use the owner-scoped session lookup. In particular, the
// extension's artifact credential cannot answer on the person's behalf.
func (ro *Router) agentInteraction(w http.ResponseWriter, r *http.Request) {
	s := ro.agentSession(w, r)
	if s == nil {
		return
	}
	interaction, ok := s.Interaction(chi.URLParam(r, "requestID"))
	if !ok {
		writeError(w, http.StatusNotFound, "interaction not found")
		return
	}
	data := agentInteractionData{Interaction: interaction, SessionID: s.ID}
	if interaction.Kind == agent.InteractionAPIConnector {
		if ro.connectionRequestStore(w) == nil {
			return
		}
		proposal, err := ro.cfg.Connections.GetAPIProposal(r.Context(), s.OwnerID, s.ArtifactID(), interaction.ProposalID)
		if err != nil {
			connectionError(w, err)
			return
		}
		data.Proposal = proposal
	}
	fragment, err := renderPage("agentInteraction", data)
	if err != nil {
		fragmentServerError(w, r, "agent interaction render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, fragment)
}

func (ro *Router) agentAnswerQuestions(w http.ResponseWriter, r *http.Request) {
	s := ro.agentSession(w, r)
	if s == nil {
		return
	}
	var req struct {
		agent.QuestionResponse
		Images []agentPromptImage `json:"images"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	images, err := promptImages(req.Images)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.QuestionResponse.Images = images
	if err := s.AnswerQuestions(chi.URLParam(r, "requestID"), req.QuestionResponse); err != nil {
		switch {
		case errors.Is(err, agent.ErrQuestionExpired):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, agent.ErrInvalidAnswer):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusBadGateway, "Could not send your answers. Please try again.")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Generic routes share authentication, transport and error behavior. Each kind
// still validates its own actions; a generic endpoint grants no new authority.
func (ro *Router) agentQuestions(w http.ResponseWriter, r *http.Request) {
	ro.agentInteraction(w, r)
}

func (ro *Router) agentInteractionAction(w http.ResponseWriter, r *http.Request) {
	s := ro.agentSession(w, r)
	if s == nil {
		return
	}
	var req struct {
		Revision      uint64 `json:"revision"`
		Action        string `json:"action"`
		Secret        string `json:"secret"`
		TrustedOrigin string `json:"trusted_origin"`
		agent.QuestionResponse
		Images []agentPromptImage `json:"images"`
	}
	if !decodeStrictJSON(w, r, &req) {
		return
	}
	interaction, ok := s.Interaction(chi.URLParam(r, "requestID"))
	if !ok {
		writeError(w, http.StatusNotFound, "interaction not found")
		return
	}
	if interaction.Kind == agent.InteractionAPIConnector {
		if ro.connectionRequestStore(w) == nil {
			return
		}
		if (req.Action != "approve" && req.Action != "deny") || req.Message != "" || len(req.Answers) != 0 || req.Cancelled || len(req.Images) != 0 || len(req.Snippets) != 0 ||
			(req.Action == "deny" && (req.Secret != "" || req.TrustedOrigin != "")) {
			writeError(w, http.StatusBadRequest, "choose Approve connection or Decline in the private card")
			return
		}
		allowance, err := ro.resolveAllowance(r.Context(), s.OwnerID)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "connection allowance could not be resolved")
			return
		}
		err = s.ResolveAPIConnector(interaction.ID, req.Revision, func(proposalID string) (string, error) {
			result, err := ro.cfg.Connections.ResolveAPIProposal(r.Context(), s.OwnerID, s.ArtifactID(), proposalID, req.Action == "approve", req.TrustedOrigin, req.Secret, allowance.Connections)
			if err != nil {
				return "", err
			}
			return result.State, nil
		})
		req.Secret = ""
		switch {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, agent.ErrInteractionExpired):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, agent.ErrInvalidInteractionAction):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			var mismatch *connectors.CredentialOriginError
			if errors.As(err, &mismatch) {
				writeError(w, http.StatusBadRequest, mismatch.Error())
				return
			}
			connectionError(w, err)
		}
		return
	}
	if req.Secret != "" || req.TrustedOrigin != "" {
		writeError(w, http.StatusBadRequest, "private connector fields require a connector approval")
		return
	}
	images, err := promptImages(req.Images)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.QuestionResponse.Images = images
	err = s.RespondInteraction(chi.URLParam(r, "requestID"), agent.InteractionAction{
		Revision: req.Revision, Action: req.Action, Questions: req.QuestionResponse,
	})
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, agent.ErrInteractionExpired):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, agent.ErrInvalidInteractionAction), errors.Is(err, agent.ErrInvalidAnswer):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusBadGateway, "Could not send your response. Please try again.")
	}
}
