package agent

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventLineLimitAndRecovery(t *testing.T) {
	for _, size := range []int{maxEventLineBytes - 1, maxEventLineBytes, maxEventLineBytes + 1} {
		for _, terminated := range []bool{false, true} {
			suffix := ""
			if terminated {
				suffix = "\nnext\n"
			}
			r := bufio.NewReaderSize(io.MultiReader(io.LimitReader(zeroReader{}, int64(size)), strings.NewReader(suffix)), 4096)
			line, oversized, err := readEventLine(r)
			wantSize := size
			if terminated {
				wantSize++
			}
			assert.Equal(t, wantSize > maxEventLineBytes, oversized)
			if oversized {
				assert.Nil(t, line)
			} else {
				assert.Len(t, line, wantSize)
			}
			if terminated {
				require.NoError(t, err)
				next, over, err := readEventLine(r)
				require.NoError(t, err)
				assert.False(t, over)
				assert.Equal(t, "next\n", string(next))
			} else {
				assert.ErrorIs(t, err, io.EOF)
			}
		}
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestReadLoopDropsOversizedEventAndContinues(t *testing.T) {
	s := newDetachedSession(t, "")
	s.readDone = make(chan struct{})
	s.readLoop(io.MultiReader(io.LimitReader(zeroReader{}, maxEventLineBytes+1), strings.NewReader("\n{\"type\":\"test\"}\n")))
	require.Len(t, s.backlog, 1)
	assert.JSONEq(t, `{"type":"test"}`, string(s.backlog[0]))
}

func TestReplayEvictsByBytesAndCount(t *testing.T) {
	s := newDetachedSession(t, "")
	// Distinct allocations model actual subprocess events.
	for i := 0; i < 4; i++ {
		s.broadcast(bytes.Repeat([]byte{byte(i)}, maxEventLineBytes))
	}
	require.Len(t, s.backlog, 2)
	assert.Equal(t, maxEventQueueBytes, s.backlogBytes)
	assert.Equal(t, byte(2), s.backlog[0][0])
	for i := 0; i < maxBacklog+1; i++ {
		s.broadcast([]byte("x"))
	}
	require.Len(t, s.backlog, maxBacklog)
	assert.Equal(t, maxBacklog, s.backlogBytes)
}

func TestSlowSubscriberQueueIsBoundedAndOrdered(t *testing.T) {
	sub := newEventSubscriber()
	for i := 0; i < 4; i++ {
		sub.enqueue(bytes.Repeat([]byte{byte(i)}, maxEventLineBytes))
	}
	require.Len(t, sub.queue, 2)
	assert.Equal(t, maxEventQueueBytes, sub.bytes)
	done := make(chan struct{})
	close(done)
	go sub.run(done)
	for _, want := range []byte{2, 3} {
		select {
		case ev := <-sub.events:
			require.NotEmpty(t, ev)
			assert.Equal(t, want, ev[0])
		case <-time.After(time.Second):
			t.Fatal("event not delivered")
		}
	}
	select {
	case _, open := <-sub.events:
		assert.False(t, open)
	case <-time.After(time.Second):
		t.Fatal("subscriber did not close after draining")
	}
	sub.mu.Lock()
	defer sub.mu.Unlock()
	assert.Nil(t, sub.queue)
	assert.Zero(t, sub.bytes)
}

func TestSubscriptionsAreLimitedAndReleaseCapacity(t *testing.T) {
	s := newDetachedSession(t, "")
	var releases []func()
	for i := 0; i < maxSubscribers; i++ {
		_, release := s.Subscribe()
		releases = append(releases, release)
		defer release()
	}
	excess, release := s.Subscribe()
	release()
	_, open := <-excess
	assert.False(t, open)
	releases[0]()
	events, release := s.Subscribe()
	defer release()
	s.broadcast([]byte("hello"))
	select {
	case ev := <-events:
		assert.Equal(t, "hello", string(ev))
	case <-time.After(time.Second):
		t.Fatal("new subscriber not admitted")
	}
}

func TestSubscriberCountEvictionAndUnsubscribe(t *testing.T) {
	sub := newEventSubscriber()
	for i := 0; i < maxBacklog+10; i++ {
		sub.enqueue([]byte("x"))
	}
	assert.Len(t, sub.queue, maxBacklog)
	assert.Equal(t, maxBacklog, sub.bytes)
	go sub.run(make(chan struct{}))
	sub.stop()
	select {
	case _, open := <-sub.events:
		if open {
			select {
			case _, open = <-sub.events:
				assert.False(t, open)
			case <-time.After(time.Second):
				t.Fatal("unsubscribe left worker running")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("unsubscribe left worker running")
	}
	sub.mu.Lock()
	defer sub.mu.Unlock()
	assert.Nil(t, sub.queue)
	assert.Zero(t, sub.bytes)
}

func TestReplayOwnsOnlyPublishedBytes(t *testing.T) {
	s := newDetachedSession(t, "")
	backing := bytes.Repeat([]byte("x"), maxEventLineBytes)
	s.broadcast(backing[:1])
	backing[0] = 'y'
	require.Len(t, s.backlog, 1)
	assert.Equal(t, "x", string(s.backlog[0]))
	assert.Less(t, cap(s.backlog[0]), 1024, "small records cannot retain an oversized reader allocation")
}
