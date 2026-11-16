package main

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFlySupervisorStopsSiblingOnExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sibling := exec.Command("sleep", "60")
	err := superviseFly(ctx, sibling, exec.Command("sh", "-c", "exit 7"))
	require.ErrorContains(t, err, "Fly child exited")
	require.NotNil(t, sibling.ProcessState)
	require.False(t, sibling.ProcessState.Success())
}

func TestFlySupervisorStopsChildrenOnSignal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	child := exec.Command("sleep", "60")
	require.NoError(t, superviseFly(ctx, child))
	require.NotNil(t, child.ProcessState)
}

func TestFlySupervisorCleansUpOnStartFailure(t *testing.T) {
	child := exec.Command("sleep", "60")
	require.Error(t, superviseFly(context.Background(), child, exec.Command("/nonexistent-exhibit-binary")))
	require.NotNil(t, child.ProcessState)
}

func TestFlyRejectsConfigurationDrift(t *testing.T) {
	for _, key := range []string{"SINGLE_LISTENER", "ADDR", "MCP_PROXY_URL", "APP_ORIGIN", "EXHIBIT_PUBLIC_URL", "MCP_URL", "MCP_PUBLIC_URL"} {
		t.Run(key, func(t *testing.T) {
			for name, value := range map[string]string{
				"SINGLE_LISTENER": "1", "ADDR": ":8080", "MCP_PROXY_URL": "http://127.0.0.1:8090",
				"APP_ORIGIN": "https://exhibit.dizzard.net", "EXHIBIT_PUBLIC_URL": "https://exhibit.dizzard.net",
				"MCP_URL": "https://exhibit.dizzard.net/mcp", "MCP_PUBLIC_URL": "https://exhibit.dizzard.net/mcp",
			} {
				t.Setenv(name, value)
			}
			t.Setenv(key, "")
			require.Error(t, serveFly(), "invalid config must be refused before starting either child")
		})
	}
}
