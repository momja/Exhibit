package agent

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtensionSuite runs the agent extension's own node tests as part of
// `go test ./...`, so the edit engine (av-f5i5) is verified by the same
// command that verifies the server half — the gallery page-script suite
// (internal/api/gallery_script_test.go) exists for exactly this reason.
//
// edit.test.ts exercises the pure matching/validation engine (ext/edit.ts,
// zero dependencies); exhibit.test.ts drives the edit_artifact tool wiring
// (ext/exhibit.ts) against stubbed pi + API fetch, with bare `typebox`
// resolved to a test double by --import of testdata/register-hooks.mjs.
//
// Node is already a build-time dependency (`make assets` needs it), so this
// adds no new class of requirement. Skipped when node is absent.
func TestExtensionSuite(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; agent extension tests need it (so does `make assets`)")
	}
	dir, err := filepath.Abs("ext")
	if err != nil {
		t.Fatal(err)
	}
	// Named explicitly rather than by a --test directory walk, so a file added
	// with a typo'd name is a missing test rather than a silent no-op.
	suites := []string{
		"edit.test.ts",
		"exhibit.test.ts",
	}
	args := append([]string{"--test", "--import", "./testdata/register-hooks.mjs"}, suites...)
	cmd := exec.Command(node, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent extension tests failed: %v\n%s", err, out)
	}
	// `node --test` exits 0 when it matched no files at all, which would make
	// this a test that passes by running nothing.
	if !strings.Contains(string(out), "# pass ") && !strings.Contains(string(out), "pass ") {
		t.Fatalf("no agent extension tests appear to have run:\n%s", out)
	}
}
