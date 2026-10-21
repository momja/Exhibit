package main

import (
	"context"
	"debug/elf"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run against the real archive emitted by package-agent.sh + package-release.sh:
// EXHIBIT_AGENT_RELEASE_ARCHIVE=... EXHIBIT_AGENT_RELEASE_VERSION=... go test ...
// Network/package installation stays in the explicit release build, not tests.
func TestPackagedReleaseMatchesSpriteBootstrap(t *testing.T) {
	archive := os.Getenv("EXHIBIT_AGENT_RELEASE_ARCHIVE")
	if archive == "" {
		t.Skip("set EXHIBIT_AGENT_RELEASE_ARCHIVE to test a built runtime archive")
	}
	expected := os.Getenv("EXHIBIT_AGENT_RELEASE_VERSION")
	require.NotEmpty(t, expected)
	root := t.TempDir()
	output, err := exec.Command("tar", "-xzf", archive, "--no-same-owner", "-C", root).CombinedOutput()
	require.NoError(t, err, "%s", output)
	manifest, err := os.ReadFile(filepath.Join(root, "EXHIBIT_RELEASE"))
	require.NoError(t, err)
	assert.Equal(t, expected, strings.TrimSpace(string(manifest)))
	for _, name := range []string{"exhibit-agent", "node", "pi"} {
		path := filepath.Join(root, "bin", name)
		target, err := filepath.EvalSymlinks(path)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(target, root+string(os.PathSeparator)), "%s must resolve inside the extracted archive", name)
		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0111)
	}
	checksum := exec.Command("sha256sum", "--check", "checksums.sha256")
	checksum.Dir = root
	output, err = checksum.CombinedOutput()
	require.NoError(t, err, "%s", output)
	worker, err := elf.Open(filepath.Join(root, "bin", "exhibit-agent"))
	require.NoError(t, err)
	defer worker.Close()
	node, err := elf.Open(filepath.Join(root, "bin", "node"))
	require.NoError(t, err)
	defer node.Close()
	assert.Equal(t, worker.Machine, node.Machine, "bundled Node must match the worker target architecture")
	versions, err := os.ReadFile(filepath.Join(root, "versions"))
	require.NoError(t, err)
	for _, pin := range []string{"release=" + expected, "protocol=" + agentexec.ProtocolVersion, "pi=" + agentexec.PiVersion, "node=" + agentexec.NodeVersion} {
		assert.Contains(t, "\n"+string(versions), "\n"+pin+"\n")
	}
	machine := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[runtime.GOARCH]
	if worker.Machine != machine {
		t.Logf("verified cross-compiled archive layout/checksums for %s; execute version/startup checks on matching hardware", worker.Machine)
		return
	}
	environment := []string{"PATH=" + filepath.Join(root, "bin") + ":/usr/bin:/bin"}
	for _, tc := range []struct{ name, version string }{{"exhibit-agent", expected}, {"node", "v" + agentexec.NodeVersion}, {"pi", agentexec.PiVersion}} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "bin", tc.name), "--version")
		command.Env = environment
		output, err = command.CombinedOutput()
		cancel()
		require.NoError(t, err, "%s: %s", tc.name, output)
		assert.Equal(t, tc.version, strings.TrimSpace(string(output)))
	}
	// Start the extracted Go daemon with the bundled Node/Pi under the same
	// empty environment used by Sprite bootstrap, then assign/close/reclaim Pi.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	private := t.TempDir()
	tokenFile := filepath.Join(private, "token")
	const token = "packaged-worker-authentication-0123456789abcdef"
	require.NoError(t, os.WriteFile(tokenFile, []byte(token), 0600))
	daemon := exec.Command(filepath.Join(root, "bin", "exhibit-agent"), "--addr", address, "--pi", filepath.Join(root, "bin", "pi"), "--local-assignments", "--token-file", tokenFile, "--work-root", filepath.Join(private, "scratch"), "--sprite-holds", "off")
	daemon.Env = environment
	daemon.Stderr = os.Stderr
	require.NoError(t, daemon.Start())
	waited := false
	defer func() {
		if !waited {
			_ = daemon.Process.Kill()
			_ = daemon.Wait()
		}
	}()
	client := &http.Client{Timeout: time.Second}
	base := "http://" + address
	require.Eventually(t, func() bool {
		request, _ := http.NewRequest(http.MethodGet, base+"/v1/ready", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set(agentexec.VersionHeader, agentexec.ProtocolVersion)
		response, err := client.Do(request)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, 10*time.Second, 20*time.Millisecond)
	process, err := (agentexec.Remote{Endpoint: agentexec.Endpoint{URL: base, Token: token, AllowUnassigned: true}}).Start(context.Background(), agentexec.StartConfig{Identity: agentexec.Identity{ExecutionID: "archive-attempt", SessionID: "archive-chat"}, Provider: "anthropic", APIKey: "assigned-dummy-key", ToolToken: "assigned-tool-token", APIBaseURL: "http://core.invalid", SystemPrompt: "Readiness only", ExpiresAt: time.Now().Add(time.Minute)})
	require.NoError(t, err)
	go io.Copy(io.Discard, process.Stdout())
	require.NoError(t, process.Cleanup())
	require.NoError(t, daemon.Process.Signal(syscall.SIGTERM))
	require.NoError(t, daemon.Wait())
	waited = true
	entries, err := os.ReadDir(filepath.Join(private, "scratch"))
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), "execution-"))
	}
}
