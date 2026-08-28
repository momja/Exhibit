package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"
)

// The SSE stream is the one route a browser cannot authenticate with a
// header: EventSource sends no Authorization. The old answer was to put the
// service bearer token in the query string, which meant the instance's single
// master credential travelled through this service's debug request log, the
// operator's proxy access log, and browser history (av-rgp1).
//
// An SSE ticket replaces it. It is minted by an ordinary authenticated request,
// is bound to one session, is single-use, and dies in seconds — so a ticket
// recovered from a log line buys at most a brief replay window on one session's
// event stream, never the library. Tickets live only in memory, like the
// sessions they name: both die with the process, so there is nothing to
// persist.
const (
	sseTicketTTL = 30 * time.Second
	// A session mints a ticket per connect, and reconnects can outpace
	// expiry on a flaky link. Cap the live set per session so a client stuck
	// in a reconnect loop cannot grow the map without bound.
	maxTicketsPerSession = 16
)

type sseTicket struct {
	value   string
	ownerID int64
	expires time.Time
}

// sseTicketStore holds the live tickets, keyed by the session each is bound to.
// Keying by session is what makes a ticket minted for one session useless on
// another: the redeeming route looks up only its own session's tickets.
type sseTicketStore struct {
	ttl time.Duration
	now func() time.Time // swapped in tests to age tickets without sleeping

	mu        sync.Mutex
	bySession map[string][]sseTicket
}

func newSSETicketStore(ttl time.Duration) *sseTicketStore {
	return &sseTicketStore{
		ttl:       ttl,
		now:       time.Now,
		bySession: map[string][]sseTicket{},
	}
}

// Issue mints a single-use ticket for one session's event stream.
func (s *sseTicketStore) Issue(sessionID string, ownerID int64) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	value := base64.RawURLEncoding.EncodeToString(buf)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	live := append(s.liveLocked(sessionID, now), sseTicket{
		value:   value,
		ownerID: ownerID,
		expires: now.Add(s.ttl),
	})
	if len(live) > maxTicketsPerSession {
		live = live[len(live)-maxTicketsPerSession:]
	}
	s.bySession[sessionID] = live
	return value, nil
}

// Redeem consumes a ticket, reporting the owner it was minted for. A ticket
// that is expired, already redeemed, or bound to a different session fails —
// there is one way to succeed and every other case is the same "no".
func (s *sseTicketStore) Redeem(sessionID, value string) (int64, bool) {
	if value == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	live := s.liveLocked(sessionID, now)
	for i, t := range live {
		if subtle.ConstantTimeCompare([]byte(t.value), []byte(value)) != 1 {
			continue
		}
		s.storeLocked(sessionID, append(live[:i:i], live[i+1:]...))
		return t.ownerID, true
	}
	s.storeLocked(sessionID, live)
	return 0, false
}

// Forget drops every ticket for a session, called when the session closes so a
// ticket cannot outlive the stream it names.
func (s *sseTicketStore) Forget(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bySession, sessionID)
}

// liveLocked returns the session's unexpired tickets. Expiry is swept lazily on
// the paths that already hold the lock — the set is small and short-lived, so a
// sweeper goroutine would be more machinery than the problem deserves. While
// the lock is already held, it also sweeps every *other* session in the store:
// a session that mints a ticket once and is later reaped without an explicit
// close (the idle reaper has no reference to this store, so it never calls
// Forget) would otherwise leave a dead entry behind forever. Piggybacking the
// full sweep on whichever call happens to hold the lock keeps the map bounded
// without adding a goroutine of its own.
func (s *sseTicketStore) liveLocked(sessionID string, now time.Time) []sseTicket {
	var requested []sseTicket
	for id, all := range s.bySession {
		live := all[:0:0]
		for _, t := range all {
			if t.expires.After(now) {
				live = append(live, t)
			}
		}
		if len(live) == 0 {
			delete(s.bySession, id)
		} else {
			s.bySession[id] = live
		}
		if id == sessionID {
			requested = live
		}
	}
	return requested
}

func (s *sseTicketStore) storeLocked(sessionID string, live []sseTicket) {
	if len(live) == 0 {
		delete(s.bySession, sessionID)
		return
	}
	s.bySession[sessionID] = live
}
