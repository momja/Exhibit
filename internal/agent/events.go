package agent

import (
	"bufio"
	"errors"
	"sync"
)

const (
	// Large tool results are allowed, but a malformed JSONL record cannot
	// allocate arbitrarily. The limit includes the line terminator.
	maxEventLineBytes  = 8 << 20
	maxEventQueueBytes = 16 << 20
	maxBacklog         = 4096
	maxSubscribers     = 4
)

// readEventLine discards an oversized record through its terminator. ReadSlice
// keeps even an endless unterminated record bounded; partial JSON is never used.
func readEventLine(reader *bufio.Reader) ([]byte, bool, error) {
	var line []byte
	oversized := false
	for {
		fragment, err := reader.ReadSlice('\n')
		if !oversized {
			if len(line)+len(fragment) > maxEventLineBytes {
				line = nil
				oversized = true
			} else {
				line = append(line, fragment...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, oversized, err
	}
}

// Each subscriber owns a bounded queue and at most one event being delivered.
// Queues share immutable event bytes with replay. Slow clients lose the oldest
// queued events, matching the stream's existing best-effort delivery contract.
// With four subscribers, retained payload is at most 16 MiB replay plus four
// times (16 MiB queue + 8 MiB in flight), independent of client reading speed.
type eventSubscriber struct {
	mu       sync.Mutex
	queue    [][]byte
	bytes    int
	events   chan []byte
	wake     chan struct{}
	done     chan struct{}
	finished chan struct{}
	once     sync.Once
}

func newEventSubscriber() *eventSubscriber {
	return &eventSubscriber{
		events:   make(chan []byte),
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (s *eventSubscriber) enqueue(line []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, line)
	s.bytes += len(line)
	for len(s.queue) > maxBacklog || s.bytes > maxEventQueueBytes {
		s.bytes -= len(s.queue[0])
		s.queue[0] = nil
		s.queue = s.queue[1:]
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *eventSubscriber) stop() {
	s.once.Do(func() { close(s.done) })
}

func (s *eventSubscriber) run(sessionDone <-chan struct{}) {
	defer close(s.finished)
	defer close(s.events)
	defer func() {
		s.mu.Lock()
		s.queue = nil
		s.bytes = 0
		s.mu.Unlock()
	}()
	for {
		select {
		case <-s.done:
			return
		default:
		}
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.mu.Unlock()
			select {
			case <-s.wake:
				continue
			case <-s.done:
				return
			case <-sessionDone:
				s.mu.Lock()
				empty := len(s.queue) == 0
				s.mu.Unlock()
				if empty {
					return
				}
				continue
			}
		}
		line := s.queue[0]
		s.queue[0] = nil
		s.queue = s.queue[1:]
		s.bytes -= len(line)
		s.mu.Unlock()
		select {
		case s.events <- line:
		case <-s.done:
			return
		}
	}
}
