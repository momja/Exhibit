package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/stretchr/testify/require"
)

func TestStaticCleanupRequiresWorkerProtocol(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		status  int
		wantErr bool
	}{
		{"proxy_not_found", "", http.StatusNotFound, true},
		{"proxy_success", "", http.StatusOK, true},
		{"incompatible_worker", "next", http.StatusNotFound, true},
		{"worker_absent", agentexec.ProtocolVersion, http.StatusNotFound, false},
		{"worker_released", agentexec.ProtocolVersion, http.StatusOK, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer cleanup-fixture", r.Header.Get("Authorization"))
				w.Header().Set(agentexec.VersionHeader, test.version)
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			coordinator := &workerCoordinator{}
			err := coordinator.cleanupStatic(context.Background(), workerRecord{StaticURL: server.URL, StaticToken: "cleanup-fixture"})
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
