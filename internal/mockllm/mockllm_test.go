package mockllm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// get_artifact's result is a label, a metadata block and the source, joined by
// blank lines. The source is everything after the second one — and it may hold
// blank lines of its own, as any real document does.
func TestBodyFromSourceResult(t *testing.T) {
	const source = "<html>\n\n<body>x</body>\n\n</html>"
	result := "current source of the artifact this session is editing:\n\n" +
		"id: abc\ntitle: T\nallowlist: []\nassets: none\n\n" + source
	assert.Equal(t, source, bodyFromSourceResult(result))

	// The assets block spans several lines when an artifact has vendored
	// payloads; none of them is a blank line, so none of them is the source.
	withAssets := "current source of the artifact this session is editing:\n\n" +
		"id: abc\ntitle: T\nallowlist: []\nassets: these URLs are served from stored copies\n" +
		"below work even though the files are not in the source.\n  - https://x/app.wasm (application/wasm, 10 bytes)\n\n" + source
	assert.Equal(t, source, bodyFromSourceResult(withAssets))

	assert.Equal(t, "no structure", bodyFromSourceResult("no structure"))
}

func msg(role, content string) chatMessage {
	raw, _ := json.Marshal(content)
	return chatMessage{Role: role, Content: raw}
}

// A session with an artifact never starts by writing: nothing about the
// artifact is in the conversation until the model reads it.
func TestDecideReadsBeforeItChanges(t *testing.T) {
	system := msg("system", "You are Exhibit.\n\nThis session is editing an artifact that already exists.")

	plan := decide([]chatMessage{system, msg("user", "make the button green")})
	assert.Equal(t, "tool", plan.kind)
	assert.Equal(t, "get_artifact", plan.toolName)

	// A prompt that says elements were selected reads them first.
	plan = decide([]chatMessage{system, msg("user", "make this green\n\n(The user selected 1 element in the artifact preview — read it with get_selection.)")})
	assert.Equal(t, "get_selection", plan.toolName)

	// A state command needs no read of the source.
	plan = decide([]chatMessage{system, msg("user", "list state")})
	assert.Equal(t, "get_state", plan.toolName)
}

// A session that has only just made its artifact has one too: the next prompt
// reads it rather than creating a second.
func TestDecideAfterACreateReadsTheArtifact(t *testing.T) {
	system := msg("system", "You are Exhibit.")
	history := []chatMessage{
		system,
		msg("user", "a counter"),
		{Role: "assistant", ToolCalls: []toolCall{{ID: "c1"}}},
		{Role: "tool", ToolCallID: "c1", Content: json.RawMessage(`"Created artifact abc (\"Counter\")."`)},
		msg("assistant", "Done!"),
		msg("user", "make it blue"),
	}
	assert.Equal(t, "get_artifact", decide(history).toolName)
}

// An unbound session's first prompt creates; a widget-only session writes its
// tile whatever it holds.
func TestDecideCreateAndWidget(t *testing.T) {
	assert.Equal(t, "create_artifact",
		decide([]chatMessage{msg("system", "You are Exhibit."), msg("user", "a counter")}).toolName)

	widget := msg("system", "This session has exactly one job: build the gallery widget for its artifact.")
	assert.Equal(t, "set_widget", decide([]chatMessage{widget, msg("user", "Build the gallery widget.")}).toolName)
}
