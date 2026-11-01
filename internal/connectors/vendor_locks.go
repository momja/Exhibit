package connectors

import "sync"

// Flow locks serialize remote account adoption and cleanup. Holders and
// waiters retain the same mutex; completed or rejected attempts leave no entry.
type vendorFlowLocks struct {
	mu   sync.Mutex
	held map[string]*vendorFlowLock
}

type vendorFlowLock struct {
	mu   sync.Mutex
	refs int
}

// acquire returns a release function that must be called exactly once.
func (l *vendorFlowLocks) acquire(id string) func() {
	l.mu.Lock()
	if l.held == nil {
		l.held = make(map[string]*vendorFlowLock)
	}
	entry := l.held[id]
	if entry == nil {
		entry = &vendorFlowLock{}
		l.held[id] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return func() {
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.held, id)
		}
		entry.mu.Unlock()
		l.mu.Unlock()
	}
}
