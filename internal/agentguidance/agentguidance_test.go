package agentguidance

import "testing"

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
