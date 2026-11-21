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
		"artifact-and-direct-open": "b2b72d2efad44acd73e0ff3850a92fa2f44fdbd5b5c6097dc5df6def0ad8c0f4",
		"widget":                   "f6c61534b9c32d02bc90adeb38a3c9402ba95e06f1eb60ea9cd3211604280893",
		"share-widget":             "f073709dfcc88b981565a63fd47a10604f6d5a4bfe2d6e88536428cf48c2ea36",
		"share":                    "d00e59857e52c7187c7d212c7537d882e9fb2ccd37f79e1d17acbb50632337c8",
		"version":                  "ce109b29fb9d87b8ba4e55763c8eae6910f5e24ce7ff7c8777a97c11e3c6a2e8",
	}
	for name, response := range cases {
		body := artifactChannelPattern.ReplaceAllString(response.Body.String(), `var ARTIFACT_CHANNEL = "CHANNEL";`)
		bytes := strings.ReplaceAll(body, link, "PUBLIC-LINK") + "\n" + fmt.Sprint(response.Header())
		got := fmt.Sprintf("%x", sha256.Sum256([]byte(bytes)))
		assert.Equal(t, want[name], got, name)
	}
}
