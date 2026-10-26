package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonProcessHelper(t *testing.T) {
	for i, arg := range os.Args {
		if arg == "--daemon-helper" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			flag.CommandLine = flag.NewFlagSet("exhibit-agent", flag.ExitOnError)
			if err := run(); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
}

func TestStandaloneDaemonWithRealPi(t *testing.T) {
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pi not installed")
	}
	binary, err := os.Executable()
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	listener.Close()
	root := t.TempDir()
	const token = "standalone-worker-secret-0123456789abcdef"
	file := filepath.Join(root, "token")
	require.NoError(t, os.WriteFile(file, []byte(token), 0600))
	cmd := exec.Command(binary, "-test.run", "^TestDaemonProcessHelper$", "--", "--daemon-helper", "--local-assignments", "--addr", address, "--pi", pi, "--token-file", file, "--work-root", filepath.Join(root, "scratch"))
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	base := "http://" + address
	request := func(method, path string, value any) (*http.Response, error) {
		var body io.Reader
		if value != nil {
			data, _ := json.Marshal(value)
			body = bytes.NewReader(data)
		}
		r, err := http.NewRequest(method, base+path, body)
		if err != nil {
			return nil, err
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(agentexec.VersionHeader, agentexec.ProtocolVersion)
		return (&http.Client{Timeout: 25 * time.Second}).Do(r)
	}
	require.Eventually(t, func() bool {
		resp, err := request("GET", "/v1/ready", nil)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == 200
	}, 10*time.Second, 20*time.Millisecond)
	config := agentexec.StartConfig{Identity: agentexec.Identity{ExecutionID: "generated-at-core", SessionID: "conversation-at-core"}, Provider: "anthropic", APIKey: "dummy-assigned-key", ToolToken: "artifact-scoped-only", APIBaseURL: "http://core.invalid", SystemPrompt: "test", ExpiresAt: time.Now().Add(time.Hour)}
	resp, err := request("POST", "/v1/executions/generated-at-core/start", config)
	require.NoError(t, err)
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, string(data))
	resp, err = request("GET", "/v1/executions/generated-at-core/events?after=0", nil)
	require.NoError(t, err)
	var batch agentexec.EventBatch
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&batch))
	resp.Body.Close()
	assert.True(t, batch.Idle)
	assert.NotEmpty(t, batch.Events)
	resp, err = request("POST", "/v1/executions/generated-at-core/close", map[string]string{"id": "close-1"})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	resp, err = request("POST", "/v1/executions/generated-at-core/release", map[string]string{"id": "release-1"})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	require.NoError(t, cmd.Wait())
	waited = true
	entries, err := os.ReadDir(filepath.Join(root, "scratch"))
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), "execution-"), "daemon drained and removed execution scratch")
	}
}
