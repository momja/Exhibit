package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type connectorAbortStdin struct{ lines chan []byte }

func (w *connectorAbortStdin) Close() error { return nil }
func (w *connectorAbortStdin) Write(p []byte) (int, error) {
	w.lines <- append([]byte(nil), p...)
	return len(p), nil
}

func TestAbortPrecedesConnectorApproval(t *testing.T) {
	s, _ := newQuestionSession(t)
	s.expireInteractions("")
	s.pending = make(map[string]chan rpcResponse)
	s.done = make(chan struct{})
	input := &connectorAbortStdin{lines: make(chan []byte, 4)}
	s.stdin = input
	payload, _ := json.Marshal(map[string]string{"toolCallId": "tool-api", "proposalId": uuid.NewString()})
	request, _ := json.Marshal(map[string]string{"type": "extension_ui_request", "method": "input", "id": "api-1", "title": apiConnectorDialogTitle, "placeholder": string(payload)})
	s.handleLine(request)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	abortDone := make(chan error, 1)
	go func() { abortDone <- s.Abort(ctx) }()
	abortLine := <-input.lines // Abort was sent first, but Pi has not acknowledged it.
	called := false
	err := s.ResolveAPIConnector("api-1", 1, func(string) (string, error) { called = true; return "approved", nil })
	var command map[string]any
	require.NoError(t, json.Unmarshal(abortLine, &command))
	response, _ := json.Marshal(map[string]any{"type": "response", "id": command["id"]})
	s.handleLine(response)
	require.NoError(t, <-abortDone)
	require.False(t, called, "Stop already reached Pi before approval: no connector may be committed")
	require.ErrorIs(t, err, ErrInteractionExpired)
}
