package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise two real Pi edit calls against the retained source. The scripted
// provider can also consume a host-prepared read, so this contract continues
// to hold when eager source loading is enabled above this layer.
func TestAgentSuccessivePatchesReuseTheReadSource(t *testing.T) {
	h := newPiHarness(t)
	h.llm.inner = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []recordedMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		read, edited := false, 0
		for _, message := range request.Messages {
			for _, call := range message.ToolCalls {
				switch call.Function.Name {
				case "get_artifact":
					read = true
				case "edit_artifact":
					edited++
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta map[string]any, finish any) {
			chunk, err := json.Marshal(map[string]any{
				"id": "patch-test", "object": "chat.completion.chunk", "created": 0, "model": "exhibit-mock-1",
				"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
			})
			if err != nil {
				panic(err)
			}
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		if edited >= 2 {
			send(map[string]any{"role": "assistant", "content": "Done."}, "stop")
		} else {
			name, args := "get_artifact", map[string]any{}
			if read {
				oldText, newText := "old", "first"
				if edited == 1 {
					oldText, newText = "first", "second"
				}
				name, args = "edit_artifact", map[string]any{"edits": []map[string]string{{"oldText": oldText, "newText": newText}}}
			}
			encoded, _ := json.Marshal(args)
			send(map[string]any{"role": "assistant", "tool_calls": []map[string]any{{
				"index": 0, "id": fmt.Sprintf("patch_test_%s_%d", name, edited), "type": "function",
				"function": map[string]any{"name": name, "arguments": string(encoded)},
			}}}, nil)
			send(map[string]any{}, "tool_calls")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	id := createArtifact(t, h.router, map[string]any{"title": "Patch", "body": "<h1>old</h1><p>keep 🌼</p>"})
	modify(t, h, id, "Make two small successive edits.")
	assert.Equal(t, "<h1>second</h1><p>keep 🌼</p>", getArtifactBody(t, h.router, id))
	require.Equal(t, []string{"get_artifact", "edit_artifact", "edit_artifact"}, h.llm.toolCallNames())
}
