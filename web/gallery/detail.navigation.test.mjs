import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadPageScript, permissionDialog } from './testdom.mjs';

const assets = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery');
const scripts = ['api.js', 'state-api.js'].map(name => path.join(assets, name));

async function loadNavigation({ approved = true, readOnly = false, publicNavigation = false } = {}) {
  const calls = [];
  const opened = [];
  const tabs = [];
  let nextCheckPause;
  let permitted = true;
  let failureStatus = 403;
  let page;
  page = loadPageScript(scripts, {
    TOKEN: 'viewer-token', READ_ONLY: readOnly, STATE_WRITABLE: true,
    SHARED_STATE: false, ID: 'app-1', SOURCE_URL: '', OPEN_URL: '/artifacts/app-1/open',
    NAVIGATION_POLICY: 'current-host-policy',
    PUBLIC_NAVIGATION: publicNavigation,
    downloadsApproved: false, clipboardApproved: false, linksApproved: approved,
    cameraApproved: false, microphoneApproved: false, geolocationApproved: false,
    CustomEvent,
    apiFetch: (...args) => page.window.apiFetch(...args),
    apiStateFetch: (...args) => page.window.apiStateFetch(...args),
    open: (...args) => {
      opened.push(args);
      const elements = [];
      let location = null;
      const tab = {
        opener: page.window, closed: false,
        close() { this.closed = true; },
        document: {
          createElement: tag => ({ tag }),
          head: { appendChild(element) { elements.push(element); } }
        },
        get location() { return location; },
        set location(value) {
          this.policyAtNavigation = elements.map(({ name, content }) => ({ name, content }));
          location = value;
        }
      };
      tabs.push(tab);
      return tab;
    },
    fetch: async (url, options) => {
      calls.push({ url, options });
      if (url.endsWith('/navigation-check')) {
        const destination = JSON.parse(options.body).url;
        if (nextCheckPause) {
          const paused = nextCheckPause;
          nextCheckPause = null;
          paused.start();
          await paused.wait;
        }
        return new Response(JSON.stringify(permitted ? { url: destination } : { error: 'Connection destinations changed; reopen the app.' }), { status: permitted ? 200 : failureStatus });
      }
      return new Response('{}', { status: 200 });
    }
  });
  for (const id of ['net-modal', 'dl-modal', 'clip-modal', 'link-modal', 'media-modal', 'geo-modal']) {
    permissionDialog(page, id);
  }
  const detail = await page.loadModule(path.join(assets, 'detail.js'), ['apiFetch']);
  return {
    ...page, detail, calls, opened, tabs,
    deny(status = 403) { permitted = false; failureStatus = status; },
    pauseNextCheck() {
      let start, release;
      const started = new Promise(resolve => { start = resolve; });
      const wait = new Promise(resolve => { release = resolve; });
      nextCheckPause = { start, wait };
      return { started, release };
    }
  };
}

const link = { __avNavigate: true, artifactId: 'app-1', url: 'https://attacker.example/?balance=private' };

test('an existing links approval cannot bypass current connection destinations', async () => {
  const page = await loadNavigation();
  page.deny();
  await page.postFromFrame(link);
  assert.deepEqual(page.opened, []);
  assert.equal(page.byId('link-modal').open, false);
  assert.match(page.byId('al-status').textContent, /destinations changed/);
  assert.equal(page.calls[0].options.headers['X-Exhibit-Relay-Policy'], 'current-host-policy');
  assert.equal(page.calls[0].options.headers.Authorization, 'Bearer viewer-token');
});

test('modal approval rechecks policy after the permission is saved', async () => {
  const page = await loadNavigation({ approved: false });
  await page.postFromFrame(link);
  assert.equal(page.byId('link-modal').open, true);
  page.deny(409);
  await page.byId('link-allow').click();
  assert.deepEqual(page.opened, [['', '_blank']]);
  assert.equal(page.tabs[0].closed, true, 'refusal closes the isolated placeholder');
  assert.equal(page.tabs[0].location, null, 'refused data never reaches the destination');
  assert.deepEqual(page.calls.map(call => call.options.method), ['POST', 'PATCH', 'POST']);
  assert.equal(page.byId('link-modal').open, true, 'the failed transaction remains dismissible');
});

test('recipients validate an existing link permission without receiving editing authority', async () => {
  const page = await loadNavigation({ readOnly: true });
  await page.postFromFrame(link);
  assert.deepEqual(page.opened, [[link.url, '_blank', 'noopener,noreferrer']]);
  assert.equal(page.calls.length, 1);
  assert.equal(page.calls[0].url, '/api/artifacts/app-1/navigation-check');
});

test('another window cannot ask the host to validate or open a link', async () => {
  const page = await loadNavigation();
  await page.postFromFrame(link, {});
  assert.deepEqual(page.calls, []);
  assert.deepEqual(page.opened, []);
});

test('an anonymous unconnected public view spends the server-derived link exemption', async () => {
  const page = await loadNavigation({ readOnly: true, publicNavigation: true });
  await page.postFromFrame(link);
  assert.deepEqual(page.opened, [[link.url, '_blank', 'noopener,noreferrer']]);
  assert.deepEqual(page.calls, []);
});

test('newly approved links also open without an app-origin referrer', async () => {
  const page = await loadNavigation({ approved: false });
  await page.postFromFrame(link);
  await page.byId('link-allow').click();
  assert.deepEqual(page.opened, [['', '_blank']]);
  assert.equal(page.tabs[0].opener, null);
  assert.equal(page.tabs[0].location, link.url);
  assert.deepEqual(page.tabs[0].policyAtNavigation, [{ name: 'referrer', content: 'no-referrer' }]);
  assert.equal(page.byId('link-modal').open, false);
});

test('preview navigation invalidates an outstanding automatic link check', async () => {
  const page = await loadNavigation();
  const paused = page.pauseNextCheck();
  const navigating = page.postFromFrame(link);
  await paused.started;
  await page.frame.dispatchEvent(new Event('load'));
  paused.release();
  await navigating;
  assert.deepEqual(page.opened, []);
  assert.equal(page.byId('link-modal').open, false);
});

test('preview navigation closes a pending approval tab before its policy check finishes', async () => {
  const page = await loadNavigation({ approved: false });
  await page.postFromFrame(link);
  const paused = page.pauseNextCheck();
  const approving = page.byId('link-allow').click();
  await paused.started;
  assert.deepEqual(page.opened, [['', '_blank']], 'the click claims a tab before awaiting the server');
  await page.frame.dispatchEvent(new Event('load'));
  paused.release();
  await approving;
  assert.equal(page.tabs[0].closed, true);
  assert.equal(page.tabs[0].location, null);
  assert.equal(page.byId('link-modal').open, false);
});

test('all current-artifact state methods receive the app-owned policy header', async () => {
  const page = await loadNavigation();
  for (const [method, suffix] of [['GET', ''], ['PUT', ''], ['DELETE', '?key=balance'], ['POST', '/import']]) {
    await page.window.apiStateFetch('/api/artifacts/app-1/state' + suffix, {
      method, headers: { 'x-exhibit-relay-policy': 'guest-choice' }, body: method === 'GET' ? undefined : '{}'
    });
    const headers = page.calls.at(-1).options.headers;
    assert.equal(headers['X-Exhibit-Relay-Policy'], 'current-host-policy');
    assert.equal(headers['x-exhibit-relay-policy'], undefined);
  }
  await page.detail.apiFetch('/api/artifacts/app-1/state', { method: 'PUT', body: '{}' });
  assert.equal(page.calls.at(-1).options.headers['X-Exhibit-Relay-Policy'], 'current-host-policy');
});

test('agent state writes use the active preview policy and do not send it to another artifact', async () => {
  const page = await loadNavigation();
  const query = page.document.querySelector;
  page.document.querySelector = selector => selector === '[data-relay-host-artifact-id-value]'
    ? { dataset: { relayHostArtifactIdValue: 'app-1', relayHostPolicyValue: 'active-preview-policy' } } : query(selector);
  await page.window.apiStateFetch('/api/artifacts/app-1/state', { method: 'PUT', body: '{}' });
  assert.equal(page.calls.at(-1).options.headers['X-Exhibit-Relay-Policy'], 'active-preview-policy');
  await page.window.apiFetch('/api/artifacts/other/state');
  assert.equal(page.calls.at(-1).options.headers['X-Exhibit-Relay-Policy'], undefined);
});

test('link messages received during approval are dropped before checking policy', async () => {
  const page = await loadNavigation({ approved: false });
  await page.postFromFrame(link);
  const paused = page.pauseNextCheck();
  const approving = page.byId('link-allow').click();
  await paused.started;
  const checks = page.calls.length;
  try {
    await page.postFromFrame({ ...link, url: 'https://other.example/' });
    assert.equal(page.calls.length, checks, 'a pending approval must not start another policy check');
  } finally {
    paused.release();
    await approving;
  }
  assert.equal(page.tabs[0].location, link.url);
  assert.equal(page.opened.length, 1);
});
