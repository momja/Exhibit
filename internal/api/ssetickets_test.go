package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A ticket is single-use: the first redemption succeeds and returns the owner
// it was minted for, and a replay of the same value is rejected. This is what
// bounds the damage of a ticket recovered from a log line (av-rgp1).
func TestSSETicketIsSingleUse(t *testing.T) {
	ts := newSSETicketStore(sseTicketTTL)

	tkt, err := ts.Issue("sess-1", 7)
	require.NoError(t, err)
	require.NotEmpty(t, tkt)

	owner, ok := ts.Redeem("sess-1", tkt)
	require.True(t, ok)
	assert.Equal(t, int64(7), owner)

	_, ok = ts.Redeem("sess-1", tkt)
	assert.False(t, ok, "a redeemed ticket must not be replayable")
}

// A ticket expires in seconds, so a leaked one is useless almost immediately.
func TestSSETicketExpires(t *testing.T) {
	ts := newSSETicketStore(30 * time.Second)
	base := time.Now()
	ts.now = func() time.Time { return base }

	assert.LessOrEqual(t, sseTicketTTL, time.Minute, "the TTL must stay in the seconds range")

	tkt, err := ts.Issue("sess-1", 1)
	require.NoError(t, err)

	ts.now = func() time.Time { return base.Add(29 * time.Second) }
	_, ok := ts.Redeem("sess-1", tkt)
	assert.True(t, ok, "a ticket inside its TTL is still good")

	ts.now = func() time.Time { return base }
	tkt, err = ts.Issue("sess-1", 1)
	require.NoError(t, err)
	ts.now = func() time.Time { return base.Add(31 * time.Second) }
	_, ok = ts.Redeem("sess-1", tkt)
	assert.False(t, ok, "a ticket past its TTL is rejected")
}

// A ticket is bound to one session: presenting session A's ticket on session B
// fails, so a ticket is never a general credential.
func TestSSETicketIsSessionBound(t *testing.T) {
	ts := newSSETicketStore(sseTicketTTL)

	a, err := ts.Issue("sess-a", 1)
	require.NoError(t, err)

	_, ok := ts.Redeem("sess-b", a)
	assert.False(t, ok, "a foreign session's ticket must be rejected")

	owner, ok := ts.Redeem("sess-a", a)
	require.True(t, ok, "the rejected foreign attempt must not consume the ticket")
	assert.Equal(t, int64(1), owner)
}

// Nothing else passes: an empty or invented value is not a ticket.
func TestSSETicketRejectsGarbage(t *testing.T) {
	ts := newSSETicketStore(sseTicketTTL)
	_, err := ts.Issue("sess-1", 1)
	require.NoError(t, err)

	_, ok := ts.Redeem("sess-1", "")
	assert.False(t, ok)
	_, ok = ts.Redeem("sess-1", "not-a-ticket")
	assert.False(t, ok)
	_, ok = ts.Redeem("gone", "not-a-ticket")
	assert.False(t, ok)
}

// Closing a session drops its tickets, so none outlives the stream it names.
func TestSSETicketForgetDropsSessionTickets(t *testing.T) {
	ts := newSSETicketStore(sseTicketTTL)
	tkt, err := ts.Issue("sess-1", 1)
	require.NoError(t, err)

	ts.Forget("sess-1")
	_, ok := ts.Redeem("sess-1", tkt)
	assert.False(t, ok)
}

// A client stuck reconnecting must not grow the live set without bound.
func TestSSETicketsPerSessionAreCapped(t *testing.T) {
	ts := newSSETicketStore(sseTicketTTL)
	var first string
	for i := 0; i < maxTicketsPerSession*3; i++ {
		tkt, err := ts.Issue("sess-1", 1)
		require.NoError(t, err)
		if i == 0 {
			first = tkt
		}
	}
	ts.mu.Lock()
	live := len(ts.bySession["sess-1"])
	ts.mu.Unlock()
	assert.LessOrEqual(t, live, maxTicketsPerSession)

	_, ok := ts.Redeem("sess-1", first)
	assert.False(t, ok, "the oldest ticket is dropped once the cap is hit")
}
