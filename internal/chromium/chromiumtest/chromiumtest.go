// Package chromiumtest starts a real headless Chromium for tests of the code
// that drives one. The browser is an optional sidecar and not a build
// dependency, so a test that needs one is skipped when none is installed.
package chromiumtest

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Start launches a headless Chromium with a DevTools port, stopped when the
// test ends, and returns its websocket address. The binary comes from
// CHROMIUM_TEST_BINARY, else a Playwright headless shell if one is installed.
// flags are added to the command line.
//
// Local Network Access is off. It refuses some requests to loopback, where
// every test server is, and it is Chromium's protection rather than ours: an
// operator's browser may not apply it, so code under test must stand without it.
func Start(t *testing.T, flags ...string) string {
	t.Helper()
	bin := os.Getenv("CHROMIUM_TEST_BINARY")
	if bin == "" {
		home, _ := os.UserHomeDir()
		matches, _ := filepath.Glob(filepath.Join(home, ".cache/ms-playwright/chromium_headless_shell-*/*/chrome-headless-shell"))
		if len(matches) > 0 {
			bin = matches[len(matches)-1]
		}
	}
	if bin == "" {
		t.Skip("no Chromium: set CHROMIUM_TEST_BINARY to run the browser tests")
	}
	ctx, cancel := context.WithCancel(context.Background())
	args := append([]string{"--headless", "--no-sandbox", "--disable-gpu",
		"--disable-features=LocalNetworkAccessChecks",
		"--remote-debugging-port=0", "--user-data-dir=" + t.TempDir()}, flags...)
	cmd := exec.CommandContext(ctx, bin, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })

	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if _, ws, ok := strings.Cut(sc.Text(), "DevTools listening on "); ok {
				found <- strings.TrimSpace(ws)
			}
		}
	}()
	select {
	case ws := <-found:
		return ws
	case <-time.After(15 * time.Second):
		t.Fatal("Chromium did not report a DevTools endpoint")
		return ""
	}
}
