package render

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrivateFrameRequestsRequireBrowserEvidenceOfTheTrustedParent(t *testing.T) {
	for _, test := range []struct {
		name, app, destination, referrer string
		allowed                          bool
	}{
		{"secure iframe", "https://app.test", "iframe", "", true},
		{"secure direct", "https://app.test", "document", "https://app.test/", false},
		{"secure missing metadata", "https://app.test", "", "https://app.test/", false},
		{"http trusted app", "http://t3code:18080", "", "http://t3code:18080/artifacts/app", true},
		{"http missing referrer", "http://t3code:18080", "", "", false},
		{"http opaque render", "http://t3code:18080", "", "http://t3code:18081/", false},
		{"http hostile origin", "http://t3code:18080", "", "http://t3code.attacker.test:18080/", false},
		{"http credentialed referrer", "http://t3code:18080", "", "http://user@t3code:18080/", false},
		{"http direct remains refused", "http://t3code:18080", "document", "http://t3code:18080/", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/a/app", nil)
			r.Header.Set("Sec-Fetch-Dest", test.destination)
			r.Header.Set("Referer", test.referrer)
			assert.Equal(t, test.allowed, privateFrameRequest(r, test.app))
		})
	}
}
