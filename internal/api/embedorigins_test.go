package api

import (
	"reflect"
	"strings"
	"testing"
)

// Unset is the default every existing instance runs, and it must produce
// nothing at all — not an empty-string entry, which would reach the CSP as a
// stray space in frame-ancestors.
func TestEmbedOriginsFromEnvUnset(t *testing.T) {
	for _, raw := range []string{"", "   ", ",", " , ,, "} {
		t.Setenv(envEmbedOrigins, raw)
		got, err := EmbedOriginsFromEnv()
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", raw, err)
		}
		if len(got) != 0 {
			t.Fatalf("%q must configure no embed origins, got %#v", raw, got)
		}
	}
}

func TestEmbedOriginsFromEnvParsesLists(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"one origin", "https://landing.example", []string{"https://landing.example"}},
		{
			"comma separated",
			"https://landing.example,https://docs.example",
			[]string{"https://landing.example", "https://docs.example"},
		},
		{
			// An operator who copies the space-separated form straight out of
			// a CSP header gets the same result as one who writes commas.
			"space separated",
			"https://landing.example https://docs.example",
			[]string{"https://landing.example", "https://docs.example"},
		},
		{
			"commas, spaces and newlines together",
			"https://landing.example, https://docs.example\n  https://blog.example",
			[]string{"https://landing.example", "https://docs.example", "https://blog.example"},
		},
		{
			// Normalized by the same rule the per-artifact allowlist uses, so
			// the value pasted into frame-ancestors is the canonical spelling
			// rather than whatever the operator typed.
			"normalized",
			"HTTPS://Landing.Example.:443",
			[]string{"https://landing.example"},
		},
		{
			// Duplicates in different spellings are one decision, not two
			// sources in the emitted header.
			"deduplicated, order preserved",
			"https://landing.example, https://docs.example, https://LANDING.example",
			[]string{"https://landing.example", "https://docs.example"},
		},
		{
			// Loopback over plaintext is how a landing page under local
			// development frames a share; everything else must be https.
			"loopback http",
			"http://localhost:3000",
			[]string{"http://localhost:3000"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envEmbedOrigins, tt.raw)
			got, err := EmbedOriginsFromEnv()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

// A value that is not an origin is an error the caller makes fatal, never an
// entry quietly dropped: dropping it leaves the operator staring at a broken
// frame on a site they believe they configured, with nothing naming the typo.
// The error names the variable and the offending value, because those are the
// two things needed to fix it.
func TestEmbedOriginsFromEnvRejectsNonOrigins(t *testing.T) {
	for _, raw := range []string{
		"landing.example",                   // no scheme
		"https://landing.example/embed",     // a path is path-matched by CSP
		"https://*.example.com",             // a wildcard is not a list of origins
		"http://landing.example",            // plaintext, and not loopback
		"'self'",                            // a CSP keyword the builder owns
		"https://ok.example, not-an-origin", // one bad entry fails the list
	} {
		t.Setenv(envEmbedOrigins, raw)
		got, err := EmbedOriginsFromEnv()
		if err == nil {
			t.Fatalf("%q must be refused, got %#v", raw, got)
		}
		if got != nil {
			t.Fatalf("%q must configure nothing when refused, got %#v", raw, got)
		}
		if !strings.Contains(err.Error(), envEmbedOrigins) {
			t.Fatalf("error must name the variable so the operator knows where to look: %v", err)
		}
	}
}
