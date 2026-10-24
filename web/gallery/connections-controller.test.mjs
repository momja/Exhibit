import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/connections-controller.js', import.meta.url));

async function setup(responses = []) {
  const calls = [], approvals = [], confirmations = [], changes = [];
  let confirm = true;
  const page = loadPageScript([], {
    CustomEvent, Headers,
    FormData: class {
      constructor(fields) { this.fields = fields; }
      get(name) { return this.fields[name] ?? null; }
    },
    fetch: async (path, options = {}) => {
      calls.push({ path, method: options.method || 'GET', data: options.body ? JSON.parse(options.body) : null });
      const response = responses.shift();
      if (response) return response;
      if (path === '/api/connections') return Response.json({ connections: [{ id: 'selected-connection', origins: ['https://accounts.example.test'] }] });
      return options.method === 'DELETE' ? new Response(null, { status: 204 }) : Response.json({}, { status: 201 });
    },
    apiFetch: async (path, options) => { approvals.push({ path, data: JSON.parse(options.body) }); return Response.json({}); },
    confirm: message => { confirmations.push(message); return confirm; },
  });
  const { default: Connections } = await page.loadModule(script);
  page.document.body.addEventListener('exhibit:connections-changed', () => changes.push('connections-changed'));
  const controller = Object.create(Connections.prototype);
  controller.artifactIdValue = 'connected-app';
  controller.feedbackTarget = page.byId('feedback');
  let pending;
  controller.action = (_event, work) => { pending = work(); };
  return {
    ...page, calls, approvals, confirmations, changes, controller,
    setConfirmation(value) { confirm = value; },
    async grant(fields) { controller.grant({ currentTarget: fields }); await pending; },
    async clear() { controller.clearData({}); await pending; },
  };
}

test('granting a connector sends credential access and sharing consent without a data ceiling', async () => {
  const page = await setup();
  await page.grant({ connection: 'selected-connection', sharing: 'on', egress: 'https://legacy.example.test' });
  assert.deepEqual(page.calls, [{
    path: '/api/artifacts/connected-app/connections', method: 'POST',
    data: { connection_id: 'selected-connection', share_with_recipients: true },
  }]);
  assert.deepEqual(page.approvals, []);
  assert.equal(page.controller.feedbackTarget.textContent, 'Connection access granted.');
});

test('optional origin approval adds only the selected connector API without replacing other approvals', async () => {
  const page = await setup();
  await page.grant({ connection: 'selected-connection', approve_origins: 'on' });
  assert.deepEqual(page.approvals, [{
    path: '/api/artifacts/connected-app/origins',
    data: { origin: 'https://accounts.example.test', decision: 'allow', source: 'connection-consent' },
  }]);
  assert.deepEqual(page.calls, [{ path: '/api/connections', method: 'GET', data: null }, {
    path: '/api/artifacts/connected-app/connections', method: 'POST',
    data: { connection_id: 'selected-connection', share_with_recipients: false },
  }]);
});

test('saved-data cleanup is optional and does not change connection grants or origin approvals', async () => {
  const page = await setup();
  page.setConfirmation(false);
  await page.clear();
  assert.deepEqual(page.calls, []);
  page.setConfirmation(true);
  await page.clear();
  assert.deepEqual(page.calls, [{ path: '/api/artifacts/connected-app/connection-data', method: 'DELETE', data: null }]);
  assert.deepEqual(page.approvals, []);
  assert.match(page.confirmations[0], /Connection grants and network approvals stay in place/);
  assert.equal(page.controller.feedbackTarget.textContent, 'Saved data cleared.');
});
