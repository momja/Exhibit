package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sidecarEnv starts one session against a fake pi that writes its environment
// into its HOME (the session workdir) and then idles, and returns what it saw.
func sidecarEnv(t *testing.T, cfg Config) []string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "fakepi.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nenv > \"$HOME/env.txt\"\nexec sleep 60\n"), 0o755))

	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	cfg.PiBin = script
	cfg.WorkRoot = t.TempDir()
	cfg.APIBaseURL = "http://app.test"
	cfg.Credentials = agentscope.NewRegistry()
	m, err := New(cfg, db)
	require.NoError(t, err)

	s, err := m.Create(context.Background(), CreateOpts{OwnerID: 1, Provider: "anthropic", APIKey: "k"})
	require.NoError(t, err)
	t.Cleanup(s.kill)

	var env []byte
	require.Eventually(t, func() bool {
		matches, _ := filepath.Glob(filepath.Join(cfg.WorkRoot, "*", "env.txt"))
		if len(matches) == 0 {
			return false
		}
		env, err = os.ReadFile(matches[0])
		return err == nil && len(env) > 0
	}, 5*time.Second, 20*time.Millisecond, "the fake pi never wrote its environment")
	return strings.Split(strings.TrimSpace(string(env)), "\n")
}

// The extension refuses an oversized write before sending it only if it knows
// the limit, and the service's environment is the one place it can learn it
// from (av-ombn).
func TestTheSidecarIsToldTheRequestBodyLimit(t *testing.T) {
	env := sidecarEnv(t, Config{MaxRequestBodyBytes: 32 << 20})
	assert.Contains(t, env, "EXHIBIT_MAX_BODY_BYTES=33554432")
}

// Zero passes nothing rather than a limit of zero, which the extension would
// read as "every write is too large".
func TestAnUnsetLimitIsNotPassedToTheSidecar(t *testing.T) {
	for _, kv := range sidecarEnv(t, Config{}) {
		assert.False(t, strings.HasPrefix(kv, "EXHIBIT_MAX_BODY_BYTES="), kv)
	}
}
