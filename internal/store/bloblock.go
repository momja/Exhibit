package store

import (
	"sort"
	"sync"
)

// Per-blob exclusion between writing a reference and unlinking the bytes
// (av-8gyd).
//
// The queue's refcount makes "condemn only what nothing names" a property of a
// transaction, but the *unlink* happens outside that transaction and outside
// SQLite entirely, so the recheck the drain takes before it deletes is a claim
// about the past by the time Blob.Delete runs. One interleaving turns that gap
// into a live artifact with no bytes:
//
//	drain          rechecks blob X            → 0 references, proceed
//	ingest         writes X's bytes, commits an artifact_assets row naming X
//	drain          Blob.Delete(X)             → a referenced payload is gone
//
// It is reachable because asset blob ids are content addresses (av-20fk): a
// re-ingest of the same payload names the very id the queue is holding, and a
// drain that failed leaves its row for a startup that may be many ingests
// later. Bodies and widgets are minted UUIDs and can never come back, so this
// is asset-shaped by construction — but the lock is keyed by blob id and knows
// nothing about that, because the *next* content-addressed thing should
// inherit the protection rather than rediscover the bug.
//
// Both sides therefore hold the same per-id lock: the ingest across
// [write bytes … commit the referencing row], the drain across
// [recheck … unlink … dequeue]. Whichever wins, the loser sees a settled
// world — either a reference the recheck now finds, or bytes it must write
// again.
//
// **Never take one of these while holding a transaction.** The database runs
// on a single connection (OpenSQLite), so a caller sitting in a tx and waiting
// for a blob lock would hold the connection the lock's owner needs to finish.
// Take the lock first, do the blob write, then open the transaction — which is
// the order the work happens in anyway.
type blobLocks struct {
	mu   sync.Mutex
	held map[string]*blobLockEntry
}

// blobLockEntry is one id's mutex plus the number of holders and waiters, so
// the map keeps an entry exactly as long as somebody is interested in it and
// a busy instance does not accumulate one per blob it has ever touched.
type blobLockEntry struct {
	mu   sync.Mutex
	refs int
}

func (l *blobLocks) lock(id string) {
	l.mu.Lock()
	if l.held == nil {
		l.held = make(map[string]*blobLockEntry)
	}
	e := l.held[id]
	if e == nil {
		e = &blobLockEntry{}
		l.held[id] = e
	}
	e.refs++
	l.mu.Unlock()

	e.mu.Lock()
}

func (l *blobLocks) unlock(id string) {
	l.mu.Lock()
	e := l.held[id]
	if e == nil {
		l.mu.Unlock()
		return
	}
	e.refs--
	if e.refs == 0 {
		delete(l.held, id)
	}
	l.mu.Unlock()

	e.mu.Unlock()
}

// LockBlobs takes the per-blob lock for every id and returns the release.
//
// Ids are deduplicated and taken in sorted order, so two callers naming
// overlapping sets acquire them in the same sequence and cannot deadlock
// against each other. Releasing is safe to call once; calling it twice is not.
func (s *SQLiteStore) LockBlobs(ids ...string) func() {
	uniq := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}
	sort.Strings(uniq)

	for _, id := range uniq {
		s.blobLocks.lock(id)
	}
	return func() {
		for i := len(uniq) - 1; i >= 0; i-- {
			s.blobLocks.unlock(uniq[i])
		}
	}
}
