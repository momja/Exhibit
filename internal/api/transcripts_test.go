package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Agent conversations kept with an artifact (av-y7td), read as JSON and as the
// fragments the chat page's history pane swaps in.

// piFile is a conversation as Pi records it, with the parts that must never
// leave the server planted in it.
const piFile = `{"type":"session","version":3,"id":"u","cwd":"/work/SECRET-CWD"}
{"type":"message","id":"s1","message":{"role":"system","content":"","sections":{"preamble":"SECRET-SYSTEM-PROMPT"}}}
{"type":"message","id":"u1","message":{"role":"user","content":"make the button <b>green</b>"}}
{"type":"message","id":"a1","message":{"role":"assistant","content":[{"type":"toolCall","id":"c1","name":"get_artifact","arguments":{}}],"provider":"SECRET-PROVIDER","model":"SECRET-MODEL"}}
{"type":"message","id":"t1","message":{"role":"toolResult","toolCallId":"c1","toolName":"get_artifact","content":[{"type":"text","text":"SECRET-ARTIFACT-SOURCE"}],"isError":false}}
{"type":"message","id":"a2","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}
`

func keepConversation(t *testing.T, r *Router, artifactID, session, title, file string) {
	t.Helper()
	require.NoError(t, r.cfg.Store.SaveTranscript(context.Background(), defaultOwnerID, store.Transcript{
		ArtifactID: artifactID, SessionID: session, Title: title, SessionFile: file,
	}))
}

func newConversationFixture(t *testing.T) (*Router, string) {
	t.Helper()
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": "<html><body>one</body></html>"})
	keepConversation(t, r, id, "sess-1", "make the button green", piFile)
	return r, id
}

type transcriptListBody struct {
	HeadSeq     int `json:"head_seq"`
	Transcripts []struct {
		SessionID  string `json:"session_id"`
		Title      string `json:"title"`
		VersionSeq int    `json:"version_seq"`
		Resumable  bool   `json:"resumable"`
	} `json:"transcripts"`
}

func TestListingConversationsReportsTheVersionEachWasWorkingAgainst(t *testing.T) {
	r, id := newConversationFixture(t)

	w := doJSON(t, r, "GET", "/api/artifacts/"+id+"/transcripts", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got transcriptListBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Transcripts, 1)
	assert.Equal(t, 1, got.HeadSeq)
	assert.Equal(t, "sess-1", got.Transcripts[0].SessionID)
	assert.Equal(t, "make the button green", got.Transcripts[0].Title)
	assert.Equal(t, 1, got.Transcripts[0].VersionSeq)
	assert.True(t, got.Transcripts[0].Resumable)

	// The artifact moves on; the conversation still says what it last saw.
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>two</body></html>"})
	w = doJSON(t, r, "GET", "/api/artifacts/"+id+"/transcripts", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 2, got.HeadSeq)
	assert.Equal(t, 1, got.Transcripts[0].VersionSeq)
}

// What leaves the server is what a person saw, never Pi's file.
func TestReadingAConversationReturnsOnlyWhatWasSaid(t *testing.T) {
	r, id := newConversationFixture(t)

	w := doJSON(t, r, "GET", "/api/artifacts/"+id+"/transcripts/sess-1", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	for _, secret := range []string{"SECRET-CWD", "SECRET-SYSTEM-PROMPT", "SECRET-PROVIDER", "SECRET-MODEL", "SECRET-ARTIFACT-SOURCE", "session_file"} {
		assert.NotContains(t, body, secret)
	}

	var got struct {
		SessionID string `json:"session_id"`
		HeadSeq   int    `json:"head_seq"`
		Messages  []struct {
			Role string `json:"role"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "sess-1", got.SessionID)
	assert.Equal(t, 1, got.HeadSeq)
	require.Len(t, got.Messages, 3)
	assert.Equal(t, "user", got.Messages[0].Role)
	assert.Equal(t, "make the button <b>green</b>", got.Messages[0].Text)
	assert.Equal(t, "Reading artifact source", got.Messages[1].Text)
	assert.Equal(t, "Done.", got.Messages[2].Text)

	w = doJSON(t, r, "GET", "/api/artifacts/"+id+"/transcripts/nope", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// The history is the owner's. A session credential — steered by text Exhibit did
// not author — cannot read the conversations before it: deny-by-default, and
// `transcripts` has no entry in agentSubResources.
func TestAnAgentCredentialCannotReadConversations(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	keepConversation(t, r, id, "sess-1", "t", piFile)
	grant, err := reg.Issue(1, id, "sess-x")
	require.NoError(t, err)

	for _, path := range []string{
		"/api/artifacts/" + id + "/transcripts",
		"/api/artifacts/" + id + "/transcripts/sess-1",
	} {
		w := doWithToken(t, r, "GET", path, grant.Token(), nil)
		assert.Equal(t, http.StatusForbidden, w.Code, path)
	}
}

func TestHistoryFragmentListsConversations(t *testing.T) {
	r, id := newConversationFixture(t)
	keepConversation(t, r, id, "sess-2", `<img src=x onerror=alert(1)>`, piFile)

	frag := getPage(t, r, "/partials/agent-history?artifact="+id)

	assert.Contains(t, frag, `class="history-list"`)
	assert.Contains(t, frag, "make the button green")
	assert.Contains(t, frag, "Based on v1 · current")
	assert.Contains(t, frag, `hx-get="/partials/agent-transcript?artifact=`+id+`&amp;session=sess-1"`)
	assert.Contains(t, frag, `hx-target="#history"`)
	assert.Contains(t, frag, `onclick="closeHistory()"`)
	// A title is the user's prompt — still text, not markup.
	assert.NotContains(t, frag, "<img src=x")
	assert.Contains(t, frag, "&lt;img src=x")
}

func TestHistoryFragmentSaysSoWhenThereIsNone(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})

	frag := getPage(t, r, "/partials/agent-history?artifact="+id)
	assert.Contains(t, frag, "No earlier conversations")
	assert.NotContains(t, frag, "history-list")
}

// A conversation whose artifact has since moved on says which version it was
// working against, and that the artifact is no longer there.
func TestHistoryFragmentMarksAConversationTheArtifactHasMovedPast(t *testing.T) {
	r, id := newConversationFixture(t)
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>two</body></html>"})

	frag := getPage(t, r, "/partials/agent-history?artifact="+id)
	assert.Contains(t, frag, "Based on v1")
	assert.NotContains(t, frag, "Based on v1 · current")
}

func TestTranscriptFragmentShowsTheConversationReadOnly(t *testing.T) {
	r, id := newConversationFixture(t)

	frag := getPage(t, r, "/partials/agent-transcript?artifact="+id+"&session=sess-1")

	assert.Contains(t, frag, `<h2 class="history-title">make the button green</h2>`)
	// The same bubbles and chips the live chat draws.
	assert.Contains(t, frag, `<div class="msg user">make the button &lt;b&gt;green&lt;/b&gt;</div>`)
	assert.Contains(t, frag, `<div class="tool-chip done"><i class="ph ph-check-circle"></i> Reading artifact source</div>`)
	assert.Contains(t, frag, `<div class="msg assistant">Done.</div>`)
	// Nothing in it talks to the live session.
	assert.NotContains(t, frag, "composer")
	assert.NotContains(t, frag, "<textarea")
	for _, secret := range []string{"SECRET-CWD", "SECRET-SYSTEM-PROMPT", "SECRET-PROVIDER", "SECRET-MODEL", "SECRET-ARTIFACT-SOURCE"} {
		assert.NotContains(t, frag, secret)
	}
	// And back to the list.
	assert.Contains(t, frag, `hx-get="/partials/agent-history?artifact=`+id+`"`)
}

// An unknown conversation or artifact answers with the shared fragment error,
// which htmx swaps into the pane in place of what it had.
func TestHistoryFragmentsAnswerNotFound(t *testing.T) {
	r, id := newConversationFixture(t)

	for _, path := range []string{
		"/partials/agent-history?artifact=no-such-artifact",
		"/partials/agent-transcript?artifact=" + id + "&session=no-such-session",
		"/partials/agent-transcript?artifact=no-such-artifact&session=sess-1",
	} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code, path)
		assert.Contains(t, w.Body.String(), `class="frag-error"`, path)
	}
}

// The history button is the chat's own control, and there is nothing to have a
// history of until the page is about an artifact — so it is rendered hidden, and
// agent.js reveals it.
func TestAgentPageCarriesTheHistoryPane(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/agent")

	assert.Contains(t, page, `id="history-btn" hidden`)
	assert.Contains(t, page, `hx-get="/partials/agent-history"`)
	assert.Contains(t, page, `hx-vals='js:{artifact: previewArtifactId()}'`)
	assert.Contains(t, page, `<div id="history"></div>`)

	js, err := embeddedAssets.ReadFile("assets/gallery/agent.js")
	require.NoError(t, err)
	for _, want := range []string{"function openHistory()", "function closeHistory()", "function syncHistoryButton()"} {
		assert.True(t, strings.Contains(string(js), want), want)
	}
}

// --- Continuing a conversation (av-b4yh): the card under the transcript ----------

// The decision is the card's whole job, so what these pin is its shape: which
// choices exist, which is the primary one, and that nothing is a missing button.
func resumeCardFixture(t *testing.T) (*Router, string) {
	t.Helper()
	r := newResumeRefusalRouter(t) // an instance that can run an agent
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": "<html><body>one</body></html>"})
	keepConversation(t, r, id, "sess-1", "make the button green", piFile)
	return r, id
}

func TestTheCardOffersASingleContinueWhileTheArtifactIsWhereTheConversationLeftIt(t *testing.T) {
	r, id := resumeCardFixture(t)

	frag := getPage(t, r, "/partials/agent-transcript?artifact="+id+"&session=sess-1")

	assert.Contains(t, frag, `data-resume="sess-1" data-rollback="0"><i class="ph ph-chat-circle"></i> Continue this conversation`)
	assert.NotContains(t, frag, "Roll back")
	assert.NotContains(t, frag, "without rolling back")
	assert.Contains(t, frag, `id="history-messages"`, "the page carries the conversation across from here")
}

func TestTheCardPutsTheRollbackToThePersonAndKeepsTheEscapeSecondary(t *testing.T) {
	r, id := resumeCardFixture(t)
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>two</body></html>"}) // v2: it has moved on

	frag := getPage(t, r, "/partials/agent-transcript?artifact="+id+"&session=sess-1")

	// What is true: which version the conversation was working against, and where
	// the artifact is now.
	assert.Contains(t, frag, "last working against <strong>v1</strong>")
	assert.Contains(t, frag, "now at <strong>v2</strong>")

	// The rollback is the primary button, and says what it costs beside it.
	rollback := `<button type="button" class="btn" data-resume="sess-1" data-rollback="1"><i class="ph ph-arrow-counter-clockwise"></i> Roll back to v1 and continue</button>`
	assert.Contains(t, frag, rollback)
	assert.Contains(t, frag, "recorded as a new version, so nothing is lost")
	assert.Contains(t, frag, "undo it from the Versions list")

	// The escape is a plain secondary button — never the primary one — and leaves the
	// artifact alone.
	escape := `<button type="button" class="btn btn-sec" data-resume="sess-1" data-rollback="0">Continue without rolling back</button>`
	assert.Contains(t, frag, escape)
	assert.Contains(t, frag, "Leaves the artifact as it is now")
	assert.Contains(t, frag, "reads it again before changing anything")
	assert.Less(t, strings.Index(frag, rollback), strings.Index(frag, escape),
		"the rollback comes first, and the escape is the second choice")

	// And a way out that changes nothing.
	assert.Contains(t, frag, `class="resume-cancel"`)
	assert.Contains(t, frag, `hx-get="/partials/agent-history?artifact=`+id+`"`)
	// Nothing about the decision is hidden away in script: it is all here to read.
	assert.Contains(t, frag, `role="status"`)
}

// A conversation that cannot be continued says why where the card would be.
func TestTheCardIsANoteWhereAConversationCannotBeContinued(t *testing.T) {
	r, _, dbPath := newResumeRefusalRouterAt(t) // an instance that can run an agent
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`INSERT INTO agent_transcripts (artifact_id, session_id, title, messages)
	                  VALUES (?, 'old', 'from before', '[{"role":"user","content":"hello"}]')`, id)
	require.NoError(t, err)

	frag := getPage(t, r, "/partials/agent-transcript?artifact="+id+"&session=old")
	assert.Contains(t, frag, "kept before conversations could be continued, so it can only be read")
	assert.NotContains(t, frag, "data-resume")
	// It can still be read.
	assert.Contains(t, frag, `<div class="msg user">hello</div>`)
}

func TestTheCardIsANoteOnAnInstanceWithNoAgent(t *testing.T) {
	r := newTestRouter(t) // no agent manager
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	keepConversation(t, r, id, "sess-1", "t", piFile)

	frag := getPage(t, r, "/partials/agent-transcript?artifact="+id+"&session=sess-1")
	assert.Contains(t, frag, "Agent support is off on this server")
	assert.NotContains(t, frag, "data-resume")
}
