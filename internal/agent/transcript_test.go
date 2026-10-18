package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A session file as Pi writes it: a header, a system message carrying the
// prompt, the conversation, and the provider that answered. Only the
// conversation is a person's to read back.
const sessionFile = `{"type":"session","version":3,"id":"u","timestamp":"2026-10-03T10:00:00.000Z","cwd":"/work/secret"}
{"type":"model_change","id":"m1","parentId":null,"provider":"anthropic","modelId":"claude-secret-1"}
{"type":"message","id":"s1","parentId":"m1","message":{"role":"system","content":"","sections":{"preamble":"SYSTEM PROMPT TEXT"}}}
{"type":"message","id":"u1","parentId":"s1","message":{"role":"user","content":"make the button green"}}
{"type":"message","id":"a1","parentId":"u1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"PRIVATE REASONING"},{"type":"toolCall","id":"c1","name":"get_artifact","arguments":{}}],"api":"anthropic-messages","provider":"anthropic","model":"claude-secret-1","usage":{"input":1,"output":2}}}
{"type":"message","id":"t1","parentId":"a1","message":{"role":"toolResult","toolCallId":"c1","toolName":"get_artifact","content":[{"type":"text","text":"<html>THE WHOLE ARTIFACT SOURCE</html>"}],"isError":false}}
{"type":"message","id":"a2","parentId":"t1","message":{"role":"assistant","content":[{"type":"toolCall","id":"c2","name":"edit_artifact","arguments":{"edits":[{"oldText":"x","newText":"HUGE ARGUMENT"}]}}]}}
{"type":"message","id":"t2","parentId":"a2","message":{"role":"toolResult","toolCallId":"c2","toolName":"edit_artifact","content":[{"type":"text","text":"no match"}],"isError":true}}
{"type":"message","id":"a3","parentId":"t2","message":{"role":"assistant","content":[{"type":"text","text":"Done — the button is green."}]}}
{"type":"usage","id":"x1","parentId":"a3","kind":"cache_warm","provider":"anthropic","model":"claude-secret-1"}
`

// What a person sees of a stored conversation is what they saw live: what they
// said, what the assistant said, and which tools it used.
func TestProjectSessionFileKeepsWhatAPersonSaw(t *testing.T) {
	got := ProjectSessionFile(sessionFile)

	assert.Equal(t, []TranscriptMessage{
		{Role: "user", Text: "make the button green"},
		{Role: "tool", Text: "Reading artifact source"},
		{Role: "tool", Text: "Editing artifact", Failed: true},
		{Role: "assistant", Text: "Done — the button is green."},
	}, got)
}

// Everything else in a session file is Pi's, and none of it leaves the server:
// the system prompt, the model's reasoning, tool results and arguments in full,
// the provider and model, the working directory.
func TestProjectSessionFilePublishesNothingElse(t *testing.T) {
	out := ""
	for _, m := range ProjectSessionFile(sessionFile) {
		out += m.Role + "|" + m.Text + "\n"
	}
	for _, secret := range []string{
		"SYSTEM PROMPT TEXT", "PRIVATE REASONING", "THE WHOLE ARTIFACT SOURCE", "HUGE ARGUMENT",
		"claude-secret-1", "anthropic", "/work/secret", "no match",
	} {
		assert.NotContains(t, out, secret)
	}
}

// A user message can be blocks, and images are counted rather than carried.
func TestProjectSessionFileCountsImages(t *testing.T) {
	file := `{"type":"message","message":{"role":"user","content":[{"type":"text","text":"this one"},{"type":"image","data":"AAAA","mimeType":"image/png"}]}}` + "\n"
	assert.Equal(t, []TranscriptMessage{{Role: "user", Text: "this one", Images: 1}}, ProjectSessionFile(file))
}

// Pi's file is Pi's: a line that is not an entry this reads is skipped, and a
// file with nothing in it projects to nothing rather than failing.
func TestProjectSessionFileToleratesWhatItDoesNotUnderstand(t *testing.T) {
	file := "not json\n" +
		`{"type":"somethingNew","message":{"role":"user","content":"x"}}` + "\n" +
		`{"type":"message","message":{"role":"somethingNew","content":"x"}}` + "\n" +
		`{"type":"message","message":{"role":"assistant","content":"not blocks"}}` + "\n"
	assert.Empty(t, ProjectSessionFile(file))
	assert.Empty(t, ProjectSessionFile(""))
}

// A conversation kept before session files existed is Pi's message list, one
// JSON array, and projects the same way.
func TestMessagesReadsAnOlderDump(t *testing.T) {
	dump := `[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"hi"}]}]`
	got := Messages(&store.Transcript{Messages: dump})
	assert.Equal(t, []TranscriptMessage{
		{Role: "user", Text: "hello"},
		{Role: "assistant", Text: "hi"},
	}, got)

	// And a stored file wins over the dump column when there is one.
	got = Messages(&store.Transcript{Resumable: true, SessionFile: sessionFile, Messages: dump})
	assert.Equal(t, "make the button green", got[0].Text)
}

func TestToolLabelsMatchTheLiveChat(t *testing.T) {
	for name, want := range map[string]string{
		"get_artifact":   "Reading artifact source",
		"write_artifact": "Writing artifact",
		"get_state":      "Reading artifact state",
		"get_selection":  "Reading selected elements",
		"set_widget":     "Saving widget",
		"unheard_of":     "unheard_of",
	} {
		assert.Equal(t, want, toolLabel(name, nil), name)
	}
	assert.Equal(t, `Creating "Weather"`, toolLabel("create_artifact", map[string]any{"title": "Weather"}))
	assert.Equal(t, `Creating "artifact"`, toolLabel("create_artifact", nil))
	assert.Equal(t, `Setting state key "count"`, toolLabel("set_state", map[string]any{"key": "count"}))
	assert.Equal(t, `Deleting state key "count"`, toolLabel("delete_state", map[string]any{"key": "count"}))
	assert.Equal(t, "Erasing all state", toolLabel("delete_state", map[string]any{}))
}

func TestConversationTitle(t *testing.T) {
	assert.Equal(t, "make the button green", ConversationTitle("  make the\n\nbutton   green \n"))
	assert.Equal(t, "", ConversationTitle("   "))

	long := ConversationTitle(strings.Repeat("word ", 40))
	assert.LessOrEqual(t, len([]rune(long)), titleRunes)
	assert.True(t, strings.HasSuffix(long, "…"))

	// Counted in characters, not bytes: a title must never be cut mid-rune.
	assert.Equal(t, strings.Repeat("é", titleRunes-1)+"…", ConversationTitle(strings.Repeat("é", 200)))
}

// A conversation is named for the first prompt the agent received. One the
// guardrail refused never reached it — it is not in the conversation — so it
// names nothing, and the next prompt that lands does.
func TestAConversationIsNamedForItsFirstPromptThatReachedTheAgent(t *testing.T) {
	s, _ := newMeteringSession(t, false)

	s.guardBlocked = true
	s.promptLanded("something the guardrail refused")
	assert.Empty(t, s.title)

	s.guardBlocked = false
	s.promptLanded("make the button green")
	assert.Equal(t, "make the button green", s.title)

	s.promptLanded("now make it blue")
	assert.Equal(t, "make the button green", s.title, "the first prompt names it, once")
}

// Pi appends a line at a time, so a read that races an append can end partway
// through one. Half a line is not an entry.
func TestReadSessionFileReturnsOnlyWholeLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")

	got, err := readSessionFile(path)
	require.NoError(t, err)
	assert.Equal(t, "", got, "no file yet is nothing, not a failure")

	require.NoError(t, os.WriteFile(path, []byte(`{"a":1}`+"\n"+`{"b":`), 0o600))
	got, err = readSessionFile(path)
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`+"\n", got)

	require.NoError(t, os.WriteFile(path, []byte(`{"only":"a fragment"`), 0o600))
	got, err = readSessionFile(path)
	require.NoError(t, err)
	assert.Equal(t, "", got)
}
