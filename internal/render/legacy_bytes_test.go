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
		"artifact-and-direct-open": "5bd7ac344aac24c42227d85445b2e904f1b063d39d863de3bdcf1e4f04013815",
		"widget":                   "f6c61534b9c32d02bc90adeb38a3c9402ba95e06f1eb60ea9cd3211604280893",
		"share-widget":             "f073709dfcc88b981565a63fd47a10604f6d5a4bfe2d6e88536428cf48c2ea36",
		"share":                    "8a66190e1b669c866d8e5c4f2048cae48830a2e3551a15fc7f93c5d6ddf96d9c",
		"version":                  "326ba575f82c012d5b5072027619b26fa26873b3d0e7a34ed3e41b4ba243c55f",
	}
	for name, response := range cases {
		body := artifactChannelPattern.ReplaceAllString(response.Body.String(), `var ARTIFACT_CHANNEL = "CHANNEL";`)
		bytes := strings.ReplaceAll(body, link, "PUBLIC-LINK") + "\n" + fmt.Sprint(response.Header())
		got := fmt.Sprintf("%x", sha256.Sum256([]byte(bytes)))
		assert.Equal(t, want[name], got, name)
	}
}
