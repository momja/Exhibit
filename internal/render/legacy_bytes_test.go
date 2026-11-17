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
// before running this fixture. Only the randomly minted public link is normalized.
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
	want := map[string]string{"artifact-and-direct-open": "0ac8161dc2796bd644fa5415ab86df5149e55495e226ad2762b750f527e4bf14", "widget": "c54d487793fb8cc7e8ede1a0e072801b4916f5693a93103fd9cf28b56a29a2fa", "share-widget": "d4b47afa2d80d41b3bda68b96e811e789898e24dc7a2be6bcade842e7606d936", "share": "04e3480bbf1efaefa19583a24e4f2689fc2ec1a315e6e832c2c814aa931b3cd9", "version": "bbe7889c0101c2e3627c1386e98de8eca8c6a50340c62c2d4af6cb9993cb240e"}
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
