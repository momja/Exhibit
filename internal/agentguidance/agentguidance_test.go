package agentguidance

import (
	"strings"
	"testing"
	"text/template"
)

type kind string

func TestParseAIKinds(t *testing.T) {
	for csv, want := range map[string]AI{
		"":                    {},
		" , ":                 {},
		"complete":            {Complete: true},
		"decide":              {Decide: true},
		" complete , decide ": {Complete: true, Decide: true},
	} {
		kinds, err := ParseAIKinds(csv)
		if err != nil {
			t.Fatalf("%q: %v", csv, err)
		}
		got, err := AIFor(kinds)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v; want %+v", csv, got, err, want)
		}
	}
	for _, csv := range []string{"embed", "complete,Decide", "complete;decide"} {
		if _, err := ParseAIKinds(csv); err == nil {
			t.Errorf("%q: accepted", csv)
		}
	}
}

func TestKindNamesConvertsTypedKinds(t *testing.T) {
	names := KindNames([]kind{"complete", "decide"})
	if len(names) != 2 || names[0] != KindComplete || names[1] != KindDecide {
		t.Fatalf("got %v", names)
	}
	if KindNames([]kind(nil)) != nil {
		t.Fatal("empty input must give nil")
	}
}

// barOptInNames are the names the artifact bar's render preamble reads and
// sets (internal/render barScript, Exh-i8q4). The guidance teaches the same
// strings, and render_test.go asserts them in the served preamble, so prompt
// and implementation cannot drift apart. The Pi prompt and the MCP
// instructions pin them in their own renderings (internal/agent,
// internal/mcpserver).
var barOptInNames = []string{
	"exhibit-bar",
	`<meta name="exhibit-bar" content="overlay">`,
	"var(--exhibit-bar, 0px)",
}

func TestArtifactFormatTeachesTheBarOptIn(t *testing.T) {
	tmpl := template.Must(template.ParseFS(FS, "artifact_format.tmpl"))
	var out strings.Builder
	if err := tmpl.Execute(&out, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range barOptInNames {
		if !strings.Contains(out.String(), name) {
			t.Errorf("artifact_format.tmpl is missing %q", name)
		}
	}
}
