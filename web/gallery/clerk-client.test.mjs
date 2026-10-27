import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = (await readFile(new URL('./clerk-client.js', import.meta.url), 'utf8'))
  .replace("'./clerk-appearance.js'", JSON.stringify(new URL('./clerk-appearance.js', import.meta.url).href));

for (const stage of ['script download', 'SDK initialization']) {
  test(`Clerk reports a timeout during ${stage}`, async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    const scripts = [];
    const oldDocument = globalThis.document;
    const oldWindow = globalThis.window;
    globalThis.document = {
      querySelector: selector => ({ content: selector.includes('publishable') ? 'pk_test_example' : 'https://example.clerk.accounts.dev' }),
      createElement: () => ({ dataset: {} }),
      head: { append: script => scripts.push(script) },
    };
    globalThis.window = { Clerk: { load: () => new Promise(() => {}) } };
    t.after(() => { globalThis.document = oldDocument; globalThis.window = oldWindow; });
    const { clerkClient } = await import(`data:text/javascript,${encodeURIComponent(source)}#${encodeURIComponent(stage)}`);
    const pending = clerkClient();
    const rejected = assert.rejects(pending, /account service did not respond/);
    if (stage === 'SDK initialization') {
      scripts.forEach(script => script.onload());
      await Promise.resolve(); await Promise.resolve();
    }
    t.mock.timers.tick(15000);
    await rejected;
  });
}

// Exh-g0fx: ordinary pages start Clerk after load, when the main thread is idle,
// and a load event held up by a slow frame cannot postpone it past the cap.
const sessionCookie = secondsLeft => {
  const payload = Buffer.from(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + secondsLeft })).toString('base64url');
  return `__client_uat=1; __session=eyJhbGciOiJSUzI1NiJ9.${payload}.sig`;
};

function fakePage(t, readyState, cookie = sessionCookie(60)) {
  const scripts = [], listeners = {}, idle = [];
  const old = { document: globalThis.document, window: globalThis.window };
  globalThis.document = {
    readyState,
    cookie,
    querySelector: selector => ({ content: selector.includes('publishable') ? 'pk_test_example' : 'https://example.clerk.accounts.dev' }),
    createElement: () => ({ dataset: {} }),
    head: { append: script => scripts.push(script) },
  };
  globalThis.window = {
    Clerk: { load: () => new Promise(() => {}) },
    addEventListener: (name, fn) => { listeners[name] = fn; },
    requestIdleCallback: fn => idle.push(fn),
  };
  t.after(() => { globalThis.document = old.document; globalThis.window = old.window; });
  return { scripts, listeners, idle };
}

const flush = async () => { for (let i = 0; i < 5; i++) await Promise.resolve(); };

test('ordinary pages load Clerk only after the load event and an idle period', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const page = fakePage(t, 'interactive');
  const { clerkClientAfterLoad } = await import(`data:text/javascript,${encodeURIComponent(source)}#after-load`);
  clerkClientAfterLoad().catch(() => {});
  await flush();
  assert.equal(page.scripts.length, 0, 'nothing loads while the page is still loading');
  page.listeners.load();
  await flush();
  assert.equal(page.scripts.length, 0, 'nothing loads until the main thread is idle');
  page.idle.forEach(fn => fn());
  await flush();
  assert.equal(page.scripts.length, 2);
});

test('a page whose load event never arrives still starts Clerk at the cap', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const page = fakePage(t, 'interactive');
  const { clerkClientAfterLoad } = await import(`data:text/javascript,${encodeURIComponent(source)}#cap`);
  clerkClientAfterLoad().catch(() => {});
  t.mock.timers.tick(4999);
  await flush();
  assert.equal(page.scripts.length, 0);
  t.mock.timers.tick(1);
  await flush();
  assert.equal(page.scripts.length, 2, 'the cap starts Clerk itself, not another idle wait');
  assert.equal(page.idle.length, 0);
});

// Review of Exh-g0fx: a page admitted on a nearly spent token must not wait
// past the point where Clerk can still renew it.
for (const [name, cookie] of [
  ['a nearly expired session', sessionCookie(14)],
  ['no session cookie', '__client_uat=0'],
  ['an unreadable session cookie', '__session=not-a-jwt'],
]) {
  test(`${name} starts Clerk at once`, async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    const page = fakePage(t, 'interactive', cookie);
    const { clerkClientAfterLoad } = await import(`data:text/javascript,${encodeURIComponent(source)}#${encodeURIComponent(name)}`);
    clerkClientAfterLoad().catch(() => {});
    await flush();
    assert.equal(page.scripts.length, 2);
  });
}

test('the wait ends early enough to renew a session expiring soon', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const page = fakePage(t, 'interactive', sessionCookie(17));
  const { clerkClientAfterLoad } = await import(`data:text/javascript,${encodeURIComponent(source)}#soon`);
  clerkClientAfterLoad().catch(() => {});
  t.mock.timers.tick(1500);
  await flush();
  assert.equal(page.scripts.length, 0);
  t.mock.timers.tick(1000);
  await flush();
  assert.equal(page.scripts.length, 2);
});

test('an already loaded page starts Clerk at the next idle period', async t => {
  const page = fakePage(t, 'complete');
  const { clerkClientAfterLoad } = await import(`data:text/javascript,${encodeURIComponent(source)}#complete`);
  clerkClientAfterLoad().catch(() => {});
  await flush();
  assert.equal(page.idle.length, 1);
  page.idle[0]();
  await flush();
  assert.equal(page.scripts.length, 2);
});
