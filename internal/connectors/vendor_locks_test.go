package connectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVendorFlowLocksKeepWaitersSerializedAndReleaseIdleEntries(t *testing.T) {
	var locks vendorFlowLocks
	first := locks.acquire("flow")
	queued := make(chan func(), 1)
	go func() { queued <- locks.acquire("flow") }()
	waitForUsers := func(count int) {
		t.Helper()
		require.Eventually(t, func() bool {
			locks.mu.Lock()
			defer locks.mu.Unlock()
			return locks.held["flow"] != nil && locks.held["flow"].refs == count
		}, time.Second, time.Millisecond)
	}
	waitForUsers(2)
	// Unrelated authentication flows do not wait for this provider call.
	other := locks.acquire("other")
	other()
	first()
	var second func()
	select {
	case second = <-queued:
	case <-time.After(time.Second):
		t.Fatal("queued flow did not acquire its lock")
	}
	// Releasing the first caller must not replace the mutex underneath its
	// waiter: a new cleanup request must queue behind the second caller.
	go func() { queued <- locks.acquire("flow") }()
	waitForUsers(2)
	select {
	case release := <-queued:
		release()
		t.Error("two callers acquired the same flow concurrently")
	default:
	}
	second()
	select {
	case release := <-queued:
		release()
	case <-time.After(time.Second):
		t.Fatal("final flow caller did not acquire its lock")
	}
	assert.Empty(t, locks.held, "idle flow IDs must not accumulate")
}
