package api

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginReturnDoesNotRetainPaddedQuery(t *testing.T) {
	ro, _ := newLoginTestRouter(t, &stubProvider{loginURL: "https://idp.test/authorize"}, nil)
	req := httptest.NewRequest(http.MethodGet, "/auth/sso?next=/&padding="+strings.Repeat("x", 32<<10), nil)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code)
	require.Len(t, ro.loginReturns.pending, 1)
	query := req.URL.RawQuery
	start := uintptr(unsafe.Pointer(unsafe.StringData(query)))
	accounted := 0
	for _, pending := range ro.loginReturns.pending {
		assert.Equal(t, "/", pending.url)
		assertStringOutside(t, "login destination", pending.url, start, start+uintptr(len(query)))
		accounted += len(pending.url)
	}
	assert.Equal(t, 1, accounted, "the store should retain and account for only the destination byte")
	runtime.KeepAlive(req)
}
