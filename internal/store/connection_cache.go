package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Bound both large responses and many small distinct requests. These limits
// apply to encrypted bytes, including the encoding overhead, across all owners.
const maxConnectionCacheEntryBytes = 12 << 20

type connectionCacheLimits struct {
	connectionEntries int
	connectionBytes   int
	globalEntries     int
	globalBytes       int
}

var defaultConnectionCacheLimits = connectionCacheLimits{64, 32 << 20, 1024, 256 << 20}

func (s *SQLiteStore) GetConnectionCache(ctx context.Context, id, hash string, now time.Time) (string, error) {
	// Expired ciphertext is discarded on reads as well as writes and periodic
	// maintenance; a quiet connection cannot retain it indefinitely.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM connection_cache WHERE expires_at<=?`, now.Unix()); err != nil {
		return "", err
	}
	var sealed string
	// Check size in SQLite before copying an old oversized record into Go.
	err := s.db.QueryRowContext(ctx, `SELECT encrypted_response FROM connection_cache
 WHERE connection_id=? AND request_hash=? AND expires_at>? AND encrypted_bytes<=?`, id, hash, now.Unix(), maxConnectionCacheEntryBytes).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return sealed, err
}

func (s *SQLiteStore) PutConnectionCache(ctx context.Context, id, hash, sealed string, now, expires time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A skipped replacement must not leave a previously cached response behind.
	if expires.After(now) && len(sealed) <= maxConnectionCacheEntryBytes {
		_, err = tx.ExecContext(ctx, `INSERT INTO connection_cache(connection_id,request_hash,encrypted_response,expires_at,encrypted_bytes) VALUES(?,?,?,?,?)
 ON CONFLICT(connection_id,request_hash) DO UPDATE SET encrypted_response=excluded.encrypted_response,expires_at=excluded.expires_at,encrypted_bytes=excluded.encrypted_bytes`, id, hash, sealed, expires.Unix(), len(sealed))
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM connection_cache WHERE connection_id=? AND request_hash=?`, id, hash)
	}
	if err != nil {
		return err
	}
	if err := pruneConnectionCache(ctx, tx, now, defaultConnectionCacheLimits); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) DeleteConnectionCache(ctx context.Context, id, hash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM connection_cache WHERE connection_id=? AND request_hash=?`, id, hash)
	return err
}

// PruneConnectionCache also runs without cache traffic, at startup and during
// connector maintenance. It bounds legacy entries as well as current writes.
func (s *SQLiteStore) PruneConnectionCache(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := pruneConnectionCache(ctx, tx, now, defaultConnectionCacheLimits); err != nil {
		return err
	}
	return tx.Commit()
}

func pruneConnectionCache(ctx context.Context, tx *sql.Tx, now time.Time, limits connectionCacheLimits) error {
	// The OR predicate otherwise favors a table scan, whose trailing size
	// column can require reading through large ciphertext overflow pages.
	if _, err := tx.ExecContext(ctx, `DELETE FROM connection_cache INDEXED BY connection_cache_retention WHERE expires_at<=? OR encrypted_bytes>?`, now.Unix(), maxConnectionCacheEntryBytes); err != nil {
		return err
	}
	// Keep entries with the longest remaining lifetime, using stable tie breaks.
	// Enforce per-connection budgets before the global budget so one connection
	// cannot monopolize all cache storage. A write and its eviction commit as one.
	// Sum persisted sizes, not ciphertext: scanning large payloads here would
	// hold the database writer for seconds whenever the cache fills up.
	if _, err := tx.ExecContext(ctx, `WITH ranked AS (
 SELECT connection_id,request_hash,
 row_number() OVER (PARTITION BY connection_id ORDER BY expires_at DESC,request_hash) AS n,
 sum(encrypted_bytes) OVER (PARTITION BY connection_id ORDER BY expires_at DESC,request_hash ROWS UNBOUNDED PRECEDING) AS bytes
 FROM connection_cache
 ) DELETE FROM connection_cache WHERE (connection_id,request_hash) IN (
 SELECT connection_id,request_hash FROM ranked WHERE n>? OR bytes>?)`, limits.connectionEntries, limits.connectionBytes); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `WITH ranked AS (
 SELECT connection_id,request_hash,
 row_number() OVER (ORDER BY expires_at DESC,connection_id,request_hash) AS n,
 sum(encrypted_bytes) OVER (ORDER BY expires_at DESC,connection_id,request_hash ROWS UNBOUNDED PRECEDING) AS bytes
 FROM connection_cache
 ) DELETE FROM connection_cache WHERE (connection_id,request_hash) IN (
 SELECT connection_id,request_hash FROM ranked WHERE n>? OR bytes>?)`, limits.globalEntries, limits.globalBytes)
	return err
}
