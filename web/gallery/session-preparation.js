// Own an unused session until Send claims it. Invalidating also disposes late
// responses while the page lives; the server deadline covers destroyed pages.
export function createSessionPreparation({ start, close, expireAfter = 90_000,
  schedule = setTimeout, cancel = clearTimeout }) {
  let generation = 0;
  let pending = null;
  let prepared = null;
  let expiry = null;
  let claimed = false;

  function clearExpiry() {
    if (expiry !== null) cancel(expiry);
    expiry = null;
  }

  function discard() {
    generation++;
    clearExpiry();
    const previous = prepared;
    prepared = null;
    pending = null;
    claimed = false;
    if (previous) close(previous);
  }

  function prepare() {
    if (prepared) return Promise.resolve(prepared);
    if (pending) return pending;
    const startedAt = generation;
    const request = Promise.resolve().then(() => startedAt === generation ? start() : null).then((session) => {
      if (!session) return null;
      if (startedAt !== generation) {
        close(session);
        return null;
      }
      prepared = session;
      if (!claimed) expiry = schedule(discard, expireAfter);
      return session;
    });
    pending = request;
    // Clear only this request: a discarded startup may finish after a new one.
    request.then(() => {
      if (pending === request) pending = null;
    }, () => {
      if (pending === request) pending = null;
    });
    return request;
  }

  async function claim() {
    claimed = true;
    clearExpiry();
    try {
      return await prepare();
    } catch (error) {
      claimed = false;
      throw error;
    }
  }

  // The caller now owns the claimed session and closes it on normal reset.
  function release() {
    if (!claimed || !prepared) return;
    clearExpiry();
    prepared = null;
    pending = null;
    claimed = false;
    generation++;
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
