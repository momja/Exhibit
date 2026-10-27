import { clerkAppearance } from './clerk-appearance.js';

let clientPromise;

const loadTimeout = 15000;

function withTimeout(promise, message) {
  let timer;
  return Promise.race([
    promise,
    new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(message)), loadTimeout); }),
  ]).finally(() => clearTimeout(timer));
}

function loadScript(src, publishableKey) {
  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = src;
    script.async = true;
    script.crossOrigin = 'anonymous';
    if (publishableKey) script.dataset.clerkPublishableKey = publishableKey;
    script.onload = resolve;
    script.onerror = () => reject(new Error('Account service unavailable. Please reload to try again.'));
    document.head.append(script);
  });
}

// Ordinary pages need Clerk for two things the first paint does not: keeping
// the minute-long session cookie fresh, and drawing the account button. Its
// two scripts are close to half a megabyte of JavaScript to compile, plus
// round trips to the Frontend API, and on a phone that work competed with the
// page and its widget frames during launch (Exh-g0fx). So those pages start
// Clerk once the page has loaded and the main thread is idle. The login page,
// where Clerk *is* the page, still calls clerkClient directly.
export function clerkClientAfterLoad() {
  return pageSettled().then(clerkClient);
}

// A widget frame that never finishes loading would hold the load event, and
// the session cookie with it, so waiting for it is capped.
const settleCap = 5000;
// Clerk needs this long, at most, to start and renew a token.
const renewalMargin = 15000;

function pageSettled() {
  // The wait must not outlive the token Clerk is there to renew: a page
  // admitted with seconds left would otherwise lose its session to the next
  // request it makes, and reload into the login flow.
  const cap = Math.min(settleCap, msUntilSessionExpires() - renewalMargin);
  return new Promise(resolve => {
    if (cap <= 0) return resolve();
    const timer = setTimeout(done, cap);
    // A page put away mid-wait — a background tab, the back/forward cache —
    // has its timers paused while its token keeps ageing, so coming back is
    // the moment to stop waiting.
    const onVisible = () => { if (document.visibilityState === 'visible') done(); };
    const onShow = event => { if (event.persisted) done(); };
    const whenIdle = () => (window.requestIdleCallback
      ? window.requestIdleCallback(done, { timeout: 2000 })
      : setTimeout(done, 0));
    function done() {
      clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('pageshow', onShow);
      window.removeEventListener('load', whenIdle);
      resolve();
    }
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('pageshow', onShow);
    if (document.readyState === 'complete') whenIdle();
    else window.addEventListener('load', whenIdle, { once: true });
  });
}

// The server says how long the token had left when it rendered the page, on
// the clock the token is checked against; time since then is measured on the
// page's monotonic clock, which starts before the response arrived and so
// errs early. The device's wall clock, which may be off by any amount, is
// never consulted. No figure counts as already expired.
function msUntilSessionExpires() {
  const left = Number(document.querySelector('meta[name="clerk-session-expires-in"]')?.content);
  return Number.isFinite(left) ? left - window.performance.now() : 0;
}

export function clerkClient() {
  if (!clientPromise) clientPromise = withTimeout(loadClient(),
    'The account service did not respond. Reload this page to retry. If this continues, check whether your browser or network is blocking Clerk.');
  return clientPromise;
}

async function loadClient() {
  const publishableKey = document.querySelector('meta[name="clerk-publishable-key"]')?.content;
  const frontend = document.querySelector('meta[name="clerk-frontend-api"]')?.content;
  if (!publishableKey || !frontend) throw new Error('Account service is not configured.');
  await Promise.all([
    loadScript(`${frontend}/npm/@clerk/ui@1.41.2/dist/ui.browser.js`),
    loadScript(`${frontend}/npm/@clerk/clerk-js@6.40.1/dist/clerk.browser.js`, publishableKey),
  ]);
  await window.Clerk.load({
    ui: { ClerkUI: window.__internal_ClerkUICtor },
    appearance: clerkAppearance,
    signInUrl: '/auth/login',
    signUpUrl: '/auth/signup',
    afterSignOutUrl: '/auth/login',
  });
  return window.Clerk;
}
