package agentexec

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cleanupTransport func(*http.Request) (*http.Response, error)

func (f cleanupTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRemoteCleanupRetriesLostReleaseResponseAndEndpointFailure(t *testing.T) {
	for _, failure := range []string{"lost-release", "endpoint"} {
		t.Run(failure, func(t *testing.T) {
			service, err := NewService(ServiceConfig{LocalAssignments: true, Token: testWorkerToken, Runner: Local{PiBin: fakePi(t), WorkRoot: t.TempDir()}})
			require.NoError(t, err)
			server := httptest.NewServer(service)
			defer server.Close()
			defer service.Shutdown(context.Background())
			var releases, endpointCalls atomic.Int32
			transport := cleanupTransport(func(r *http.Request) (*http.Response, error) {
				response, err := http.DefaultTransport.RoundTrip(r)
				if strings.HasSuffix(r.URL.Path, "/release") && releases.Add(1) == 1 && failure == "lost-release" && err == nil {
					_ = response.Body.Close() // Service has reclaimed/deleted the assignment.
					return nil, errors.New("lost successful response")
				}
				return response, err
			})
			p, err := (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken, AllowUnassigned: true, Transport: transport, Release: func(context.Context) error {
				if endpointCalls.Add(1) == 1 && failure == "endpoint" {
					return errors.New("endpoint unavailable")
				}
				return nil
			}}}).Start(context.Background(), testConfig())
			require.NoError(t, err)
			collect(t, p)
			require.Error(t, p.Cleanup())
			if failure == "lost-release" {
				assert.Zero(t, endpointCalls.Load(), "endpoint release follows acknowledged worker reclamation")
			}
			require.NoError(t, p.Cleanup(), "same cleanup capability must retry its unfinished phase")
			require.NoError(t, p.Cleanup())
			if failure == "lost-release" {
				assert.Equal(t, int32(2), releases.Load())
				assert.Equal(t, int32(1), endpointCalls.Load())
			} else {
				assert.Equal(t, int32(1), releases.Load(), "acknowledged worker release must not repeat")
				assert.Equal(t, int32(2), endpointCalls.Load())
			}
			service.mu.Lock()
			assert.Empty(t, service.children)
			assert.True(t, service.retired[testConfig().ExecutionID])
			service.mu.Unlock()
		})
	}
}

func TestLocalCleanupRetriesFilesystemFailure(t *testing.T) {
	process, err := (Local{PiBin: fakePi(t), WorkRoot: t.TempDir()}).Start(context.Background(), testConfig())
	require.NoError(t, err)
	p := process.(*LocalProcess)
	collect(t, p)
	root := p.WorkDir
	defer os.RemoveAll(root)
	obstruction := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(obstruction, []byte("x"), 0600))
	p.WorkDir = filepath.Join(obstruction, "child")
	require.Error(t, p.Cleanup())
	require.Error(t, p.Cleanup(), "an unfinished cleanup cannot return a false success")
	p.WorkDir = root
	require.NoError(t, p.Cleanup())
	_, err = os.Stat(root)
	assert.True(t, os.IsNotExist(err))
}
