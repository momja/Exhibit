package blob

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type Store interface {
	Put(ctx context.Context, id string, r io.Reader) error
	Get(ctx context.Context, id string) (io.ReadCloser, error)
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
