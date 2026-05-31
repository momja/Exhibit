package blob

import (
	"context"
	"io"
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
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (s *FSStore) Get(ctx context.Context, id string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.dir, id))
}
