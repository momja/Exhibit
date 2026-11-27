package mcpserver

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/agentguidance"
)

var updateInstructionGoldens = flag.Bool("update-instruction-goldens", false, "rewrite testdata/instructions-*.golden")

// The instructions teach exactly the window.exhibit.ai kinds the instance
// offers (Exh-15tf). "neither" is the text of an instance without inference
// and must stay byte-identical to what it was before inference existed.
func TestInstructionsMatchGoldens(t *testing.T) {
	cases := map[string]agentguidance.AI{
		"instructions-ai-none.golden":     {},
		"instructions-ai-complete.golden": {Complete: true},
		"instructions-ai-decide.golden":   {Decide: true},
		"instructions-ai-both.golden":     {Complete: true, Decide: true},
		// Exh-v81w: images extend complete() and change nothing else.
		"instructions-ai-complete-images.golden": {Complete: true, Images: true},
	}
	for name, ai := range cases {
		got := InstructionsFor(ai)
		path := filepath.Join("testdata", name)
		if *updateInstructionGoldens {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != got {
			t.Errorf("%s: instructions differ from golden; rerun with -update-instruction-goldens and review the diff", name)
		}
	}
	if Instructions != InstructionsFor(agentguidance.AI{}) {
		t.Error("Instructions must be the no-inference text")
	}
}

func TestInstructionsDescribeOnlyOfferedKinds(t *testing.T) {
	if off := InstructionsFor(agentguidance.AI{}); strings.Contains(off, "exhibit.ai") {
		t.Error("instructions without inference mention window.exhibit.ai")
	}
	if decide := InstructionsFor(agentguidance.AI{Decide: true}); strings.Contains(decide, "complete({") || !strings.Contains(decide, "decide({") {
		t.Error("decide-only instructions must teach decide() and not complete()")
	}
	complete := InstructionsFor(agentguidance.AI{Complete: true})
	if strings.Contains(complete, "decide({") || !strings.Contains(complete, "complete({") {
		t.Error("complete-only instructions must teach complete() and not decide()")
	}
	if strings.Contains(complete, "type: 'image'") {
		t.Error("instructions teach images on an instance that does not offer them")
	}
	if images := InstructionsFor(agentguidance.AI{Complete: true, Images: true}); !strings.Contains(images, "type: 'image'") || !strings.Contains(images, "canvas") {
		t.Error("image instructions must teach image parts and canvas downscaling")
	}
}

func TestHTTPHandlerRefusesUnknownAIKind(t *testing.T) {
	if _, err := NewHTTPHandler(HTTPConfig{BaseURL: "http://core.test", AIKinds: []string{"embed"}}); err == nil {
		t.Fatal("accepted an unknown AI kind")
	}
	if _, err := NewHTTPHandler(HTTPConfig{BaseURL: "http://core.test", AIKinds: []string{"complete", "decide"}}); err != nil {
		t.Fatal(err)
	}
}

// The artifact bar's overlay opt-in (Exh-i8q4) uses the names the render
// preamble reads and sets; render_test.go asserts the same strings there.
func TestInstructionsTeachTheBarOptIn(t *testing.T) {
	for _, name := range []string{"exhibit-bar", `<meta name="exhibit-bar" content="overlay">`, "var(--exhibit-bar, 0px)"} {
		if !strings.Contains(Instructions, name) {
			t.Errorf("instructions are missing %q", name)
		}
	}
}
