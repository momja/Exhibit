import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = (await readFile(new URL('./clerk-controller.js', import.meta.url), 'utf8'))
  .replace("import { Controller } from '../stimulus/stimulus.js';", 'const Controller = class {};')
  .replace("import { clerkClient, clerkClientAfterLoad } from './clerk-client.js';", 'const clerkClient = async () => globalThis.testClerk; const clerkClientAfterLoad = clerkClient;')
  .replace("import { clerkUserButtonOptions } from './clerk-menu.js';", 'const clerkUserButtonOptions = () => ({});')
  .replace("import { clerkAuthAppearance } from './clerk-appearance.js';", 'const clerkAuthAppearance = {};');

for (const failure of ['denied', 'network']) {
  test(`hosted login offers explicit recovery after ${failure} without signing out automatically`, async t => {
    let signOuts = 0, redirects = 0, reloads = 0;
    const old = { fetch: globalThis.fetch, window: globalThis.window, clerk: globalThis.testClerk };
    globalThis.testClerk = { isSignedIn: true, session: { getToken: async () => {} }, signOut: async () => { signOuts++; } };
    globalThis.fetch = async () => { if (failure === 'network') throw new Error('Network unavailable'); return { ok: false, status: 403 }; };
    globalThis.window = { location: { replace: () => { redirects++; }, reload: () => { reloads++; } } };
    t.after(() => { globalThis.fetch = old.fetch; globalThis.window = old.window; globalThis.testClerk = old.clerk; });
    const { default: Controller } = await import(`data:text/javascript,${encodeURIComponent(source)}#${failure}`);
    const controller = new Controller();
    Object.assign(controller, { element: { isConnected: true }, modeValue: 'signin', nextValue: '/', statusTarget: { textContent: '' }, hasRecoveryTarget: true, recoveryTarget: { hidden: true } });
    await controller.connect();
    assert.equal(controller.recoveryTarget.hidden, false);
    assert.ok(controller.statusTarget.textContent.length > 0);
    assert.equal(signOuts, 0);
    assert.equal(redirects, 0);
    controller.retry();
    assert.equal(reloads, 1);
  });
}

// Exh-g0fx: the account button waits for the page; login pages do not.
test('only the account button defers loading Clerk', async () => {
  const calls = [];
  const deferred = (await readFile(new URL('./clerk-controller.js', import.meta.url), 'utf8'))
    .replace("import { Controller } from '../stimulus/stimulus.js';", 'const Controller = class {};')
    .replace("import { clerkClient, clerkClientAfterLoad } from './clerk-client.js';",
      "const clerkClient = async () => { globalThis.clerkCalls.push('now'); return globalThis.testClerk; }; const clerkClientAfterLoad = async () => { globalThis.clerkCalls.push('after-load'); return globalThis.testClerk; };")
    .replace("import { clerkUserButtonOptions } from './clerk-menu.js';", 'const clerkUserButtonOptions = () => ({});')
    .replace("import { clerkAuthAppearance } from './clerk-appearance.js';", 'const clerkAuthAppearance = {};');
  globalThis.clerkCalls = calls;
  globalThis.testClerk = { isSignedIn: false, mountSignIn: () => {} };
  const { default: Controller } = await import(`data:text/javascript,${encodeURIComponent(deferred)}#modes`);
  for (const mode of ['user', 'signin']) {
    const controller = new Controller();
    Object.assign(controller, { element: { isConnected: true }, modeValue: mode, nextValue: '/', statusTarget: { textContent: '' }, hasRecoveryTarget: false, mountTarget: {} });
    await controller.connect();
  }
  assert.deepEqual(calls, ['after-load', 'now']);
});

// safeNext is enforced by the server (TestClerkLoginNextStaysOnOrigin).
// Exercise its allowed outputs here with browser URL semantics, particularly
// the encoded slash that must not be decoded again before navigation.
for (const mode of ['signed-in', 'signin', 'signup']) {
  test(`hosted ${mode} keeps server-provided return paths on origin`, async t => {
    const origin = 'https://app.test';
    const destinations = [];
    const old = { fetch: globalThis.fetch, window: globalThis.window, clerk: globalThis.testClerk };
    globalThis.testClerk = {
      isSignedIn: mode === 'signed-in',
      session: { getToken: async () => {} },
      mountSignIn: (_element, options) => destinations.push(options.forceRedirectUrl),
      mountSignUp: (_element, options) => destinations.push(options.forceRedirectUrl),
    };
    globalThis.fetch = async () => ({ ok: true });
    globalThis.window = { location: { replace: next => destinations.push(next) } };
    t.after(() => { globalThis.fetch = old.fetch; globalThis.window = old.window; globalThis.testClerk = old.clerk; });
    const { default: Controller } = await import(`data:text/javascript,${encodeURIComponent(source)}#return-${mode}`);
    // '/' is the server's fallback for //evil.test, ///evil.test and ////evil.test.
    for (const next of ['/', '/%2F/evil.test', '/ok', '/ok?x=//y']) {
      const controller = new Controller();
      Object.assign(controller, {
        element: { isConnected: true }, modeValue: mode === 'signup' ? 'signup' : 'signin',
        nextValue: next, statusTarget: { textContent: '' }, hasRecoveryTarget: false, mountTarget: {},
      });
      await controller.connect();
      assert.equal(controller.statusTarget.textContent, '');
      assert.equal(destinations.length, 1);
      const destination = destinations.pop();
      assert.equal(destination, next);
      assert.equal(new URL(destination, origin).origin, origin);
    }
  });
}
