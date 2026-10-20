package agentcontrol

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fenceReferences(f *conversationFences, conversation string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entry := f.entries[conversation]; entry != nil {
		return entry.references
	}
	return 0
}

func TestConversationFencesReleaseCanceledWaiterReferences(t *testing.T) {
	for _, mode := range []string{"takeover", "operation", "create"} {
		t.Run(mode, func(t *testing.T) {
			var fences conversationFences
			heldExclusive := mode == "operation"
			creating := mode == "create"
			release, err := fences.acquire(context.Background(), "conversation", heldExclusive, creating)
			require.NoError(t, err)
			defer release()
			waiterCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				unlock, err := fences.acquire(waiterCtx, "conversation", mode == "takeover", creating)
				if err == nil {
					unlock()
				}
				done <- err
			}()
			require.Eventually(t, func() bool { return fenceReferences(&fences, "conversation") == 2 }, time.Second, time.Millisecond)
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("cancellation did not release waiter")
			}
			assert.Equal(t, 1, fenceReferences(&fences, "conversation"), "the holder alone retains this entry")
			release()
			release()
			require.Nil(t, fences.entries, "last release drops the map and its buckets")
		})
	}
}

func TestConversationFencesCanceledContextDoesNotRetainEntries(t *testing.T) {
	var fences conversationFences
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 1000; i++ {
		_, err := fences.acquire(canceled, fmt.Sprint(i), true, false)
		require.ErrorIs(t, err, context.Canceled)
	}
	require.Nil(t, fences.entries)
}

// References must cover waiters as well as holders; deleting an entry too early
// could give a new operation a different lock while the first waiter takes over.
func TestConversationFencesConcurrentReuseAndCleanup(t *testing.T) {
	var fences conversationFences
	const conversations = 16
	var operations [conversations]atomic.Int64
	var creates [conversations]atomic.Int64
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := (worker + i) % conversations
				exclusive := i%3 == 0
				creating := i%3 == 1
				release, err := fences.acquire(context.Background(), fmt.Sprint(key), exclusive, creating)
				if !assert.NoError(t, err) {
					return
				}
				if exclusive {
					assert.True(t, operations[key].CompareAndSwap(0, -1), "a takeover must exclude every other operation")
					assert.True(t, operations[key].CompareAndSwap(-1, 0))
				} else {
					assert.Positive(t, operations[key].Add(1))
					if creating {
						assert.True(t, creates[key].CompareAndSwap(0, 1))
						assert.True(t, creates[key].CompareAndSwap(1, 0))
					}
					assert.GreaterOrEqual(t, operations[key].Add(-1), int64(0))
				}
				release()
			}
		}(worker)
	}
	wg.Wait()
	require.Nil(t, fences.entries)
}

type refusingFenceRepository struct{ Repository }

func (refusingFenceRepository) ReserveAgentExecution(context.Context, Reservation, time.Time) (Assignment, error) {
	return Assignment{}, ErrInvalid
}
func (refusingFenceRepository) GetAgentExecution(context.Context, Identity, time.Time) (Assignment, error) {
	return Assignment{}, ErrStale
}
func (refusingFenceRepository) TransitionAgentExecution(context.Context, Identity, string, time.Time, time.Time) error {
	return ErrStale
}

func TestControlReleasesConversationEntriesOnRepositoryFailure(t *testing.T) {
	c := New(refusingFenceRepository{}, nil, Config{})
	for i := 0; i < 1000; i++ {
		conversation := fmt.Sprint(i)
		_, err := c.Reserve(context.Background(), Reservation{ConversationID: conversation})
		require.ErrorIs(t, err, ErrInvalid)
		id := Identity{ConversationID: conversation}
		_, err = c.Guard(context.Background(), id)
		require.ErrorIs(t, err, ErrStale)
		require.ErrorIs(t, c.Release(context.Background(), id, Closed), ErrStale)
	}
	require.Nil(t, c.fences.entries)
}
