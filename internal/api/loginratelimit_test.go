package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Login rate limiting (av-t21v). The properties held here are the ones the
// throttle would be worth nothing without: that it slows an attacker, that it
// never disables an identity, that one person's failures are not another's
// problem, and that the map it keeps cannot itself be filled up.

// testClock is the injected time these tests advance instead of sleeping —
// the refill intervals are seconds to minutes and a suite that waited them out
// would be unrunnable.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// newThrottledLoginRouter is the local-login router with its limiter on a
// clock the test owns.
func newThrottledLoginRouter(t *testing.T) (*Router, *testClock) {
	t.Helper()
	ro, _ := newLocalLoginRouter(t)
	clock := newTestClock()
	ro.logins = newLoginLimiter(clock.now)
	return ro, clock
}

// loginFrom posts the login form from a named source address.
func loginFrom(t *testing.T, ro *Router, remoteAddr, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	req := httptest.NewRequest("POST", "/auth/local", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

const (
	attackerAddr = "198.51.100.7:33333"
	curatorAddr  = "203.0.113.10:5000"
)

// --- the throttle ------------------------------------------------------

// The point of the feature: guessing gets slower. And the shape of the answer
// matters as much as the fact of it — a bounded wait that the clock ends, not
// an account somebody else disabled.
func TestRepeatedFailuresAreThrottled(t *testing.T) {
	ro, clock := newThrottledLoginRouter(t)

	for i := 0; i < int(perUsernameFailures.burst); i++ {
		w := loginFrom(t, ro, attackerAddr, testUsername, "not-the-passphrase")
		require.Equal(t, http.StatusUnauthorized, w.Code, "attempt %d is a normal rejection", i+1)
	}

	w := loginFrom(t, ro, attackerAddr, testUsername, "not-the-passphrase")
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "30", w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), "Too many sign-in attempts")

	// A throttled attempt is refused before the credential is looked at, so
	// the right password does not walk through it either. That is the cost of
	// the protection and it is why the wait has to stay short.
	w = loginFrom(t, ro, attackerAddr, testUsername, testPassword)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Nil(t, cookiesFrom(w)[sessionCookieName], "no session is issued while throttled")

	// The identity was never disabled: one refill later it is usable again,
	// with no operator action and nothing to un-lock.
	clock.advance(perUsernameFailures.refill)
	w = loginFrom(t, ro, attackerAddr, testUsername, testPassword)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	assert.NotNil(t, cookiesFrom(w)[sessionCookieName])
}

// The balance the feature is judged on. Somebody who fumbles their password a
// few times must not notice this exists.
func TestAMistypedPasswordCostsTheUserNothing(t *testing.T) {
	ro, _ := newThrottledLoginRouter(t)

	for i := 0; i < 2; i++ {
		require.Equal(t, http.StatusUnauthorized,
			loginFrom(t, ro, curatorAddr, testUsername, "typo").Code)
	}

	w := loginFrom(t, ro, curatorAddr, testUsername, testPassword)
	require.Equal(t, http.StatusFound, w.Code, "the third, correct attempt is not delayed")
	require.NotNil(t, cookiesFrom(w)[sessionCookieName])

	// And they are left as they started: signing in returns the username's
	// budget, so the two typos are not still on the account tomorrow.
	assert.Zero(t, ro.logins.user.retryAfter(usernameKey(testUsername)))
	assert.Equal(t, 0, ro.logins.user.size(), "nothing is remembered about a name that signed in")
}

// Per-username limiting alone would hand an attacker a lockout primitive
// against anyone whose name they can guess. The source key is the half that
// makes that expensive; this holds the other half of the bargain — that a
// name under attack does not cost anybody else anything.
func TestOneAccountUnderAttackDoesNotAffectAnother(t *testing.T) {
	ro, _ := newThrottledLoginRouter(t)

	for i := 0; i < int(perUsernameFailures.burst); i++ {
		require.Equal(t, http.StatusUnauthorized,
			loginFrom(t, ro, attackerAddr, "victim", "guess").Code)
	}
	require.NotZero(t, ro.logins.user.retryAfter(usernameKey("victim")),
		"the attacked name is out of budget")

	w := loginFrom(t, ro, curatorAddr, testUsername, testPassword)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	assert.NotNil(t, cookiesFrom(w)[sessionCookieName])
}

// The failure a global counter would have: one source, however determined,
// must not be able to shut the front door on everyone else. Deliberately no
// instance-wide budget exists, and this is what says so.
func TestOneSourceCannotLockOutTheInstance(t *testing.T) {
	ro, _ := newThrottledLoginRouter(t)

	// Distinct names each attempt, so it is the source budget that runs out
	// rather than any one account's.
	for i := 0; i < int(perIPFailures.burst); i++ {
		require.Equal(t, http.StatusUnauthorized,
			loginFrom(t, ro, attackerAddr, fmt.Sprintf("guess-%d", i), "password").Code)
	}
	require.Equal(t, http.StatusTooManyRequests,
		loginFrom(t, ro, attackerAddr, "guess-again", "password").Code)

	// Another address, arriving in the middle of all that, is unaffected.
	w := loginFrom(t, ro, curatorAddr, testUsername, testPassword)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	assert.NotNil(t, cookiesFrom(w)[sessionCookieName])
}

// --- what counts as a failure ------------------------------------------

// stubLogin drives the middleware over a handler that just answers a status,
// which is all the middleware reads from it — and which keeps these cases off
// bcrypt.
func stubLogin(t *testing.T, ro *Router, status int, username string) *httptest.ResponseRecorder {
	t.Helper()
	h := ro.loginRateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	form := url.Values{"username": {username}, "password": {"whatever"}}
	req := httptest.NewRequest("POST", "/auth/local", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = attackerAddr
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func newStubRouter(clock *testClock) *Router {
	return &Router{logins: newLoginLimiter(clock.now)}
}

// Only a rejected credential is a guess. A server that failed to create a
// session has charged the user for our fault, and a request nobody could parse
// is not an attempt at a password.
func TestOnlyARejectedCredentialIsCharged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		charged bool
	}{
		{"rejected credential", http.StatusUnauthorized, true},
		{"signed in", http.StatusFound, false},
		{"server could not start a session", http.StatusInternalServerError, false},
		{"handler rejected the request", http.StatusBadRequest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ro := newStubRouter(newTestClock())
			stubLogin(t, ro, tc.status, "someone")
			assert.Equal(t, tc.charged, ro.logins.ip.size() > 0)
		})
	}
}

// Signing in returns the *name's* budget, not the source's. Refunding the
// source too would let anyone holding one valid credential top up between
// guesses at somebody else's.
func TestSuccessRefundsTheUsernameNotTheSource(t *testing.T) {
	ro := newStubRouter(newTestClock())

	stubLogin(t, ro, http.StatusUnauthorized, "someone")
	stubLogin(t, ro, http.StatusUnauthorized, "someone")
	require.Equal(t, 1, ro.logins.user.size())
	require.Equal(t, 1, ro.logins.ip.size())

	stubLogin(t, ro, http.StatusFound, "someone")
	assert.Equal(t, 0, ro.logins.user.size(), "the name it proved ownership of is forgiven")
	assert.Equal(t, 1, ro.logins.ip.size(), "the source it came from is not")
}

// The handler used to answer an unparseable form itself. Reading the username
// moved that parse into the middleware, so the middleware has to keep the
// answer — and must not charge for it.
func TestUnreadableFormIsRejectedWithoutBeingCharged(t *testing.T) {
	ro := newStubRouter(newTestClock())
	h := ro.loginRateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("the handler must not be reached")
	}))
	req := httptest.NewRequest("POST", "/auth/local", strings.NewReader("username=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = attackerAddr
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Could not read that form")
	assert.Equal(t, 0, ro.logins.ip.size())
}

// --- bounded memory ----------------------------------------------------

// The failure mode a naive limiter has: both keys are attacker-controlled, so
// a map that only grows turns the throttle into the denial of service it was
// meant to prevent.
func TestMemoryStaysBoundedUnderManyDistinctKeys(t *testing.T) {
	const capacity = 64
	clock := newTestClock()
	l := newKeyedLimiter(perUsernameFailures, capacity, clock.now)

	const total = 100_000
	for i := 0; i < total; i++ {
		key := fmt.Sprintf("attacker-%d", i)
		l.retryAfter(key)
		l.penalise(key)
		// Sampled, not every iteration: the bound holds throughout, but
		// asserting 100,000 times buys nothing a sparser check doesn't.
		if i%1000 == 0 {
			require.LessOrEqual(t, l.size(), 2*capacity, "after %d distinct keys", i+1)
		}
	}
	require.LessOrEqual(t, l.size(), 2*capacity, "after %d distinct keys", total)

	// Eviction drops the oldest generation, not the traffic in front of the
	// limiter: a key that is being used right now is still counted.
	for i := 0; i < int(perUsernameFailures.burst); i++ {
		l.penalise("still-here")
	}
	assert.NotZero(t, l.retryAfter("still-here"))
}

// A lookup alone must not allocate an entry, or a flood of requests would grow
// the map without a single failed password among them.
func TestLookupsDoNotAllocateEntries(t *testing.T) {
	l := newKeyedLimiter(perUsernameFailures, 64, newTestClock().now)
	for i := 0; i < 1000; i++ {
		assert.Zero(t, l.retryAfter(fmt.Sprintf("never-failed-%d", i)))
	}
	assert.Equal(t, 0, l.size())
}

// Idle keys are reclaimed without waiting for the generations to rotate, which
// is what keeps an honest instance's map empty rather than merely bounded.
func TestFullyRefilledKeysAreForgotten(t *testing.T) {
	clock := newTestClock()
	l := newKeyedLimiter(perUsernameFailures, 64, clock.now)

	l.penalise("someone")
	l.penalise("someone")
	require.Equal(t, 1, l.size())

	clock.advance(perUsernameFailures.refill)
	assert.Zero(t, l.retryAfter("someone"))
	assert.Equal(t, 1, l.size(), "one token back is not a clean slate")

	clock.advance(perUsernameFailures.refill * 2)
	assert.Zero(t, l.retryAfter("someone"))
	assert.Equal(t, 0, l.size(), "back to full: there is nothing left to remember")
}

// --- the budget arithmetic ---------------------------------------------

func TestBudgetRefillsWithTheClock(t *testing.T) {
	clock := newTestClock()
	l := newKeyedLimiter(rateLimit{burst: 2, refill: 10 * time.Second}, 8, clock.now)

	l.penalise("k")
	require.Zero(t, l.retryAfter("k"), "one of two spent")
	l.penalise("k")
	assert.InDelta(t, float64(10*time.Second), float64(l.retryAfter("k")), float64(time.Millisecond))

	clock.advance(4 * time.Second)
	assert.InDelta(t, float64(6*time.Second), float64(l.retryAfter("k")), float64(time.Millisecond))

	clock.advance(6 * time.Second)
	assert.Zero(t, l.retryAfter("k"))
}

// Nothing accumulates past the burst, so a key idle for a week does not arrive
// with a week of attempts saved up.
func TestBudgetDoesNotAccumulatePastTheBurst(t *testing.T) {
	clock := newTestClock()
	l := newKeyedLimiter(rateLimit{burst: 2, refill: 10 * time.Second}, 8, clock.now)

	l.penalise("k")
	clock.advance(7 * 24 * time.Hour)
	l.penalise("k")
	l.penalise("k")
	assert.NotZero(t, l.retryAfter("k"), "still exactly two failures' worth, no more")
}

func TestHumanSeconds(t *testing.T) {
	assert.Equal(t, "a second", humanSeconds(1))
	assert.Equal(t, "30 seconds", humanSeconds(30))
	assert.Equal(t, "a minute", humanSeconds(60))
	assert.Equal(t, "2 minutes", humanSeconds(61))
}

// --- the keys ----------------------------------------------------------

// The peer address is the only value a client cannot forge, so it wins by
// default; a forwarded one is read only when the peer looks like the operator's
// own proxy, and only from the end the proxy appends to.
func TestClientIPKeying(t *testing.T) {
	for _, tc := range []struct {
		name, remote, forwarded, want string
	}{
		{"direct", "203.0.113.9:4000", "", "203.0.113.9"},
		{"a header off the open internet is ignored", "203.0.113.9:4000", "10.0.0.1", "203.0.113.9"},
		{"behind a loopback proxy", "127.0.0.1:8080", "198.51.100.4", "198.51.100.4"},
		{"behind a private-network proxy", "10.1.2.3:40000", "198.51.100.4", "198.51.100.4"},
		{"a spoofed prefix cannot move the key", "127.0.0.1:8080", "1.1.1.1, 198.51.100.4", "198.51.100.4"},
		{"unparseable header falls back to the peer", "127.0.0.1:8080", "not-an-ip", "127.0.0.1"},
		{"empty header falls back to the peer", "127.0.0.1:8080", "", "127.0.0.1"},
		{"ipv6 peer", "[2001:db8::1]:443", "", "2001:db8::1"},
		{"ipv6 forwarded", "[::1]:443", "2001:db8::2", "2001:db8::2"},
		{"forwarded entry carrying a port", "127.0.0.1:8080", "198.51.100.4:51000", "198.51.100.4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/auth/local", nil)
			req.RemoteAddr = tc.remote
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			assert.Equal(t, tc.want, clientIP(req))
		})
	}
}

// Case is folded so that capitalising the name being guessed is not a way to
// buy another burst against it, and the key is bounded so a huge form field
// cannot become a huge retained map key.
func TestUsernameKey(t *testing.T) {
	assert.Equal(t, usernameKey("curator"), usernameKey("Curator"))
	assert.Equal(t, usernameKey("curator"), usernameKey("  CURATOR  "))
	assert.NotEqual(t, usernameKey("curator"), usernameKey("someone-else"))
	assert.Len(t, usernameKey(strings.Repeat("a", 5000)), usernameKeyMax)
}
