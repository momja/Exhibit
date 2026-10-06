package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/store"
)

// Agent conversations kept with an artifact (av-y7td), read two ways: as JSON
// for a client, and as the two fragments the chat page swaps into its history
// pane. Both are built from the same store reads and the same projection
// (agent.Messages), so what the page shows and what the API returns cannot
// disagree about what was said.
//
// What leaves the server is the projection and never the stored file. The file
// is Pi's: it holds the system prompt, every tool result in full, and the model
// that answered, none of which a person looking back at a conversation needs —
// and on a platform instance the model is deliberately unreported.

type transcriptListResponse struct {
	// HeadSeq is the artifact's current version, which each conversation's
	// version_seq is read against.
	HeadSeq     int                `json:"head_seq"`
	Transcripts []store.Transcript `json:"transcripts"`
}

type transcriptResponse struct {
	store.Transcript
	HeadSeq  int                       `json:"head_seq"`
	Messages []agent.TranscriptMessage `json:"messages"`
}

// listTranscripts returns an artifact's conversations, most recent first, as
// summaries.
func (ro *Router) listTranscripts(w http.ResponseWriter, r *http.Request) {
	ownerID, id := ownerIDFromCtx(r.Context()), urlParamID(r, "artifactID")
	ts, head, err := ro.transcriptList(r, ownerID, id)
	if err != nil {
		serverError(w, r, "list transcripts", err)
		return
	}
	writeJSON(w, http.StatusOK, transcriptListResponse{HeadSeq: head, Transcripts: ts})
}

// getTranscript returns one conversation as what was said in it. Another
// owner's, and one that does not exist, answer alike.
func (ro *Router) getTranscript(w http.ResponseWriter, r *http.Request) {
	ownerID, id := ownerIDFromCtx(r.Context()), urlParamID(r, "artifactID")
	t, head, err := ro.transcript(r, ownerID, id, urlParamID(r, "sessionID"))
	if err != nil {
		serverError(w, r, "get transcript", err)
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	writeJSON(w, http.StatusOK, transcriptResponse{Transcript: *t, HeadSeq: head, Messages: agent.Messages(t)})
}

// transcriptList and transcript are the two reads behind both the JSON routes
// and the fragments. Each pairs conversations with the artifact's head version,
// because a conversation's version means nothing without it.
func (ro *Router) transcriptList(r *http.Request, ownerID int64, artifactID string) ([]store.Transcript, int, error) {
	ts, err := ro.cfg.Store.ListTranscripts(r.Context(), ownerID, artifactID)
	if err != nil {
		return nil, 0, err
	}
	head, err := ro.cfg.Store.HeadVersionSeq(r.Context(), ownerID, artifactID)
	return ts, head, err
}

func (ro *Router) transcript(r *http.Request, ownerID int64, artifactID, sessionID string) (*store.Transcript, int, error) {
	t, err := ro.cfg.Store.GetTranscript(r.Context(), ownerID, artifactID, sessionID)
	if err != nil || t == nil {
		return nil, 0, err
	}
	head, err := ro.cfg.Store.HeadVersionSeq(r.Context(), ownerID, artifactID)
	return t, head, err
}

// --- The chat page's history pane ---------------------------------------------

// conversationView is one conversation as the history pane words it.
type conversationView struct {
	ArtifactID string
	SessionID  string
	Title      string
	// VersionSeq is the artifact version the conversation was last working
	// against; 0 for one kept before that was recorded.
	VersionSeq int
	// Current says the artifact has not changed since: the conversation's
	// version is the head.
	Current   bool
	Resumable bool
	// When is the last activity in a form a person reads; WhenISO is the same
	// instant for the <time> element.
	When    string
	WhenISO string
}

// untitledConversation stands in for a conversation with no first prompt on
// record, which only a malformed row has.
const untitledConversation = "Untitled conversation"

func newConversationView(t store.Transcript, headSeq int) conversationView {
	title := t.Title
	if title == "" {
		title = untitledConversation
	}
	return conversationView{
		ArtifactID: t.ArtifactID,
		SessionID:  t.SessionID,
		Title:      title,
		VersionSeq: t.VersionSeq,
		Current:    t.VersionSeq != 0 && t.VersionSeq == headSeq,
		Resumable:  t.Resumable,
		When:       t.UpdatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		WhenISO:    t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// agentHistoryData feeds the "agentHistory" partial.
type agentHistoryData struct {
	ArtifactID string
	Items      []conversationView
}

// agentTranscriptData feeds the "agentTranscript" partial: one conversation,
// read-only, with the version it was last working against beside the one the
// artifact is at now.
type agentTranscriptData struct {
	conversationView
	HeadSeq  int
	Messages []agent.TranscriptMessage
}

// agentHistoryPartial lists the conversations kept with an artifact, for the
// chat page's history pane. Like the other fragments it renders the template
// partial the page would have, owner-scoped through the same store reads as the
// JSON route; an artifact that is not the owner's is the 404 a missing one is.
func (ro *Router) agentHistoryPartial(w http.ResponseWriter, r *http.Request) {
	ownerID, id := ownerIDFromCtx(r.Context()), r.URL.Query().Get("artifact")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		fragmentServerError(w, r, "agent history lookup", err)
		return
	}
	if a == nil {
		fragmentNotFound(w)
		return
	}
	ts, head, err := ro.transcriptList(r, ownerID, id)
	if err != nil {
		fragmentServerError(w, r, "agent history list", err)
		return
	}
	data := agentHistoryData{ArtifactID: id, Items: make([]conversationView, 0, len(ts))}
	for _, t := range ts {
		data.Items = append(data.Items, newConversationView(t, head))
	}
	ro.writeFragment(w, r, "agentHistory", data)
}

// agentTranscriptPartial shows one stored conversation read-only.
func (ro *Router) agentTranscriptPartial(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	q := r.URL.Query()
	t, head, err := ro.transcript(r, ownerID, q.Get("artifact"), q.Get("session"))
	if err != nil {
		fragmentServerError(w, r, "agent transcript lookup", err)
		return
	}
	if t == nil {
		fragmentNotFound(w)
		return
	}
	ro.writeFragment(w, r, "agentTranscript", agentTranscriptData{
		conversationView: newConversationView(*t, head),
		HeadSeq:          head,
		Messages:         agent.Messages(t),
	})
}

// writeFragment renders a named template partial as a response.
func (ro *Router) writeFragment(w http.ResponseWriter, r *http.Request, name string, data any) {
	fragment, err := renderPage(name, data)
	if err != nil {
		fragmentServerError(w, r, name+" render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, fragment)
}
