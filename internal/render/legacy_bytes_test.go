package render

import (
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Goldens retain the serveDoc output from 99452599 with the refreshed artifact
// favicon. Only the randomly minted public link is normalized.
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
		"artifact-and-direct-open": "b80d692af4b0d5a939bf243b6f3c528bd694ff1014b11b4f284c06466f636dff",
		"widget":                   "c54d487793fb8cc7e8ede1a0e072801b4916f5693a93103fd9cf28b56a29a2fa",
		"share-widget":             "d4b47afa2d80d41b3bda68b96e811e789898e24dc7a2be6bcade842e7606d936",
		"share":                    "f368b0b367b0e47980e27fa5484186efb56f5e6a8138eb5af864f7b1c25658e1",
		"version":                  "4e46950c4a721bdff60e1c02754adaebc75753f6a71418bb04d4ba9471412df6",
	}
	for name, response := range cases {
		bytes := strings.ReplaceAll(response.Body.String(), link, "PUBLIC-LINK") + "\n" + fmt.Sprint(response.Header())
		got := fmt.Sprintf("%x", sha256.Sum256([]byte(bytes)))
		if len(want) == 0 {
			t.Logf("GOLDEN %s %s", name, got)
		} else {
			require.Equal(t, want[name], got, name)
		}
	}
}
