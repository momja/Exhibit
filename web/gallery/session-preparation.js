// Own an unused session until Send claims it. The fixed startup deadline also
// bounds Send while the POST is pending; late responses are still disposed.
export function createSessionPreparation({ start, close, expireAfter = 90_000,
  schedule = setTimeout, cancel = clearTimeout, now = Date.now }) {
  let current = null;

  function clearExpiry(attempt) {
    if (attempt.expiry !== null) cancel(attempt.expiry);
    attempt.expiry = null;
  }

  function expire(attempt) {
    if (current !== attempt) return;
    current = null;
    clearExpiry(attempt);
    if (attempt.session) close(attempt.session);
    else attempt.reject(new Error('Agent session startup timed out. Send again to retry.'));
  }

  function discard() {
    const previous = current;
    if (!previous) return;
    current = null;
    clearExpiry(previous);
    // Unblock a waiting Send immediately, even if the POST never completes.
    previous.resolve(null);
    if (previous.session) close(previous.session);
  }

  function prepare() {
    if (current && !(current.claimed && current.session) && now() >= current.deadline) expire(current);
    if (current) return current.promise;
    const attempt = { session: null, claimed: false, expiry: null,
      deadline: now() + expireAfter, resolve: null, reject: null, promise: null };
    attempt.promise = new Promise((resolve, reject) => {
      attempt.resolve = resolve;
      attempt.reject = reject;
    });
    current = attempt;
    attempt.expiry = schedule(() => expire(attempt), expireAfter);
    Promise.resolve().then(() => current === attempt ? start() : null).then((session) => {
      // A delayed response must not beat an overdue timer callback and claim
      // a session whose server preparation deadline has already elapsed.
      if (current === attempt && now() >= attempt.deadline) expire(attempt);
      if (current !== attempt) {
        if (session) close(session);
        return;
      }
      if (!session) {
        discard();
        return;
      }
      attempt.session = session;
      if (attempt.claimed) clearExpiry(attempt);
      attempt.resolve(session);
    }, (error) => {
      if (current === attempt) {
        current = null;
        clearExpiry(attempt);
      }
      attempt.reject(error);
    });
    return attempt.promise;
  }

  function claim() {
    const promise = prepare();
    current.claimed = true;
    // A pending claim still needs its startup deadline. Only a ready session
    // can move into normal caller-owned lifetime.
    if (current.session) clearExpiry(current);
    return promise;
  }

  // The caller now owns the claimed session and closes it on normal reset.
  function release() {
    if (!current?.claimed || !current.session) return;
    clearExpiry(current);
    current = null;
  }

  return { prepare, claim, discard, release };
}

// A short, sustained draft signals intent without sending its text anywhere.
export function createTypingPreparation({ prepare, ready, delay = 300,
  minimumCharacters = 3, schedule = setTimeout, cancel = clearTimeout }) {
  let timer = null;
  function reset() {
    if (timer !== null) cancel(timer);
    timer = null;
  }
  function update(value) {
    reset();
    if (!ready() || [...value.replace(/\s/gu, '')].length < minimumCharacters) return;
    timer = schedule(() => {
      timer = null;
      if (ready()) Promise.resolve().then(prepare).catch(() => {});
    }, delay);
  }
  return { update, reset };
}
