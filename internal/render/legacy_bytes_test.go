package render

import (
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Goldens were measured by substituting serveDoc verbatim from 99452599,
// before running this fixture, and re-measured when every render gained the
// window.exhibit.ai stand-in and the framed AI bridge (Exh-zue3). Only the randomly minted public link is normalized.
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
	want := map[string]string{"artifact-and-direct-open": "e76985eed05003624afcb1a0ffdca713e2838162e2ed94c02bd70f23cb3466af", "widget": "d8ae7c668c86b0f5f0de9063347ef8e1cd7d430825625867edbfdbbdee7d084c", "share-widget": "336a1672eb0029421354c517839d5ef5b9c093c11307168f1d044e84252aa7c1", "share": "56acf56a220747f093839a3d5229c13073fcef8755325be9424bfe7c4b07f9bc", "version": "5af46df8105df5f9ddbebe284a82a7efaf275a2e409211317ec4857f07a5ed62"}
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
