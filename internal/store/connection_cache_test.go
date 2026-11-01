package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cacheTestConnection(t *testing.T, s *SQLiteStore, id string) {
	t.Helper()
	require.NoError(t, s.CreateConnection(context.Background(), &ConnectionRecord{ID: id, OwnerID: alice, Name: id, Provider: "bearer", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}, -1))
}

func TestConnectionCacheExpiresOnReadWriteAndMaintenance(t *testing.T) {
	for _, operation := range []string{"read", "write", "maintenance"} {
		t.Run(operation, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			cacheTestConnection(t, s, "quiet")
			cacheTestConnection(t, s, "busy")
			now := time.Now()
			require.NoError(t, s.PutConnectionCache(ctx, "quiet", "expired", "old-data", now.Add(-2*time.Hour), now.Add(-time.Hour)))
			switch operation {
			case "read":
				_, err := s.GetConnectionCache(ctx, "busy", "missing", now)
				require.ErrorIs(t, err, ErrNotFound)
			case "write":
				require.NoError(t, s.PutConnectionCache(ctx, "busy", "fresh", "new-data", now, now.Add(time.Hour)))
			case "maintenance":
				require.NoError(t, s.PruneConnectionCache(ctx, now))
			}
			var count int
			require.NoError(t, s.db.QueryRowContext(ctx, `SELECT count(*) FROM connection_cache WHERE connection_id='quiet'`).Scan(&count))
			assert.Zero(t, count, "expired ciphertext must be deleted, not merely hidden")
		})
	}
}

func TestConnectionCacheBoundsWritesAndReplacements(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cacheTestConnection(t, s, "busy")
	now := time.Now()
	for i := 0; i < defaultConnectionCacheLimits.connectionEntries+5; i++ {
		require.NoError(t, s.PutConnectionCache(ctx, "busy", fmt.Sprint(i), "encrypted", now, now.Add(time.Duration(i+1)*time.Minute)))
	}
	var count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT count(*) FROM connection_cache`).Scan(&count))
	assert.Equal(t, defaultConnectionCacheLimits.connectionEntries, count)
	_, err := s.GetConnectionCache(ctx, "busy", "0", now)
	assert.ErrorIs(t, err, ErrNotFound, "the soonest-expiring entries are evicted first")
	newest := fmt.Sprint(defaultConnectionCacheLimits.connectionEntries + 4)
	require.NoError(t, s.PutConnectionCache(ctx, "busy", newest, "replacement", now, now.Add(2*time.Hour)))
	sealed, err := s.GetConnectionCache(ctx, "busy", newest, now)
	require.NoError(t, err)
	assert.Equal(t, "replacement", sealed)
	var size int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT encrypted_bytes FROM connection_cache WHERE connection_id=? AND request_hash=?`, "busy", newest).Scan(&size))
	assert.Equal(t, len(sealed), size, "replacements update their persisted byte count")
	require.NoError(t, s.PutConnectionCache(ctx, "busy", newest, strings.Repeat("x", maxConnectionCacheEntryBytes+1), now, now.Add(time.Hour)))
	_, err = s.GetConnectionCache(ctx, "busy", newest, now)
	assert.ErrorIs(t, err, ErrNotFound, "a rejected replacement cannot leave stale bytes")
}

func TestConnectionCacheMigrationBackfillsByteCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cacheTestConnection(t, s, "legacy")
	require.NoError(t, goose.DownToContext(ctx, s.db, "migrations", 52))
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO connection_cache(connection_id,request_hash,encrypted_response,expires_at) VALUES(?,?,?,?)`, "legacy", "old", "encrypted-data", now.Add(time.Hour).Unix())
	require.NoError(t, err)
	require.NoError(t, s.migrate())
	var size int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT encrypted_bytes FROM connection_cache WHERE connection_id='legacy'`).Scan(&size))
	assert.Equal(t, len("encrypted-data"), size)
	sealed, err := s.GetConnectionCache(ctx, "legacy", "old", now)
	require.NoError(t, err)
	assert.Equal(t, "encrypted-data", sealed)
	require.NoError(t, s.PruneConnectionCache(ctx, now.Add(2*time.Hour)))
	_, err = s.GetConnectionCache(ctx, "legacy", "old", now)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestConnectionCachePruningEnforcesAllAggregateBudgets(t *testing.T) {
	// Small independent budgets exercise the same eviction SQL without writing
	// hundreds of megabytes in a test. Seed legacy rows directly: maintenance
	// must repair old databases as well as bounding current inserts.
	for _, tc := range []struct {
		name                 string
		limits               connectionCacheLimits
		wantCount, wantBytes int
	}{
		{"per-connection count", connectionCacheLimits{2, 100, 100, 1000}, 4, 16},
		{"per-connection bytes", connectionCacheLimits{100, 9, 100, 1000}, 4, 16},
		{"global count", connectionCacheLimits{100, 100, 3, 1000}, 3, 12},
		{"global bytes", connectionCacheLimits{100, 100, 100, 13}, 3, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			now := time.Now()
			for _, id := range []string{"one", "two"} {
				cacheTestConnection(t, s, id)
				for i := 1; i <= 4; i++ {
					_, err := s.db.ExecContext(ctx, `INSERT INTO connection_cache(connection_id,request_hash,encrypted_response,expires_at,encrypted_bytes) VALUES(?,?,?,?,?)`, id, fmt.Sprint(i), "abcd", now.Add(time.Duration(i)*time.Hour).Unix(), 4)
					require.NoError(t, err)
				}
			}
			tx, err := s.db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer tx.Rollback()
			require.NoError(t, pruneConnectionCache(ctx, tx, now, tc.limits))
			require.NoError(t, tx.Commit())
			var count, size int
			require.NoError(t, s.db.QueryRowContext(ctx, `SELECT count(*),sum(length(encrypted_response)) FROM connection_cache`).Scan(&count, &size))
			assert.Equal(t, tc.wantCount, count)
			assert.Equal(t, tc.wantBytes, size)
			_, err = s.GetConnectionCache(ctx, "one", "1", now)
			assert.ErrorIs(t, err, ErrNotFound)
			_, err = s.GetConnectionCache(ctx, "one", "4", now)
			require.NoError(t, err, "entries with longer remaining lifetimes survive")
		})
	}
}
