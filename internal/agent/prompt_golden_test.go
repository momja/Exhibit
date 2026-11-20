package agent

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updatePromptGoldens = flag.Bool("update-prompt-goldens", false, "rewrite testdata/prompts/*.golden")

// Keep the rendered prompt stable across template refactors, including the
// relay and connector guidance selected by instance capabilities.
func TestPromptsMatchGoldens(t *testing.T) {
	cases := map[string]string{
		"system-relay.golden":       buildSystemPrompt("", nil, CreateOpts{NetworkRelayEnabled: true}),
		"system-connections.golden": buildSystemPrompt("", nil, CreateOpts{NetworkRelayEnabled: true, ConnectionsEnabled: true}),
		"system-default.golden":     buildSystemPrompt("", nil, CreateOpts{}),
		// Inference guidance (Exh-15tf): "neither" is system-default above,
		// which must stay byte-identical to an instance without inference.
		"system-ai-complete.golden": buildSystemPrompt("", []string{"complete"}, CreateOpts{}),
		"system-ai-decide.golden":   buildSystemPrompt("", []string{"decide"}, CreateOpts{}),
		"system-ai-both.golden":     buildSystemPrompt("", []string{"complete", "decide"}, CreateOpts{}),
		"system-edit.golden":        buildSystemPrompt("", nil, CreateOpts{ArtifactID: "a"}),
		"system-widget-only.golden": buildSystemPrompt("", nil, CreateOpts{ArtifactID: "a", WidgetOnly: true}),
		"system-override.golden":    buildSystemPrompt("You are a haiku generator.", nil, CreateOpts{ArtifactID: "a"}),
		"selection-1.golden":        selectionNotice(1),
		"selection-3.golden":        selectionNotice(3),
		"resumed.golden":            resumedNotice,
		"generate-widget.golden":    GenerateWidgetPrompt,
	}
	for name, got := range cases {
		path := filepath.Join("testdata", "prompts", name)
		if *updatePromptGoldens {
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
			continue
		}
		want, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, string(want), got, name)
	}
}

// A WorkRoot left by an older binary may hold outdated prompt files; startup
// must overwrite them with the embedded bytes guard.ts is meant to read.
func TestMaterializeExtPromptsOverwritesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "guard_policy.tmpl")
	require.NoError(t, os.WriteFile(stale, []byte("outdated"), 0o644))

	require.NoError(t, materializeExtPrompts(dir))

	want, err := extFS.ReadFile("ext/prompts/guard_policy.tmpl")
	require.NoError(t, err)
	got, err := os.ReadFile(stale)
	require.NoError(t, err)
	require.Equal(t, string(want), string(got))
}
