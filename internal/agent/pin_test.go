package agent

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestPinnedPiMatchesTheOneTested fails when the pi on this machine is not
// the version the Dockerfile ships. The guardrail (av-gust) depends on Pi's
// extension API, and the pipeline tests run against whatever `pi` is on PATH,
// so a green suite on a different version proves nothing about the image.
// That is exactly how av-gust first shipped: tested on 0.99.1, pinned to
// 0.87.1, which has no classifier API, so every prompt failed closed.
func TestPinnedPiMatchesTheOneTested(t *testing.T) {
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pi not installed; the pipeline tests skip too")
	}
	dockerfile, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^ARG PI_VERSION=(\S+)$`).FindSubmatch(dockerfile)
	if m == nil {
		t.Fatal("Dockerfile has no ARG PI_VERSION line; Pi must be pinned")
	}
	pinned := string(m[1])
	out, err := exec.Command(pi, "--version").Output()
	if err != nil {
		t.Fatalf("pi --version: %v", err)
	}
	local := strings.TrimSpace(string(out))
	if local != pinned {
		t.Fatalf("local pi is %s but the Dockerfile pins %s: install the pinned version (npm install -g @earendil-works/pi-coding-agent@%s) or bump the pin", local, pinned, pinned)
	}
}
