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
    const timer = setTimeout(resolve, cap);
    const done = () => { clearTimeout(timer); resolve(); };
    const whenIdle = () => (window.requestIdleCallback
      ? window.requestIdleCallback(done, { timeout: 2000 })
      : setTimeout(done, 0));
    if (document.readyState === 'complete') whenIdle();
    else window.addEventListener('load', whenIdle, { once: true });
  });
}

// The session cookie is Clerk's JWT and readable by design; only its expiry is
// read here, and anything unreadable counts as already expired.
function msUntilSessionExpires() {
  const cookie = document.cookie.split('; ').find(c => c.startsWith('__session='));
  try {
    const payload = cookie.slice('__session='.length).split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(atob(payload)).exp * 1000 - Date.now();
  } catch {
    return 0;
  }
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
