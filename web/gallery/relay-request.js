// Only Exhibit throttle refusals of GET/HEAD are retried here. Other methods,
// provider responses, quota failures and permission changes retain their
// normal observable result.
export async function requestRelay(send, method, signal) {
  const deadline = Date.now() + 60000;
  const read = ['GET', 'HEAD'].includes(method.toUpperCase());
  for (;;) {
    signal.throwIfAborted();
    const response = await send();
    const result = await response.json();
    const retryAfter = response.headers.get('Retry-After');
    if (!read || response.status !== 429 || result.code !== 'relay_rate_limited' ||
        !retryAfter || !/^[1-9]\d*$/.test(retryAfter)) return { response, result };
    const delay = Number(retryAfter) * 1000;
    if (!Number.isFinite(delay) || Date.now() + delay >= deadline) return { response, result };
    await waitForRetry(delay, signal);
    if (Date.now() >= deadline) return { response, result };
  }
}

function waitForRetry(delay, signal) {
  return new Promise((resolve, reject) => {
    const finish = callback => {
      clearTimeout(timer);
      signal.removeEventListener('abort', abort);
      callback();
    };
    const abort = () => finish(() => reject(signal.reason));
    const timer = setTimeout(() => finish(resolve), delay);
    signal.addEventListener('abort', abort, { once: true });
    if (signal.aborted) abort();
  });
}
