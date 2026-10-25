package agent

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderFailureLogsFullDetailBeforeHostedRedaction(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		name := "byok"
		if hosted {
			name = "hosted"
		}
		t.Run(name, func(t *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelInfo})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			s := newDetachedSession(t, "")
			s.hideModelIdentity = hosted
			detail := "401 OpenRouter: private-provider-diagnostic; API key expired. " + strings.Repeat("diagnostic detail ", 100)
			message := map[string]any{
				"role": "assistant", "provider": "openrouter", "model": "private-model",
				"stopReason": "error", "errorMessage": detail,
				"content": []any{map[string]string{"type": "text", "text": "not-a-log-payload"}},
			}
			for _, event := range []map[string]any{
				{"type": "message_start", "message": message},
				{"type": "message_end", "message": message},
				{"type": "turn_end", "message": message},
				{"type": "agent_end", "messages": []any{message}, "willRetry": false},
			} {
				line, err := json.Marshal(event)
				require.NoError(t, err)
				s.handleLine(line)
			}

			lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
			require.Len(t, lines, 1, "one log per failed assistant response, not one per event")
			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
			assert.Equal(t, "ERROR", record["level"])
			assert.Equal(t, "agent provider request failed", record["msg"])
			assert.Equal(t, s.ID, record["session_id"])
			assert.Equal(t, "openrouter", record["provider"])
			assert.Equal(t, "private-model", record["model"])
			assert.Equal(t, detail, record["err"], "the provider's error is not truncated")
			assert.NotContains(t, logged.String(), "not-a-log-payload", "conversation content is not logged")

			require.Len(t, s.backlog, 4)
			for _, event := range s.backlog {
				if hosted {
					assert.NotContains(t, string(event), "private-provider-diagnostic")
					assert.NotContains(t, string(event), "API key")
					assert.Contains(t, string(event), platformAgentFailureMessage)
				} else {
					assert.Contains(t, string(event), "private-provider-diagnostic")
				}
			}
		})
	}
}

func TestCancellationAndToolErrorsDoNotLogProviderFailures(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	s := newDetachedSession(t, "")
	for _, line := range []string{
		`{"type":"message_end","message":{"role":"assistant","stopReason":"aborted","errorMessage":"Aborted"}}`,
		`{"type":"message_end","message":{"role":"toolResult","isError":true,"errorMessage":"Tool failed"}}`,
	} {
		s.handleLine([]byte(line))
	}
	assert.Empty(t, logged.String())
}
