import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const assets = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery');
const scripts = ['api.js', 'state-api.js', 'network-prompt.js', 'detail.js'].map(name => path.join(assets, name));

function loadNavigation({ approved = true, readOnly = false, publicNavigation = false } = {}) {
  const calls = [];
  const opened = [];
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
    open: (...args) => { opened.push(args); },
    fetch: async (url, options) => {
      calls.push({ url, options });
      if (url.endsWith('/navigation-check')) {
        const destination = JSON.parse(options.body).url;
        return new Response(JSON.stringify(permitted ? { url: destination } : { error: 'Connection destinations changed; reopen the app.' }), { status: permitted ? 200 : failureStatus });
      }
      return new Response('{}', { status: 200 });
    }
  }, { 'link-modal': { hidden: true }, 'net-modal': { hidden: true } });
  return { ...page, calls, opened, deny(status = 403) { permitted = false; failureStatus = status; } };
}

const link = { __avNavigate: true, artifactId: 'app-1', url: 'https://attacker.example/?balance=private' };

test('an existing links approval cannot bypass current connection destinations', async () => {
  const page = loadNavigation();
  page.deny();
  await page.postFromFrame(link);
  assert.deepEqual(page.opened, []);
  assert.equal(page.byId('link-modal').hidden, true);
  assert.match(page.byId('al-status').textContent, /destinations changed/);
  assert.equal(page.calls[0].options.headers['X-Exhibit-Relay-Policy'], 'current-host-policy');
  assert.equal(page.calls[0].options.headers.Authorization, 'Bearer viewer-token');
});

test('modal approval rechecks policy after the permission is saved', async () => {
  const page = loadNavigation({ approved: false });
  await page.postFromFrame(link);
  assert.equal(page.byId('link-modal').hidden, false);
  page.deny(409);
  await page.byId('link-allow').click();
  assert.deepEqual(page.opened, []);
  assert.deepEqual(page.calls.map(call => call.options.method), ['POST', 'PATCH', 'POST']);
  assert.equal(page.byId('link-modal').hidden, true);
});

test('recipients validate an existing link permission without receiving editing authority', async () => {
  const page = loadNavigation({ readOnly: true });
  await page.postFromFrame(link);
  assert.deepEqual(page.opened, [[link.url, '_blank', 'noopener,noreferrer']]);
  assert.equal(page.calls.length, 1);
  assert.equal(page.calls[0].url, '/api/artifacts/app-1/navigation-check');
});

test('another window cannot ask the host to validate or open a link', async () => {
  const page = loadNavigation();
  await page.postFromFrame(link, {});
  assert.deepEqual(page.calls, []);
  assert.deepEqual(page.opened, []);
});

test('an anonymous unconnected public view spends the server-derived link exemption', async () => {
  const page = loadNavigation({ readOnly: true, publicNavigation: true });
  await page.postFromFrame(link);
  assert.deepEqual(page.opened, [[link.url, '_blank', 'noopener,noreferrer']]);
  assert.deepEqual(page.calls, []);
});

test('newly approved links also open without an app-origin referrer', async () => {
  const page = loadNavigation({ approved: false });
  await page.postFromFrame(link);
  await page.byId('link-allow').click();
  assert.deepEqual(page.opened, [[link.url, '_blank', 'noopener,noreferrer']]);
});

test('all current-artifact state methods receive the app-owned policy header', async () => {
  const page = loadNavigation();
  for (const [method, suffix] of [['GET', ''], ['PUT', ''], ['DELETE', '?key=balance'], ['POST', '/import']]) {
    await page.window.apiStateFetch('/api/artifacts/app-1/state' + suffix, {
      method, headers: { 'x-exhibit-relay-policy': 'guest-choice' }, body: method === 'GET' ? undefined : '{}'
    });
    const headers = page.calls.at(-1).options.headers;
    assert.equal(headers['X-Exhibit-Relay-Policy'], 'current-host-policy');
    assert.equal(headers['x-exhibit-relay-policy'], undefined);
  }
  const { apiFetch } = await page.loadModule(path.join(assets, 'api-module.js'));
  await apiFetch('/api/artifacts/app-1/state', { method: 'PUT', body: '{}' });
  assert.equal(page.calls.at(-1).options.headers['X-Exhibit-Relay-Policy'], 'current-host-policy');
});

test('agent state writes use the active preview policy and do not send it to another artifact', async () => {
  const page = loadNavigation();
  const query = page.document.querySelector;
  page.document.querySelector = selector => selector === '[data-relay-host-artifact-id-value]'
    ? { dataset: { relayHostArtifactIdValue: 'app-1', relayHostPolicyValue: 'active-preview-policy' } } : query(selector);
  await page.window.apiStateFetch('/api/artifacts/app-1/state', { method: 'PUT', body: '{}' });
  assert.equal(page.calls.at(-1).options.headers['X-Exhibit-Relay-Policy'], 'active-preview-policy');
  await page.window.apiFetch('/api/artifacts/other/state');
  assert.equal(page.calls.at(-1).options.headers['X-Exhibit-Relay-Policy'], undefined);
});
