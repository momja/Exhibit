package api

import (
	"context"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
)

func TestConnectionCacheMaintenanceRunsWithConnectorsDisabled(t *testing.T) {
	ro := newTestRouter(t)
	pruned := make(chan time.Time, 1)
	ro.cfg.Store = &recordingConnectionCachePruner{Store: ro.cfg.Store, pruned: pruned}
	ro.cfg.Connections = nil
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ro.StartConnectorMaintenance(ctx)
	select {
	case <-pruned:
	case <-time.After(5 * time.Second):
		t.Fatal("startup must prune cached provider data even with connectors disabled")
	}
}

type recordingConnectionCachePruner struct {
	store.Store
	pruned chan time.Time
}

func (s *recordingConnectionCachePruner) PruneConnectionCache(ctx context.Context, now time.Time) error {
	select {
	case s.pruned <- now:
	case <-ctx.Done():
	}
	return nil
}
