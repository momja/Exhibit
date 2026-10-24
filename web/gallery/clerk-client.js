let clientPromise;

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
  if (!clientPromise) clientPromise = loadClient();
  return clientPromise;
}

async function loadClient() {
  const publishableKey = document.querySelector('meta[name="clerk-publishable-key"]')?.content;
  const frontend = document.querySelector('meta[name="clerk-frontend-api"]')?.content;
  if (!publishableKey || !frontend) throw new Error('Account service is not configured.');
  await Promise.all([
    loadScript(`${frontend}/npm/@clerk/ui@1/dist/ui.browser.js`),
    loadScript(`${frontend}/npm/@clerk/clerk-js@6/dist/clerk.browser.js`, publishableKey),
  ]);
  await window.Clerk.load({
    ui: { ClerkUI: window.__internal_ClerkUICtor },
    signInUrl: '/auth/login',
    signUpUrl: '/auth/signup',
    afterSignOutUrl: '/auth/login',
  });
  return window.Clerk;
}
