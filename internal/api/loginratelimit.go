package api

import (
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Login rate limiting (av-t21v).
//
// Until this existed, bcrypt's cost *was* the whole of the throttle: a guess
// cost the attacker roughly the tens of milliseconds it cost the server, and an
// instance with exactly one credential could live with that. Issuing
// credentials for several people (av-sz4e) changes the shape of the attack —
// credential stuffing sprays one likely password across N accounts, so the
// per-guess cost that made one credential safe buys almost nothing — and it
// weakens the answer, because "put a limit at your proxy" is a thinner reply
// when we are the thing minting the credentials rather than delegating them.
//
// # What is limited, and what is not
//
// Only *failed* attempts are debited, and the check runs before the handler, so
// a person who signs in correctly pays nothing and is never delayed by other
// people's failures. Nothing here disables an account: a bucket that empties
// refills on a clock, so the worst an attacker can inflict on a real user is a
// bounded wait, never a lockout somebody else triggered on their behalf. That
// is the whole reason this is a token bucket rather than a failed-attempt
// counter with a threshold — a counter that disables an identity hands every
// attacker a denial of service against any user whose name they can guess.
//
// # Two keys, checked independently
//
// Neither key works alone, so an attempt must have budget under both:
//
//   - **Source address.** The precise key, and the only one that sees a spray
//     across many accounts — per-username budgets never notice one guess each.
//     It is also the one that protects the server's CPU, since every attempt
//     costs a bcrypt comparison. Its weakness is collateral: a household or
//     office behind one NAT shares it, and behind a reverse proxy (the
//     documented deployment) it may be the proxy's address for everyone. So its
//     budget is the generous one.
//   - **Username.** Survives a botnet, because rotating source addresses does
//     not rotate the account being guessed. Its weakness is the mirror image:
//     the collateral lands on one named person, which is why the refill is
//     brisk enough that an attacker occupying someone's bucket costs them a
//     wait measured in seconds and never an account they cannot use.
//
// Deliberately absent: a global budget across all logins. It would be the one
// key an attacker could use to lock out the entire instance, which is a worse
// failure than the brute force it would slow.
//
// The residual gap is a distributed spray — many addresses, many accounts, one
// guess each — which no in-process limiter can see. That is where the proxy or
// fail2ban advice in docs/security.md §1.4 still belongs, now as the
// complement to this rather than as the only answer.
var (
	// perIPFailures: 20 failures at once, then one more every 3s (20/min).
	// Sized so a shared address — several people behind one NAT or one proxy,
	// each fumbling a password — never reaches it, while a single source
	// hammering the endpoint drops from what bcrypt allows (hundreds a minute,
	// more with concurrency) to twenty.
	perIPFailures = rateLimit{burst: 20, refill: 3 * time.Second}
	// perUsernameFailures: 10 failures at once, then one more every 30s
	// (2/min). The burst is what makes a legitimate mistyper unaffected — five
	// consecutive typos still leave budget — and the refill is what keeps the
	// worst case for a user under attack at a 30-second wait rather than a
	// lockout.
	perUsernameFailures = rateLimit{burst: 10, refill: 30 * time.Second}
)

// limiterCapacity bounds each limiter's live key count. Memory is the attack
// surface a naive limiter opens: both keys are attacker-controlled, so a map
// that only ever grows is itself the denial of service. See keyedLimiter.keep
// for how the bound is enforced — the ceiling is 2×this per limiter, and an
// entry costs a small struct and its key.
const limiterCapacity = 4096

// usernameKeyMax truncates the key taken from a submitted username. Nobody's
// account name is this long, so the only thing it costs is that two absurd
// usernames may share a bucket; what it buys is that a megabyte of form field
// cannot become a megabyte of retained map key.
const usernameKeyMax = 128

// rateLimit is a budget: how many failures may land at once, and how long one
// costs to earn back.
type rateLimit struct {
	burst  float64
	refill time.Duration
}

// bucket is one key's remaining budget, refilled lazily from the clock rather
// than by a background sweeper — a timer per key would be its own memory
// problem, and nothing needs the count to be current except a lookup.
type bucket struct {
	tokens float64
	last   time.Time
}

func (b *bucket) refill(limit rateLimit, now time.Time) {
	elapsed := now.Sub(b.last)
	if elapsed <= 0 {
		return
	}
	b.tokens = math.Min(limit.burst, b.tokens+float64(elapsed)/float64(limit.refill))
	b.last = now
}

// keyedLimiter is a bounded set of buckets.
//
// The bound is a two-generation scheme rather than an LRU: when the current
// generation fills, it becomes the previous one and a fresh map takes over, so
// the previous generation is dropped whole on the next rotation. That costs
// nothing per lookup (no list to splice, no heap to sift), holds at most
// 2×capacity entries, and its one imprecision — an unlucky key can be forgotten
// while still active — is acceptable because forgetting a key only ever grants
// budget that was going to refill anyway.
//
// Idle keys are reclaimed without waiting for a rotation: a bucket that has
// refilled to full carries no information and is deleted on the next lookup, so
// the honest traffic that dominates a real instance leaves nothing behind.
type keyedLimiter struct {
	limit    rateLimit
	capacity int
	// now is the clock, injected so tests can advance time instead of
	// sleeping through a 30-second refill.
	now func() time.Time

	mu   sync.Mutex
	cur  map[string]*bucket
	prev map[string]*bucket
}

func newKeyedLimiter(limit rateLimit, capacity int, now func() time.Time) *keyedLimiter {
	return &keyedLimiter{
		limit:    limit,
		capacity: capacity,
		now:      now,
		cur:      make(map[string]*bucket),
	}
}

// retryAfter reports how long key must wait for its next attempt, or zero when
// it may proceed now.
//
// It deliberately does not create a bucket. Only a failure does that, which is
// what keeps the map's growth tied to failures rather than to requests — and
// since a failure also debits the source address, one attacker cannot fill the
// username map faster than their own address budget allows.
func (l *keyedLimiter) retryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.lookup(key)
	if b == nil {
		return 0
	}
	b.refill(l.limit, l.now())
	if b.tokens >= l.limit.burst {
		// Fully recovered: indistinguishable from never having been seen.
		l.drop(key)
		return 0
	}
	if b.tokens >= 1 {
		return 0
	}
	return time.Duration((1 - b.tokens) * float64(l.limit.refill))
}

// penalise charges key for one failed attempt.
func (l *keyedLimiter) penalise(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b := l.lookup(key)
	if b == nil {
		b = &bucket{tokens: l.limit.burst, last: now}
	} else {
		b.refill(l.limit, now)
	}
	if b.tokens >= 1 {
		b.tokens--
	} else {
		b.tokens = 0
	}
	l.keep(key, b)
}

// forget returns key to full budget. It is what a successful login does to the
// *username* it proved ownership of, so a person who mistypes twice and then
// signs in is left exactly as they started.
func (l *keyedLimiter) forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.drop(key)
}

// lookup finds key in either generation. Callers hold the mutex.
func (l *keyedLimiter) lookup(key string) *bucket {
	if b, ok := l.cur[key]; ok {
		return b
	}
	if b, ok := l.prev[key]; ok {
		return b
	}
	return nil
}

// keep files key in the current generation, rotating when it is full. Callers
// hold the mutex.
func (l *keyedLimiter) keep(key string, b *bucket) {
	if _, ok := l.cur[key]; ok {
		return
	}
	if len(l.cur) >= l.capacity {
		l.prev = l.cur
		l.cur = make(map[string]*bucket, l.capacity)
	}
	delete(l.prev, key)
	l.cur[key] = b
}

// drop removes key from both generations. Callers hold the mutex.
func (l *keyedLimiter) drop(key string) {
	delete(l.cur, key)
	delete(l.prev, key)
}

// size is the live key count, for the test that holds the memory bound.
func (l *keyedLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.cur) + len(l.prev)
}

// loginLimiter is the pair of budgets a login attempt is checked against.
type loginLimiter struct {
	ip   *keyedLimiter
	user *keyedLimiter
}

// newLoginLimiter builds the pair. A nil clock means the real one; tests pass
// their own and replace Router.logins wholesale.
func newLoginLimiter(now func() time.Time) *loginLimiter {
	if now == nil {
		now = time.Now
	}
	return &loginLimiter{
		ip:   newKeyedLimiter(perIPFailures, limiterCapacity, now),
		user: newKeyedLimiter(perUsernameFailures, limiterCapacity, now),
	}
}

// loginRateLimit throttles the local login endpoint.
//
// It is middleware on the route rather than a check inside the handler, and
// that is a boundary as much as a style: the handler owns *whether a credential
// is correct* — today an env-configured hash, tomorrow a row on `users`
// (av-rzvf) — and this owns *how often it may be asked*. Nothing here reads the
// credential, so the two move independently.
//
// The failure signal is the handler's status code, for the same reason: 401 is
// its answer for a credential that did not match, whatever it compared against.
// A 5xx is not charged — a session the server failed to create is our fault,
// not a guess — and a redirect is the success that clears the username's
// budget.
func (ro *Router) loginRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The username is a form field, so the form has to be read here to key
		// on it. ParseForm caches into r.PostForm, so the handler's own call
		// reuses this parse rather than finding a drained body — but it also
		// means the handler can no longer see a parse *error*, so this answers
		// that case itself, with the message the handler would have rendered.
		// An unreadable form is not a password guess and is not charged.
		if err := r.ParseForm(); err != nil {
			ro.renderLogin(w, r, http.StatusBadRequest, "", "", "Could not read that form. Try again.")
			return
		}
		username := r.PostFormValue("username")
		ipKey := clientIP(r)
		userKey := usernameKey(username)

		// Both budgets must allow the attempt; the longer wait is the one
		// reported, since satisfying the shorter one would still be refused.
		wait := max(ro.logins.ip.retryAfter(ipKey), ro.logins.user.retryAfter(userKey))
		if wait > 0 {
			ro.throttleLogin(w, r, wait, username)
			return
		}

		rec := &loginStatus{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		switch {
		case rec.status == http.StatusUnauthorized:
			ro.logins.ip.penalise(ipKey)
			ro.logins.user.penalise(userKey)
		case rec.status >= 300 && rec.status < 400:
			// Signed in. The username's budget is returned because this
			// request proved the identity owns it; the address's is not,
			// because otherwise anyone holding one valid credential could
			// refill their source budget between guesses at somebody else's.
			ro.logins.user.forget(userKey)
		}
	})
}

// throttleLogin answers a refused attempt: the login page again, with the wait
// stated in the form rather than in a status line nobody reads, plus the
// Retry-After a non-browser client needs.
func (ro *Router) throttleLogin(w http.ResponseWriter, r *http.Request, wait time.Duration, username string) {
	seconds := int(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	// The username is not logged, for the same reason a failed login does not
	// log one: it is a credential someone tried, and it may be a password typed
	// into the wrong field.
	slog.Warn("local login throttled",
		slog.String("remote_addr", r.RemoteAddr),
		slog.Int("retry_after_seconds", seconds))
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	ro.renderLogin(w, r, http.StatusTooManyRequests, safeNext(r.PostFormValue("next")), username,
		fmt.Sprintf("Too many sign-in attempts. Try again in %s.", humanSeconds(seconds)))
}

func humanSeconds(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	minutes := (seconds + 59) / 60
	if minutes == 1 {
		return "a minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// loginStatus records the status the login handler answered with, which is the
// only thing this middleware needs from it.
type loginStatus struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *loginStatus) WriteHeader(status int) {
	if !w.wrote {
		w.status = status
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *loginStatus) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// usernameKey is the account half of the key.
//
// Case is folded even though the credential comparison is case-sensitive, and
// the mismatch is deliberate: "Curator" and "curator" cannot both be signed
// into, so letting them hold separate budgets would only hand an attacker a
// fresh burst per capitalisation of the name they are guessing.
func usernameKey(username string) string {
	key := strings.ToLower(strings.TrimSpace(username))
	if len(key) > usernameKeyMax {
		key = key[:usernameKeyMax]
	}
	return key
}

// clientIP names the source of a login attempt.
//
// The peer address is the trustworthy value and the default, because a client
// cannot forge it. It is also the wrong value in the deployment this project
// documents — the app serves plain HTTP behind a proxy the operator supplies
// (technical_stack.md §12), so every request arrives from the proxy and one
// budget would cover the whole instance.
//
// So a forwarded address is consulted, but only when the peer is itself local
// or private, i.e. plausibly that proxy: a request straight off the internet is
// keyed on where it actually came from and no header can move it. The
// *rightmost* entry is taken, because that is the hop the nearest proxy
// appended; the leftmost entries are whatever the client sent and are exactly
// what an attacker would spoof to get a fresh budget per request.
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil || !isLocalPeer(ip) {
		return host
	}
	if fwd := rightmostForwarded(r.Header.Get("X-Forwarded-For")); fwd != "" {
		return fwd
	}
	return host
}

func isLocalPeer(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// rightmostForwarded returns the last parseable address in an X-Forwarded-For
// header, or "" when there is none.
func rightmostForwarded(header string) string {
	parts := strings.Split(header, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		if h, _, err := net.SplitHostPort(candidate); err == nil {
			candidate = h
		}
		if ip := net.ParseIP(strings.Trim(candidate, "[]")); ip != nil {
			return ip.String()
		}
	}
	return ""
}
