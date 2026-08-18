package blob_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/blob/blobtest"
)

// The Store contract, written once and run against every implementation
// (av-52ll). Until this ticket there was one backend and its tests could be
// about files; now there are two, and what has to hold is the *interface's*
// promises — the ones handlers rely on without knowing which store is behind
// them. A backend that passes this is substitutable; that is the entire claim
// the S3 implementation makes.
//
// FSStore's own filesystem-level tests stay in blob_test.go: they assert the
// file really leaves the disk, which is a fact about that backend and not
// something this suite can or should ask.

func runStoreContract(t *testing.T, open func(t *testing.T) blob.Store) {
	t.Run("RoundTrip", func(t *testing.T) {
		s := open(t)
		ctx := context.Background()
		body := "<html><body>round trip</body></html>"

		require.NoError(t, s.Put(ctx, "body-round-trip", strings.NewReader(body)))
		rc, err := s.Get(ctx, "body-round-trip")
		require.NoError(t, err)
		defer rc.Close()
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, body, string(got))
	})

	t.Run("PutOverwrites", func(t *testing.T) {
		// PATCH of an artifact body rewrites the blob under the same id
		// (artifacts.go), so an overwrite has to be a replacement and not an
		// append or a refusal.
		s := open(t)
		ctx := context.Background()
		require.NoError(t, s.Put(ctx, "body-overwrite", strings.NewReader("first version, longer")))
		require.NoError(t, s.Put(ctx, "body-overwrite", strings.NewReader("second")))

		rc, err := s.Get(ctx, "body-overwrite")
		require.NoError(t, err)
		defer rc.Close()
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, "second", string(got))
	})

	t.Run("GetMissingIsAnError", func(t *testing.T) {
		// And an error from Get, not from a read half-way through: the render
		// surface turns this into a 404 before it has written a 200.
		s := open(t)
		rc, err := s.Get(context.Background(), "never-stored")
		if err == nil {
			rc.Close()
			t.Fatal("Get of a missing blob must fail at Get, not at first Read")
		}
	})

	t.Run("DeleteRemovesTheBytes", func(t *testing.T) {
		s := open(t)
		ctx := context.Background()
		require.NoError(t, s.Put(ctx, "body-delete", strings.NewReader("secret")))
		require.NoError(t, s.Delete(ctx, "body-delete"))

		rc, err := s.Get(ctx, "body-delete")
		if err == nil {
			rc.Close()
			t.Fatal("a deleted blob must no longer be readable")
		}
	})

	t.Run("DeleteIsIdempotent", func(t *testing.T) {
		// The contract stated on Store.Delete, and the reason it is stated: an
		// object store's DeleteObject already answers success for a key that
		// was never there, so a backend must not add an existence check to
		// manufacture the failure.
		s := open(t)
		ctx := context.Background()
		assert.NoError(t, s.Delete(ctx, "never-existed"))

		require.NoError(t, s.Put(ctx, "body-twice", strings.NewReader("x")))
		require.NoError(t, s.Delete(ctx, "body-twice"))
		assert.NoError(t, s.Delete(ctx, "body-twice"), "a repeated delete is the shape a retry takes")
	})

	t.Run("DeleteLeavesOtherBlobsAlone", func(t *testing.T) {
		s := open(t)
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
	})

	t.Run("RejectsIDsThatAreNotFlatNames", func(t *testing.T) {
		// Ids are server-minted UUIDs, but the validation is what lets both
		// backends treat an id as a single name — a path segment on one, a key
		// suffix under a prefix on the other.
		s := open(t)
		ctx := context.Background()
		for _, id := range []string{"", "../escape", "nested/id", "back\\slash"} {
			assert.Error(t, s.Put(ctx, id, strings.NewReader("x")), "Put(%q)", id)
			_, err := s.Get(ctx, id)
			assert.Error(t, err, "Get(%q)", id)
			assert.Error(t, s.Delete(ctx, id), "Delete(%q)", id)
		}
	})

	t.Run("StreamsAPayloadLargerThanOnePart", func(t *testing.T) {
		// A snapshot that vendored a wasm payload is tens of megabytes, and
		// crossing the multipart threshold is where a backend's upload path
		// changes shape. 6 MiB is past the 5 MiB minimum part size while
		// staying cheap enough to run on every `go test`.
		s := open(t)
		ctx := context.Background()
		want := patternedBytes(6 << 20)

		// A plain io.Reader, so the store cannot learn the length from the
		// reader and has to take the path that discovers it.
		require.NoError(t, s.Put(ctx, "body-large", io.LimitReader(bytes.NewReader(want), int64(len(want)))))

		rc, err := s.Get(ctx, "body-large")
		require.NoError(t, err)
		defer rc.Close()
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.Len(t, got, len(want))
		assert.True(t, bytes.Equal(want, got), "the bytes read back must be the bytes written")
	})

	t.Run("StreamsAKnownLengthPayload", func(t *testing.T) {
		// The same payload through the other upload path — a reader that can
		// report what is left, which is what every caller above the interface
		// actually hands Put.
		s := open(t)
		ctx := context.Background()
		want := patternedBytes(6 << 20)

		require.NoError(t, s.Put(ctx, "body-large-sized", bytes.NewReader(want)))

		rc, err := s.Get(ctx, "body-large-sized")
		require.NoError(t, err)
		defer rc.Close()
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.Len(t, got, len(want))
		assert.True(t, bytes.Equal(want, got))
	})

	t.Run("PartiallyReadReaderStoresOnlyTheRemainder", func(t *testing.T) {
		// The size a backend infers must be what is *left* to read, not the
		// reader's original length. Getting that wrong is a truncated or
		// padded upload rather than a visible failure, so it is pinned here.
		s := open(t)
		ctx := context.Background()
		r := strings.NewReader("HEADERtail")
		_, err := io.CopyN(io.Discard, r, 6)
		require.NoError(t, err)

		require.NoError(t, s.Put(ctx, "body-partial", r))
		rc, err := s.Get(ctx, "body-partial")
		require.NoError(t, err)
		defer rc.Close()
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, "tail", string(got))
	})

	t.Run("PartiallyReadReaderLargerThanOnePartStoresOnlyTheRemainder", func(t *testing.T) {
		// The same rule as above, past the multipart threshold — which is
		// where it is genuinely dangerous rather than merely wrong. A
		// *bytes.Reader is an io.ReaderAt, and an object store SDK's fast path
		// for a large known-size ReaderAt addresses parts by absolute offset
		// from zero, ignoring the read position. A backend that hands it a
		// consumed reader plus the remaining length uploads the *first* n
		// bytes under the guise of the last n: right length, wrong content,
		// no error anywhere. Nothing above the interface would ever find out.
		s := open(t)
		ctx := context.Background()
		full := patternedBytes(7 << 20)
		const skip = 1 << 20

		r := bytes.NewReader(full)
		if _, err := io.CopyN(io.Discard, r, skip); err != nil {
			t.Fatal(err)
		}
		require.NoError(t, s.Put(ctx, "body-partial-large", r))

		rc, err := s.Get(ctx, "body-partial-large")
		require.NoError(t, err)
		defer rc.Close()
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.Len(t, got, len(full)-skip)
		assert.True(t, bytes.Equal(full[skip:], got),
			"the stored bytes must be the unread remainder, not the same length taken from the start")
	})

	t.Run("StoresAnEmptyBlob", func(t *testing.T) {
		// An artifact with an empty body is a real state the API permits, and
		// zero bytes is the length most likely to be confused with "unknown".
		s := open(t)
		ctx := context.Background()
		require.NoError(t, s.Put(ctx, "body-empty", strings.NewReader("")))

		rc, err := s.Get(ctx, "body-empty")
		require.NoError(t, err)
		defer rc.Close()
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestFSStoreContract(t *testing.T) {
	runStoreContract(t, func(t *testing.T) blob.Store {
		s, err := blob.NewFSStore(t.TempDir())
		require.NoError(t, err)
		return s
	})
}

// TestS3StoreContract runs the identical suite against a real S3-compatible
// bucket. It skips without one; see blobtest for the invocation.
func TestS3StoreContract(t *testing.T) {
	runStoreContract(t, blobtest.S3OrSkip)
}

// patternedBytes is deliberately not random and not uniform: a repeated
// counter makes an off-by-a-part or a re-ordered multipart upload show up as a
// mismatch rather than as bytes that happen to compare equal.
func patternedBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}
