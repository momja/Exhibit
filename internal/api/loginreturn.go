package api

import (
	"strings"
	"sync"
	"time"
)

// loginReturnStore preserves a complete destination across an OIDC login without
// putting an arbitrarily long URL in a cookie. The existing unpredictable OAuth
// state is its key. Restarting or evicting a pending flow only loses its return
// location: a successful login still lands safely at the library.
type loginReturnStore struct {
	mu      sync.Mutex
	pending map[string]loginReturn
}

type loginReturn struct {
	url     string
	expires time.Time
}

const (
	maxLoginReturns     = 1024
	maxLoginReturnBytes = 8 << 20
)

func (s *loginReturnStore) put(state, destination string) {
	if destination == "" || len(destination) > maxLoginReturnBytes {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = make(map[string]loginReturn)
	}
	now := time.Now()
	delete(s.pending, state)
	pendingBytes := 0
	for key, pending := range s.pending {
		if !now.Before(pending.expires) {
			delete(s.pending, key)
			continue
		}
		pendingBytes += len(pending.url)
	}
	// Starting login is public. Eviction keeps memory bounded without letting
	// a full store prevent the next person from starting a login.
	for len(s.pending) >= maxLoginReturns || pendingBytes+len(destination) > maxLoginReturnBytes {
		var oldest string
		var oldestExpiry time.Time
		for key, pending := range s.pending {
			if oldest == "" || pending.expires.Before(oldestExpiry) {
				oldest, oldestExpiry = key, pending.expires
			}
		}
		pendingBytes -= len(s.pending[oldest].url)
		delete(s.pending, oldest)
	}
	s.pending[state] = loginReturn{url: strings.Clone(destination), expires: now.Add(loginFlowTTL)}
}

func (s *loginReturnStore) take(state string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pending[state]
	delete(s.pending, state)
	if !ok || !time.Now().Before(pending.expires) {
		return ""
	}
	return pending.url
}
