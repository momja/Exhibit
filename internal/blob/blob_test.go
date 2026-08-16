package blob

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-7jcq. Store gained Delete so that removing an artifact removes its bytes.
// These pin the two halves of the contract stated on the interface: the file
// really leaves the disk, and a missing id is success rather than an error a
// caller has to recognise and forgive.

func TestFSStoreDeleteRemovesTheFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFSStore(dir)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "body-1", strings.NewReader("<html>secret</html>")))
	path := filepath.Join(dir, "body-1")
	require.FileExists(t, path)

	require.NoError(t, s.Delete(ctx, "body-1"))

	// The filesystem, not Get: the claim is that the bytes are gone, and only
	// the disk can answer that.
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "the blob file must be gone, got %v", statErr)

	_, err = s.Get(ctx, "body-1")
	assert.Error(t, err, "a deleted blob must no longer be readable")
}

func TestFSStoreDeleteIsIdempotent(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()

	// An id that was never stored. Store.Delete promises this is success, so
	// that an S3 backend (whose DeleteObject says the same) needs no
	// compensating HEAD, and so that no caller carries a branch meaning "fine".
	assert.NoError(t, s.Delete(ctx, "never-existed"))

	require.NoError(t, s.Put(ctx, "body-2", strings.NewReader("x")))
	require.NoError(t, s.Delete(ctx, "body-2"))
	// And a second delete of the same id — the shape a retry after a partial
	// failure takes.
	assert.NoError(t, s.Delete(ctx, "body-2"))
}

// The other two methods still work either side of a delete, so Delete is not
// quietly breaking the directory it operates in.
func TestFSStoreDeleteLeavesOtherBlobsAlone(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "keep", strings.NewReader("kept")))
	require.NoError(t, s.Put(ctx, "drop", strings.NewReader("dropped")))
	require.NoError(t, s.Delete(ctx, "drop"))

	rc, err := s.Get(ctx, "keep")
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "kept", string(got))
}
