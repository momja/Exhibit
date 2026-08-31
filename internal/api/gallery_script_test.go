package api

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGalleryPageScriptSuite runs web/gallery's own tests as part of `go test
// ./...`, so the client half of a feature is verified by the same command that
// verifies the server half (av-kmwj).
//
// The gallery's page scripts are plain files served verbatim, with no bundler
// and no module system, so the only way to exercise one is to load it into a
// small DOM (web/gallery/testdom.mjs) and drive it. The Go tests in this
// package can assert that an id is in the rendered markup and that a substring
// is in the shipped asset; they cannot assert that clicking Allow writes a
// decision and reloads the frame. That gap is what let a whole feature reach a
// merged PR and never reach main.
//
// Node is already a build-time dependency — `make build` cannot produce the
// embedded assets these tests read without it — so this adds no new class of
// requirement. It is skipped rather than failed when node is genuinely absent,
// since a machine with no node has no built assets either and would already be
// failing the tests around this one.
func TestGalleryPageScriptSuite(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; gallery page-script tests need it (so does `make assets`)")
	}
	dir, err := filepath.Abs(filepath.Join("..", "..", "web", "gallery"))
	if err != nil {
		t.Fatal(err)
	}
	// Named explicitly rather than by a --test directory walk, so a file added
	// with a typo'd name is a missing test rather than a silent no-op.
	suites := []string{"detail.net.test.mjs", "edit.origins.test.mjs", "agent.net.test.mjs"}
	cmd := exec.Command(node, append([]string{"--test"}, suites...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("web/gallery page-script tests failed: %v\n%s", err, out)
	}
	// `node --test` exits 0 when it matched no files at all, which would make
	// this a test that passes by running nothing.
	if !strings.Contains(string(out), "# pass ") && !strings.Contains(string(out), "pass ") {
		t.Fatalf("no page-script tests appear to have run:\n%s", out)
	}
}
