package export

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/momja/Exhibit/internal/inference/standin"
)

// The stand-in only helps if it runs before the artifact's first script, and
// only stays harmless if it adds nothing else: the rest of the document, AI
// call sites included, must come back byte for byte (Exh-i902).
func TestDisableAIPutsTheStandInFirstInTheHead(t *testing.T) {
	const call = `<script>window.exhibit.ai.complete({prompt:'hi'})</script>`
	cases := map[string]struct{ doc, before string }{
		"plain head":           {`<!doctype html><html><head>` + call + `</head><body></body></html>`, `<!doctype html><html><head>`},
		"head with attributes": {`<!DOCTYPE html><html lang="en"><head data-x=">">` + call + `</head></html>`, `<!DOCTYPE html><html lang="en"><head data-x=">">`},
		"no head element":      {"<!doctype html>\n<html>\n" + call + `<body></body></html>`, "<!doctype html>\n<html>"},
		"no html element":      {`<!doctype html>` + call + `<p>x</p>`, `<!doctype html>`},
		"bare fragment":        {call + `<p>x</p>`, ``},
		"head text in a comment": {
			`<!-- <head> --><!doctype html><html><head>` + call + `</head></html>`,
			`<!-- <head> --><!doctype html><html><head>`,
		},
		"header is not head": {`<!doctype html><header>` + call + `</header>`, `<!doctype html>`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out := DisableAI(tc.doc)
			assert.Equal(t, tc.before+standin.Tag+strings.TrimPrefix(tc.doc, tc.before), out)
			assert.Less(t, strings.Index(out, standin.Tag), strings.Index(out, call), "stand-in precedes the artifact's script")
			assert.Equal(t, tc.doc, strings.Replace(out, standin.Tag, "", 1), "nothing else changes")
		})
	}
}

// What the stand-in carries is what every export carries, so it is checked
// here for anything that would reach back to an instance.
func TestTheStandInCarriesNoEndpointOrConfiguration(t *testing.T) {
	for _, forbidden := range []string{"/inference", "INFERENCE_", "fetch(", "XMLHttpRequest", "postMessage", "sendBeacon", "WebSocket", "http:", "https:"} {
		assert.NotContains(t, standin.Tag, forbidden)
	}
}
