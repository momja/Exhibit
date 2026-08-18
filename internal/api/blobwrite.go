package api

import (
	"context"
	"io"
	"log/slog"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/store"
)

// The funnel every blob write and blob deletion passes through, so that
// per-owner storage accounting (av-fw1b) is a property of *writing a blob*
// rather than something each of the five call sites has to remember.
//
// There are only a handful of them — a create, a body edit, a refetch, a
// widget save, and their deletions — and that is exactly why the funnel is
// worth having: five is few enough that they all look correct today and few
// enough that the sixth gets added without anyone noticing the accounting is
// missing from it. Calling Blob.Put directly is now the thing that looks
// wrong.
//
// The length is measured here, at the call site, and deliberately not by
// changing blob.Store.Put's signature: that interface is the seam an
// object-store backend drops in behind (av-52ll, architecture §3.3), and a
// size out-parameter on it would be a second thing every implementation has
// to get right for the benefit of one caller. A counting reader gets the same
// number from any implementation, including one that streams.

// putBlob writes bytes through the blob store and records how many there were.
//
// Bytes first, then the length: a recorded size with no bytes behind it
// over-reports what an owner is holding, where bytes whose length is not yet
// recorded merely under-report — and the under-report is the one a recompute
// can find and fix, since the blob is reachable from the rows either way.
//
// A failure to record the length is logged and NOT returned. The bytes are
// stored and the artifact is fine; only the accounting is stale, by one blob,
// which is exactly what the recompute path exists for. Returning it would turn
// a bookkeeping miss into a 500 on an ingest that had already succeeded — a
// request refused because of this ticket, which is the one thing it promised
// not to do — and would leave the caller unable to tell that their artifact is
// in fact there.
func putBlob(ctx context.Context, st store.Store, blobs blob.Store, blobID string, r io.Reader) error {
	c := &countingReader{r: r}
	if err := blobs.Put(ctx, blobID, c); err != nil {
		return err
	}
	if err := st.RecordBlobSize(ctx, blobID, c.n); err != nil {
		slog.WarnContext(ctx, "record blob size: storage accounting will under-report until recomputed",
			slog.String("blob_id", blobID), slog.Int64("bytes", c.n), slog.String("err", err.Error()))
	}
	return nil
}

// countingReader is the whole measurement: it works for a reader that is a
// []byte today and for one that streams a multi-megabyte snapshot payload
// later, and it never holds the bytes.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
