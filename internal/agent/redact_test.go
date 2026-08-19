package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures below are verbatim lines from a real `pi --mode rpc` turn
// (v0.84.1), trimmed only in length. They are what makes this a test of Pi's
// actual protocol rather than of an assumed one: every assistant message it
// emits carries api/provider/model, on message_start, message_end, turn_end
// and agent_end alike.
const (
	piTurnEnd  = `{"type":"turn_end","message":{"role":"assistant","content":[{"type":"toolCall","id":"call_1","name":"update_artifact","arguments":{"body":"<html>ok</html>"}}],"api":"openai-completions","provider":"anthropic","model":"claude-sonnet-4-5","usage":{"input":12,"output":34,"totalTokens":46,"cost":{"total":0.002}},"stopReason":"toolUse","timestamp":1787034911097,"responseId":"chatcmpl-1"},"toolResults":[{"role":"toolResult","toolCallId":"call_1","toolName":"update_artifact","content":[{"type":"text","text":"Updated artifact abc."}],"isError":false,"timestamp":1787034911128}]}`
	piAgentEnd = `{"type":"agent_end","messages":[{"role":"user","content":[{"type":"text","text":"make it green"}],"timestamp":1787034911051},{"role":"assistant","content":[],"api":"openai-completions","provider":"anthropic","model":"claude-sonnet-4-5","timestamp":1787034911097}],"willRetry":false}`
)

func TestRedactModelIdentityStripsPiMessageEnvelopes(t *testing.T) {
	for _, line := range []string{piTurnEnd, piAgentEnd} {
		out := redactModelIdentity([]byte(line))
		assert.NotContains(t, string(out), "anthropic")
		assert.NotContains(t, string(out), "claude-sonnet-4-5")
		assert.NotContains(t, string(out), "openai-completions")
	}
}

// Everything that is not an identifier survives, including the usage block:
// it names no model, and metering (av-hyo6) is what will read it.
func TestRedactModelIdentityKeepsEverythingElse(t *testing.T) {
	out := redactModelIdentity([]byte(piTurnEnd))
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))

	msg := got["message"].(map[string]any)
	assert.Equal(t, "assistant", msg["role"])
	assert.Equal(t, "toolUse", msg["stopReason"])
	assert.NotNil(t, msg["usage"], "token usage is not an identifier and metering will need it")
	assert.NotNil(t, msg["content"])
	assert.NotNil(t, got["toolResults"])
}

// Timestamps are milliseconds since the epoch and must survive the round trip
// as integers — a float64 decode would re-marshal them in scientific notation
// and change every event on the wire.
func TestRedactModelIdentityPreservesNumberLiterals(t *testing.T) {
	out := redactModelIdentity([]byte(piTurnEnd))
	assert.Contains(t, string(out), "1787034911097")
	assert.NotContains(t, string(out), "e+")
}

// The match is on Pi's message envelope, not on the key name: a "model" the
// artifact's own data happens to carry is the artifact's, and stripping it
// would corrupt what the chat shows of a tool call.
func TestRedactModelIdentityLeavesArtifactDataAlone(t *testing.T) {
	line := `{"type":"tool_execution_start","toolName":"set_state","args":{"key":"car","value":{"provider":"Volvo","model":"240"}}}`
	out := redactModelIdentity([]byte(line))
	assert.Contains(t, string(out), "Volvo")
	assert.Contains(t, string(out), "240")
}

// An untouched line comes back as the exact bytes Pi wrote, so the common case
// costs no re-serialization and no key reordering.
func TestRedactModelIdentityReturnsCleanLinesUnchanged(t *testing.T) {
	line := []byte(`{"type":"agent_start"}`)
	assert.Equal(t, string(line), string(redactModelIdentity(line)))
}

// A transcript is the same filter over Pi's message list — the second seam
// that publishes its protocol, and the one that keeps a record of it.
func TestRedactModelIdentityOverATranscript(t *testing.T) {
	messages := `[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[],"api":"openai-completions","provider":"openai","model":"gpt-5.2"}]`
	out := redactModelIdentity([]byte(messages))
	assert.NotContains(t, string(out), "gpt-5.2")
	assert.NotContains(t, string(out), "openai")
	assert.Contains(t, string(out), `"hi"`)
}
