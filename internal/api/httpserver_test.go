package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bare ListenAndServe set none of these (av-ombn). WriteTimeout stays zero on
// purpose (NewServer's comment says why), so it is pinned too, to keep a
// well-meant "set all the timeouts" change from cutting every SSE stream.
func TestNewServerBoundsTheConnection(t *testing.T) {
	srv := NewServer(":0", http.NotFoundHandler())
	assert.Positive(t, srv.ReadHeaderTimeout)
	assert.Positive(t, srv.ReadTimeout)
	assert.Positive(t, srv.IdleTimeout)
	assert.Positive(t, srv.MaxHeaderBytes)
	assert.Less(t, srv.MaxHeaderBytes, http.DefaultMaxHeaderBytes, "explicitly tighter than net/http's default")
	assert.Zero(t, srv.WriteTimeout, "a write deadline would cut the agent's SSE stream")
}

// Over a real socket, with the client still sending: what a browser or the
// agent's tools actually see is the 413 and its JSON, not a reset connection.
// The httptest-level tests prove the body is not read; this proves the
// refusal survives being sent while the rest of the upload is still on the
// wire.
func TestAnOversizedUploadGetsA413OverARealConnection(t *testing.T) {
	ro := bodyLimitRouter(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := NewServer(ln.Addr().String(), ro)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	upload := bytes.Repeat([]byte("a"), 1<<20)
	req, err := http.NewRequest(http.MethodPost, "http://"+ln.Addr().String()+"/api/artifacts", bytes.NewReader(upload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var refusal bodyTooLargeResponse
	require.NoError(t, json.Unmarshal(raw, &refusal))
	assert.Equal(t, int64(testBodyLimit), refusal.LimitBytes)
}
