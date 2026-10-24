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
