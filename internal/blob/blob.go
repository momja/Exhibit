package blob

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type Store interface {
	Put(ctx context.Context, id string, r io.Reader) error
	Get(ctx context.Context, id string) (io.ReadCloser, error)
	// Delete removes the bytes stored under id (av-7jcq).
	//
	// The contract is **idempotent**: an id that was never stored, or that a
	// previous partially-failed delete already removed, is success. Only a
	// delete that genuinely could not happen — a permission error, a failing
	// disk, a refused API call — returns an error, so a caller may read a nil
	// return as "these bytes are not here any more" without first having to
	// establish whether they ever were.
	//
	// Two reasons it is defined that way rather than "missing is an error":
	//
	//  1. It is the contract the *other* backend already has. This interface
	//     is the seam an S3/MinIO implementation drops in behind
	//     (architecture §3.3), and S3's DeleteObject answers success for a key
	//     that does not exist. Making missing an error here would force the
	//     object-store backend to synthesize a failure — a HEAD before every
	//     delete, racy and paid on every call — purely to honour a distinction
	//     no caller wants.
	//  2. Every caller's intent is "these bytes must not exist", which a
	//     missing blob already satisfies. That is the same reasoning
	//     store.DeleteState and ClearState are idempotent for, and the
	//     alternative gives each caller an errors.Is(…, fs.ErrNotExist) branch
	//     whose body means "fine".
	//
	// An implementation must not leak its backend's own missing-key error;
	// FSStore below swallows os.ErrNotExist rather than passing it up.
	Delete(ctx context.Context, id string) error
}

type FSStore struct {
	dir string
}

func NewFSStore(dir string) (*FSStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &FSStore{dir: dir}, nil
}

func (s *FSStore) Put(ctx context.Context, id string, r io.Reader) error {
	path := filepath.Join(s.dir, id)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, r)
	if err != nil {
		_ = f.Close() // copy already failed; return that error, not Close's
		return err
	}
	// On the write path a Close error can mean the bytes never flushed, so it
	// must surface rather than be dropped by a bare defer.
	if err := f.Close(); err != nil {
		return err
	}
	slog.DebugContext(ctx, "blob stored", slog.String("id", id), slog.Int64("bytes", n))
	return nil
}

func (s *FSStore) Get(ctx context.Context, id string) (io.ReadCloser, error) {
	rc, err := os.Open(filepath.Join(s.dir, id))
	if err == nil {
		slog.DebugContext(ctx, "blob opened", slog.String("id", id))
	}
	return rc, err
}

// Delete removes the file holding this blob, honouring Store.Delete's
// idempotent contract: os.Remove reports ErrNotExist for a path that is not
// there and this is where that is absorbed, so the distinction never reaches a
// caller. Anything else — a read-only volume, a permission problem — is a real
// failure and surfaces, because the whole point of this method is that a
// deletion which claims to have removed the bytes did.
func (s *FSStore) Delete(ctx context.Context, id string) error {
	err := os.Remove(filepath.Join(s.dir, id))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	slog.DebugContext(ctx, "blob deleted", slog.String("id", id))
	return nil
}
