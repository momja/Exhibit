package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Continuing a stored conversation (av-b4yh). The refusals first, which need no
// agent process; then the real thing against a `pi` sidecar, because what a
// resumed conversation hands the model is Pi's doing and nothing else is
// evidence of it.

// newResumeRefusalRouter is a router with an agent manager that can never start
// a process — enough for every answer the route gives before one would start.
func newResumeRefusalRouter(t *testing.T) *Router {
	t.Helper()
	r, _, _ := newResumeRefusalRouterAt(t)
	return r
}

func newResumeRefusalRouterAt(t *testing.T) (*Router, string, string) {
	t.Helper()
	r, blobDir, dbPath := newTestRouterAt(t)
	creds := agentscope.NewRegistry()
	mgr, err := agent.New(agent.Config{
		PiBin: "/no/such/pi", WorkRoot: t.TempDir(), Credentials: creds, MockLLMURL: "http://127.0.0.1:1",
	}, r.cfg.Store)
	require.NoError(t, err)
	r.cfg.Agent, r.cfg.AgentCredentials, r.cfg.MockEnabled = mgr, creds, true
	w := doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "exhibit-mock", "model": "exhibit-mock-1", "api_key": "k"})
	require.Equal(t, http.StatusOK, w.Code)
	return r, blobDir, dbPath
}

func resumeReq(t *testing.T, r *Router, body map[string]string) (int, string) {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/agent/sessions", body)
	return w.Code, w.Body.String()
}

func TestResumingNeedsTheArtifactItBelongsTo(t *testing.T) {
	r := newResumeRefusalRouter(t)
	code, body := resumeReq(t, r, map[string]string{"resume_session_id": "whatever"})
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, "needs artifact_id")
}

func TestResumingAConversationThatIsNotThereIsNotFound(t *testing.T) {
	r := newResumeRefusalRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})

	code, body := resumeReq(t, r, map[string]string{"artifact_id": id, "resume_session_id": "no-such-conversation"})
	assert.Equal(t, http.StatusNotFound, code, body)
}

// Another owner's artifact answers as one that does not exist, and so does
// their conversation: the route is not a way to learn which ids are real.
func TestResumingSomeoneElsesConversationIsNotFound(t *testing.T) {
	r := newResumeRefusalRouter(t)
	foreign := seedForeignArtifact(t, r)
	require.NoError(t, r.cfg.Store.SaveTranscript(context.Background(), otherOwner, store.Transcript{
		ArtifactID: foreign, SessionID: "theirs", Title: "t", SessionFile: "{}\n"}))

	theirs, _ := resumeReq(t, r, map[string]string{"artifact_id": foreign, "resume_session_id": "theirs"})
	ghost, _ := resumeReq(t, r, map[string]string{"artifact_id": "no-such-artifact", "resume_session_id": "theirs"})
	assert.Equal(t, http.StatusNotFound, theirs)
	assert.Equal(t, ghost, theirs)
}

// A conversation kept before session files existed has nothing to start Pi from.
func TestAConversationKeptBeforeSessionFilesCannotBeResumed(t *testing.T) {
	r, _, dbPath := newResumeRefusalRouterAt(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`INSERT INTO agent_transcripts (artifact_id, session_id, messages) VALUES (?, 'old', '[]')`, id)
	require.NoError(t, err)

	code, body := resumeReq(t, r, map[string]string{"artifact_id": id, "resume_session_id": "old"})
	assert.Equal(t, http.StatusConflict, code, body)
	assert.Contains(t, body, "can only be read")
}

// --- Against a real Pi --------------------------------------------------------

// resume starts a stored conversation again and returns its session id.
func resume(t *testing.T, r *Router, artifactID, conversationID string) string {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{
		"artifact_id": artifactID, "resume_session_id": conversationID})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var got struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	t.Cleanup(func() { r.cfg.Agent.Close(defaultOwnerID, got.ID) })
	return got.ID
}

func prompt(t *testing.T, r *Router, session, message string) {
	t.Helper()
	events, unsubscribe := r.cfg.Agent.Get(defaultOwnerID, session).Subscribe()
	defer unsubscribe()
	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt", map[string]any{"message": message})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	settles(t, events, 1)
}

// What continuing a conversation hands the model: the conversation as it was,
// under the current instructions, with a note that the artifact may have moved.
func TestAResumedConversationHasItsHistoryAndIsToldToReadAgain(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})

	original := conversation(t, h, id, "make the button purple")
	closeAndWait(t, r, original)

	resumed := resume(t, r, id, original)
	assert.Equal(t, original, resumed, "a conversation keeps its identity")
	prompt(t, r, resumed, "now make it red")
	waitForBody(t, r, id, func(b string) bool { return strings.Contains(b, "#d64545") }, "the resumed agent to recolor")

	// The model's last request holds everything said before, then the new
	// message carrying the note — exactly once, on the first prompt only.
	last := lastTurn(t, h.llm)
	users := messagesWithRole(last, "user")
	require.Len(t, users, 2, "the old prompt and the new one")
	assert.Equal(t, "make the button purple", users[0])
	assert.True(t, strings.HasPrefix(users[1], "now make it red"), users[1])
	assert.Contains(t, users[1], "may have changed since you last read it")
	assert.Equal(t, 1, strings.Count(strings.Join(users, "\n"), "may have changed since you last read it"))
	assert.NotEmpty(t, messagesWithRole(last, "assistant"), "what the assistant said before is still there")

	// The instructions are the current ones, and the stored copy is not
	// replayed beside them.
	assert.Len(t, messagesWithRole(last, "system"), 1)

	// A later prompt in the same session does not say it again.
	prompt(t, r, resumed, "and a little bigger")
	users = messagesWithRole(lastTurn(t, h.llm), "user")
	assert.Equal(t, 1, strings.Count(strings.Join(users, "\n"), "may have changed since you last read it"))
}

// The conversation is one record that goes on, not a second one.
func TestAResumedConversationIsKeptInTheSameRecord(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})
	original := conversation(t, h, id, "make the button purple")
	closeAndWait(t, r, original)

	resumed := resume(t, r, id, original)
	prompt(t, r, resumed, "now make it red")

	kept := keptConversation(t, r, id, original, 2)
	assert.Equal(t, "make the button purple", kept.Title, "named by how it began, however it went on")
	assert.Equal(t, 3, kept.VersionSeq, "two agent writes on top of the first version")
	list, err := r.cfg.Store.ListTranscripts(context.Background(), defaultOwnerID, id)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// And the versions the resumed turn wrote carry the conversation's own id.
	vs, err := r.cfg.Store.ListVersions(context.Background(), defaultOwnerID, id)
	require.NoError(t, err)
	assert.Equal(t, original, vs[0].SessionID)
}

// Continuing without rolling back: the artifact is left alone, so a person's
// edit made since the conversation last ran is still there afterwards, because
// the agent read the artifact as it now is.
func TestContinuingWithoutRollingBackKeepsWhatChangedSince(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})
	original := conversation(t, h, id, "make the button purple")
	closeAndWait(t, r, original)
	patchArtifact(t, r, id, map[string]any{"body": strings.Replace(artifactBody(t, r, id), "Count!", "Tally!", 1)})

	resumed := resume(t, r, id, original)
	prompt(t, r, resumed, "now make it red")

	body := artifactBody(t, r, id)
	assert.Contains(t, body, "Tally!", "the person's edit survives")
	assert.Contains(t, body, "#d64545")
}

// Rolling back first, as the page does on a confirmed choice: the agent then
// reads the artifact as the version left it, and the edit made since is not
// part of what it builds on.
func TestContinuingAfterARollbackBuildsOnTheRestoredVersion(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})
	original := conversation(t, h, id, "make the button purple")
	closeAndWait(t, r, original)
	kept := keptConversation(t, r, id, original, 1)
	patchArtifact(t, r, id, map[string]any{"body": strings.Replace(artifactBody(t, r, id), "Count!", "Tally!", 1)})
	require.NotEqual(t, kept.VersionSeq, headSeq(t, r, id), "the artifact has moved on")

	require.Equal(t, http.StatusOK, restoreReq(t, r, id, kept.VersionSeq).Code)
	resumed := resume(t, r, id, original)
	prompt(t, r, resumed, "now make it red")

	body := artifactBody(t, r, id)
	assert.NotContains(t, body, "Tally!", "the edit made after that version is not carried forward")
	assert.Contains(t, body, "Count!")
	assert.Contains(t, body, "#d64545")
}

// A conversation that is still running is continued by attaching to it: one
// process, one writer of the record.
func TestResumingARunningConversationAttachesToIt(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})
	live := conversation(t, h, id, "make the button purple")

	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{"artifact_id": id, "resume_session_id": live})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct {
		ID        string `json:"id"`
		SSETicket string `json:"sse_ticket"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, live, got.ID)
	assert.NotEmpty(t, got.SSETicket, "the caller can stream from it")
}

// A new conversation is the default. A session created without a conversation to
// continue starts a new one whatever the artifact has kept: nothing said before
// reaches the model, there is no "the artifact may have changed" note (it
// remembers nothing that could be stale), and what was kept is left exactly as it
// was. Continuing a conversation is asked for by name; it is never what opening a
// chat does.
func TestAChatOnAnArtifactThatHasHistoryStartsANewConversation(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})
	earlier := conversation(t, h, id, "make the button purple")
	closeAndWait(t, r, earlier)
	before := keptConversation(t, r, id, earlier, 1)

	fresh := conversation(t, h, id, "now make it red")
	assert.NotEqual(t, earlier, fresh, "a new conversation, not the one that was kept")

	// The model was asked to continue this chat and nothing else.
	users := messagesWithRole(lastTurn(t, h.llm), "user")
	assert.Equal(t, []string{"now make it red"}, users)

	// The kept conversation is exactly as it was, with the new one beside it.
	after := keptConversation(t, r, id, earlier, 1)
	assert.Equal(t, before.SessionFile, after.SessionFile)
	assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "it was not touched")
	list, err := r.cfg.Store.ListTranscripts(context.Background(), defaultOwnerID, id)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

// The guardrail screens a prompt before the model sees it, so a refused first
// prompt on a resumed conversation carried the "the artifact may have changed"
// note to nobody. The model is still owed it, and the refused words are not in
// the conversation it is shown.
func TestTheResumeNoteIsStillOwedAfterARefusedPrompt(t *testing.T) {
	h := newPiHarnessWith(t, false, mockGuardrail)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})
	original := conversation(t, h, id, "make the button purple")
	closeAndWait(t, r, original)

	resumed := resume(t, r, id, original)
	s := r.cfg.Agent.Get(defaultOwnerID, resumed)
	require.NotNil(t, s)
	events, unsubscribe := s.Subscribe()
	defer unsubscribe()
	w := doJSON(t, r, "POST", "/api/agent/sessions/"+resumed+"/prompt",
		map[string]any{"message": "mock-policy-violation now make it red"})
	require.Equal(t, http.StatusAccepted, w.Code)
	awaitEvent(t, events, isGuardBlocked, "the guardrail to refuse the prompt")

	prompt(t, r, resumed, "now make it red")

	users := messagesWithRole(lastTurn(t, h.llm), "user")
	require.Len(t, users, 2, "the first prompt, and the one that was let through")
	assert.Contains(t, users[1], "may have changed since you last read it")
	assert.NotContains(t, strings.Join(users, "\n"), "mock-policy-violation")
	assert.Equal(t, 1, strings.Count(strings.Join(users, "\n"), "may have changed since you last read it"))
}

// --- helpers ------------------------------------------------------------------

func headSeq(t *testing.T, r *Router, artifactID string) int {
	t.Helper()
	seq, err := r.cfg.Store.HeadVersionSeq(context.Background(), defaultOwnerID, artifactID)
	require.NoError(t, err)
	return seq
}

// lastTurn is the conversation the model was most recently asked to continue.
func lastTurn(t *testing.T, rec *transcriptRecorder) []recordedMessage {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.NotEmpty(t, rec.turns)
	return rec.turns[len(rec.turns)-1]
}

func messagesWithRole(turn []recordedMessage, role string) []string {
	var out []string
	for _, m := range turn {
		if m.Role == role {
			out = append(out, strings.TrimSpace(messageText(m.Content)))
		}
	}
	return out
}
