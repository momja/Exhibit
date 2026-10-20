package agentcontrol

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

// Weighted semaphores provide cancellable shared/exclusive acquisition without
// a goroutine per waiter. Later readers cannot bypass a queued exclusive waiter.
// The weight is a lock implementation detail, not a session limit.
const conversationFenceWeight int64 = 1 << 30

type conversationFence struct {
	operations *semaphore.Weighted
	create     *semaphore.Weighted
	references int
}

type conversationFences struct {
	mu      sync.Mutex
	entries map[string]*conversationFence
}

// acquire retains the entry before waiting and releases that reference on every
// failure or final unlock. Thus holders and waiters always share one entry,
// while memory is bounded by concurrent operations rather than historical IDs.
// Never hold the map mutex during semaphore acquisition or repository calls.
func (f *conversationFences) acquire(ctx context.Context, conversation string, exclusive, creating bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if conversation == "" {
		return nil, ErrInvalid
	}
	f.mu.Lock()
	if f.entries == nil {
		f.entries = make(map[string]*conversationFence)
	}
	entry := f.entries[conversation]
	if entry == nil {
		entry = &conversationFence{operations: semaphore.NewWeighted(conversationFenceWeight), create: semaphore.NewWeighted(1)}
		f.entries[conversation] = entry
	}
	entry.references++
	f.mu.Unlock()

	// Wait for create serialization before taking shared authority, so queued
	// creates do not themselves hold operation fences that delay takeover.
	if creating {
		if err := entry.create.Acquire(ctx, 1); err != nil {
			f.releaseReference(conversation, entry)
			return nil, err
		}
	}
	weight := int64(1)
	if exclusive {
		weight = conversationFenceWeight
	}
	if err := entry.operations.Acquire(ctx, weight); err != nil {
		if creating {
			entry.create.Release(1)
		}
		f.releaseReference(conversation, entry)
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			entry.operations.Release(weight)
			if creating {
				entry.create.Release(1)
			}
			f.releaseReference(conversation, entry)
		})
	}, nil
}

func (f *conversationFences) releaseReference(conversation string, entry *conversationFence) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry.references--
	if entry.references == 0 {
		delete(f.entries, conversation)
		// Dropping the empty map also releases its buckets after a concurrency
		// burst; no empty historical conversation cache remains resident.
		if len(f.entries) == 0 {
			f.entries = nil
		}
	}
}
