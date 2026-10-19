package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func promptWithImage(data, mime string) agentPromptRequest {
	req := agentPromptRequest{Message: "describe this"}
	req.Images = append(req.Images, struct {
		Data     string `json:"data"`
		MimeType string `json:"mime_type"`
	}{data, mime})
	return req
}

func TestAgentPromptValidation(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	encoded := base64.StdEncoding.EncodeToString(png)
	for _, mime := range []string{"", "image/png"} {
		images, err := promptWithImage(encoded, mime).validate()
		require.NoError(t, err)
		require.Len(t, images, 1)
		assert.Equal(t, "image/png", images[0].MimeType)
		assert.Equal(t, encoded, images[0].Data)
	}
	_, err := (agentPromptRequest{Message: strings.Repeat("x", maxAgentMessageBytes)}).validate()
	require.NoError(t, err)
	tooMany := promptWithImage(encoded, "image/png")
	for len(tooMany.Images) <= maxAgentImages {
		tooMany.Images = append(tooMany.Images, tooMany.Images[0])
	}
	large := make([]byte, maxAgentImageBytes)
	copy(large, png)
	largeEncoded := base64.StdEncoding.EncodeToString(large)
	exact := promptWithImage(largeEncoded, "image/png")
	_, err = exact.validate()
	require.NoError(t, err)
	tooTotal := exact
	tooTotal.Images = append(tooTotal.Images, exact.Images[0], exact.Images[0], exact.Images[0])
	for _, tc := range []struct {
		name      string
		req       agentPromptRequest
		errorPart string
	}{
		{"empty", agentPromptRequest{Message: "  "}, "message is required"},
		{"text", agentPromptRequest{Message: strings.Repeat("x", maxAgentMessageBytes+1)}, "message exceeds"},
		{"count", tooMany, "at most"},
		{"empty image", promptWithImage("", "image/png"), "empty"},
		{"encoding", promptWithImage("not base64!", "image/png"), "base64"},
		{"mime", promptWithImage(encoded, "image/svg+xml"), "unsupported MIME"},
		{"mismatch", promptWithImage(encoded, "image/jpeg"), "does not match"},
		{"size", promptWithImage(base64.StdEncoding.EncodeToString(append(large, 0)), "image/png"), "decoded limit"},
		{"total", tooTotal, "total decoded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.req.validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errorPart)
		})
	}
}

func TestInvalidAgentPromptNeverWritesToSubprocess(t *testing.T) {
	r := newTestRouter(t)
	root := t.TempDir()
	writes := filepath.Join(root, "writes")
	script := filepath.Join(root, "pi")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexec cat > '"+writes+"'\n"), 0700))
	mgr, err := agent.New(agent.Config{PiBin: script, WorkRoot: root, Credentials: agentscope.NewRegistry()}, r.cfg.Store)
	require.NoError(t, err)
	r.cfg.Agent = mgr
	session, err := mgr.Create(context.Background(), agent.CreateOpts{OwnerID: defaultOwnerID, Provider: "anthropic", APIKey: "test"})
	require.NoError(t, err)
	defer mgr.Close(defaultOwnerID, session.ID)
	for _, req := range []agentPromptRequest{
		{Message: strings.Repeat("x", maxAgentMessageBytes+1)},
		promptWithImage("broken", "image/png"),
	} {
		body, err := json.Marshal(req)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		request := httptest.NewRequest("POST", "/api/agent/sessions/"+session.ID+"/prompt", bytes.NewReader(body)).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+r.cfg.AuthToken)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		cancel()
		require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	}
	b, err := os.ReadFile(writes)
	if !os.IsNotExist(err) {
		require.NoError(t, err)
		assert.Empty(t, b, "validation must precede every subprocess write")
	}
}
