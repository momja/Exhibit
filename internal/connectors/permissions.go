package connectors

import (
	"context"
	"fmt"
	"sync"

	"github.com/momja/Exhibit/internal/store"
)

// UpdatePermissions edits a native connection without acquiring, decrypting,
// or replacing its credential. Requests and revocation share this same lock,
// so a completed permission change governs the next authorized request.
func (s *Service) UpdatePermissions(ctx context.Context, ownerID int64, connectionID string, allowWrites bool) (*Connection, error) {
	if s.cfg.Store == nil {
		return nil, ErrDisabled
	}
	if ownerID <= 0 {
		return nil, store.ErrNotFound
	}
	record, err := s.cfg.Store.GetConnection(ctx, ownerID, connectionID)
	if err != nil {
		return nil, err
	}
	if err := editableNativePermissions(record); err != nil {
		return nil, err
	}
	lock, _ := s.locks.LoadOrStore(connectionID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	// Another request may have revoked the connection while this edit waited.
	record, err = s.cfg.Store.GetConnection(ctx, ownerID, connectionID)
	if err != nil {
		return nil, err
	}
	if err := editableNativePermissions(record); err != nil {
		return nil, err
	}
	if err := s.cfg.Store.UpdateConnectionPermissions(ctx, ownerID, connectionID, allowWrites); err != nil {
		return nil, err
	}
	record, err = s.cfg.Store.GetConnection(ctx, ownerID, connectionID)
	if err != nil {
		return nil, err
	}
	return s.metadata(ctx, record)
}

func editableNativePermissions(record *store.ConnectionRecord) error {
	if record.Status != "active" {
		return store.ErrNotFound
	}
	if record.Kind == "native" {
		switch record.Provider {
		case "api-key", "bearer", "basic":
			return nil
		}
	}
	return fmt.Errorf("%w: write permissions can be edited only for native API key, Bearer, or Basic connections", ErrInvalid)
}
