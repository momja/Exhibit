package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetiredConfigurationCannotOpenLibrary(t *testing.T) {
	// Old deployment environments must not restore the anonymous read bypass.
	for _, value := range []string{"", "true", "false"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PUBLIC_"+"MODE_ENABLED", value)
			for _, localLogin := range []bool{false, true} {
				ro := newTestRouter(t)
				ro.cfg.LocalUsers = localLogin
				if localLogin {
					ro.cfg.AuthToken = ""
				}
				w := httptest.NewRecorder()
				ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/artifacts", nil))
				assert.Equal(t, http.StatusUnauthorized, w.Code)
			}
		})
	}
}

func TestRetiredSettingsRouteIsAbsent(t *testing.T) {
	for _, login := range []bool{false, true} {
		ro := newTestRouter(t)
		ro.cfg.LocalUsers = login
		for _, token := range []string{"", ro.cfg.AuthToken} {
			req := httptest.NewRequest(http.MethodGet, "/api/settings/"+"public", nil)
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			ro.ServeHTTP(w, req)
			assert.Equal(t, http.StatusNotFound, w.Code)
		}
	}
}
