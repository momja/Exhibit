package agent

import (
	"strings"
	"testing"
)

// A widget-only session (av-fafu — the edit page's "Generate widget" button)
// must be scoped to set_widget and nothing else. In particular it must NOT
// inherit the ordinary edit-an-artifact paragraph, which instructs the model to
// save with update_artifact — the one thing this session must never do, since
// the artifact's own source is not what the user asked to change.
func TestWidgetOnlySessionIsScopedToTheWidget(t *testing.T) {
	prompt := sessionSystemPrompt("", CreateOpts{
		WidgetOnly:    true,
		ArtifactID:    "abc",
		ArtifactTitle: "Run Log",
	})

	if !strings.Contains(prompt, "set_widget") {
		t.Fatalf("widget-only prompt must direct the model to set_widget:\n%s", prompt)
	}
	if !strings.Contains(prompt, `artifact id "abc" titled "Run Log"`) {
		t.Fatalf("widget-only prompt must name its artifact:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Do NOT call create_artifact or update_artifact") {
		t.Fatalf("widget-only prompt must forbid rewriting the artifact:\n%s", prompt)
	}
	// The edit-mode paragraph tells the model to "save with update_artifact".
	// Both paragraphs at once would be a direct contradiction.
	if strings.Contains(prompt, "save with update_artifact") {
		t.Fatalf("widget-only prompt must not also carry the edit-artifact instruction:\n%s", prompt)
	}
}

// The ordinary modify-an-artifact session is unchanged by the widget case.
func TestEditSessionKeepsItsInstruction(t *testing.T) {
	prompt := sessionSystemPrompt("", CreateOpts{ArtifactID: "abc", ArtifactTitle: "Run Log"})

	if !strings.Contains(prompt, "save with update_artifact (never create_artifact)") {
		t.Fatalf("edit session lost its instruction:\n%s", prompt)
	}
	if strings.Contains(prompt, "exactly one job") {
		t.Fatalf("edit session must not get the widget-only scoping:\n%s", prompt)
	}
}

// A fresh create session gets the base prompt with no scoping paragraph, and
// the base still carries the widget contract so an agent building a new tool
// gives it a tile without being told twice.
func TestCreateSessionGetsBasePromptOnly(t *testing.T) {
	prompt := sessionSystemPrompt("", CreateOpts{})

	if strings.Contains(prompt, "This session") {
		t.Fatalf("a create session must carry no scoping paragraph:\n%s", prompt)
	}
	if !strings.Contains(prompt, "WIDGETS.") {
		t.Fatalf("base prompt must still carry the widget contract:\n%s", prompt)
	}
}

// A configured override replaces the base but still receives the scoping.
func TestSystemPromptOverrideIsHonored(t *testing.T) {
	prompt := sessionSystemPrompt("CUSTOM BASE", CreateOpts{WidgetOnly: true, ArtifactID: "x"})

	if !strings.HasPrefix(prompt, "CUSTOM BASE") {
		t.Fatalf("override must replace the default base:\n%s", prompt)
	}
	if !strings.Contains(prompt, "set_widget") {
		t.Fatalf("override must still get the widget scoping:\n%s", prompt)
	}
}
