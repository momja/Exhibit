package render

import (
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Goldens were measured by substituting serveDoc verbatim from 99452599,
// before running this fixture, and re-measured when every render gained the
// window.exhibit.ai stand-in and the document channels (Exh-zue3). The random
// public link and the channel derived from the fixture's signing key are normalized.
// The artifact and version goldens were re-measured again when framed renders
// gained the artifact bar script (Exh-i8q4); widget and share bytes did not move.
// All five moved once more when the stand-in's available() gained images:
// false (Exh-v81w), and nothing else.
var artifactChannelPattern = regexp.MustCompile(`var ARTIFACT_CHANNEL = "[^"]*";`)

func TestLegacyDocumentBytes(t *testing.T) {
	rd, _, link, _ := newShareWidgetFixture(t)
	private := httptest.NewRecorder()
	rd.ServeArtifact(private, renderRequest("/a/abc", "abc", 1))
	vr, _ := newVersionedRenderer(t)
	cases := map[string]*httptest.ResponseRecorder{
		"artifact-and-direct-open": private,
		"widget":                   serveWidget(t, rd, "abc"),
		"share-widget":             serveShareWidgetRaw(t, rd, link),
		"share":                    serveShare(t, rd, link),
		"version":                  serveVersion(vr, "abc", 1, versionToken("abc", 1, 1)),
	}
	want := map[string]string{
		"artifact-and-direct-open": "bdf46121d82b901758640b7e6c2bfb8ff30e422d21023a05c8db6e6ff8c947e5",
		"widget":                   "677c972ac710039eeb45678f9774cebf04269747dd758028d82e07396cdea518",
		"share-widget":             "ecc1a8111194a9f0810efc476d43fd7b6101e4ada1319e143db121cba63d436f",
		"share":                    "c62e16f7ab7d549ca55a2c3c46251981303251ce01c9e8b4d7aacb1a677cde22",
		"version":                  "46a84ef2b4b194f86bf6320059b56fc3da55e320c18c7750b16ba1db062026d6",
	}
	for name, response := range cases {
		body := artifactChannelPattern.ReplaceAllString(response.Body.String(), `var ARTIFACT_CHANNEL = "CHANNEL";`)
		bytes := strings.ReplaceAll(body, link, "PUBLIC-LINK") + "\n" + fmt.Sprint(response.Header())
		got := fmt.Sprintf("%x", sha256.Sum256([]byte(bytes)))
		assert.Equal(t, want[name], got, name)
	}
}
