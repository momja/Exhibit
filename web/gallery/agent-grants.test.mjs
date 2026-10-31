import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadPageScript, recordingApi } from './testdom.mjs';

const SCRIPT = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery/agent-grants.js');
const issued = (id, token) => ({ ok: true, status: 201, json: async () => ({ grant: { id }, token }) });
const paths = (api) => api.calls.map((call) => call.method + ' ' + call.path);

async function load(responses = []) {
  const api = recordingApi(responses);
  const events = [];
  const copied = [];
  const page = loadPageScript([], {
    apiFetch: api.apiFetch,
    navigator: { clipboard: { writeText: async (text) => { copied.push(text); } } }
  }, {
    'agent-grants-panel': { dataset: { mcpUrl: 'https://agents.example.test/mcp' } },
    'agent-grant-issued': { hidden: true },
    'agent-grant-label': { value: '' },
    'agent-mcp-url': { value: 'https://agents.example.test/mcp' },
    'agent-mcp-command': { value: "claude mcp add --transport http exhibit 'https://agents.example.test/mcp'" }
  });
  page.document.body.addEventListener('exhibit:agent-grants-changed', () => events.push('changed'));
  const panel = page.byId('agent-grants-panel');
  const controls = [page.byId('agent-grant-label'), page.byId('agent-grant-issue')];
  panel.querySelectorAll = () => controls;
  const issue = (label) => {
    page.byId('agent-grant-label').value = label;
    return panel.dispatchEvent({ type: 'submit', target: page.byId('agent-grant-form'), preventDefault() {} });
  };
  const click = (selector, element) => panel.dispatchEvent({ type: 'click', target: {
    closest: (query) => query === selector ? element : null
  } });
  const revoke = (grantId) => click('[data-action="revoke-agent-grant"]', { dataset: { grantId } });
  const copy = () => click('#agent-grant-copy', page.byId('agent-grant-copy'));
  const dismiss = () => click('#agent-grant-dismiss', page.byId('agent-grant-dismiss'));
  const refresh = () => panel.dispatchEvent({ type: 'htmx:after:swap', target: page.byId('agent-grants-body') });
  await page.loadModule(SCRIPT);
  return { ...page, api, events, copied, panel, controls, issue, revoke, copy, click, dismiss, refresh };
}

test('issue displays and copies the secret, keeping it through server list refresh', async () => {
  const page = await load([issued('g-1', 'exgrant_secret')]);
  await page.issue('  Claude Code  ');
  assert.deepEqual(paths(page.api), ['POST /api/agent-grants']);
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { label: 'Claude Code' });
  assert.deepEqual(page.events, ['changed']);
  assert.equal(page.byId('agent-grant-issued').hidden, false);
  assert.equal(page.byId('agent-grant-token').value, 'exgrant_secret');
  assert.equal(page.byId('agent-grant-label').disabled, true);
  await page.issue('double submit');
  assert.equal(page.api.calls.length, 1, 'do not issue again while refreshing');
  await page.refresh();
  assert.equal(page.byId('agent-grant-label').disabled, false);
  assert.equal(page.byId('agent-grant-token').value, 'exgrant_secret');
  await page.copy();
  assert.deepEqual(page.copied, ['exgrant_secret']);
  assert.equal(page.byId('agent-grants-status').textContent, 'Token copied.');
});

test('dismissing and leaving the page erase the secret', async () => {
  const page = await load([issued('g-1', 'exgrant_secret'), issued('g-2', 'exgrant_new')]);
  await page.issue('agent');
  await page.refresh();
  await page.dismiss();
  assert.equal(page.byId('agent-grant-token').value, '');
  assert.equal(page.byId('agent-grant-issued').hidden, true);
  assert.equal(page.byId('agent-grant-label').focused, true);
  await page.issue('agent');
  await page.dispatchOnWindow('pagehide');
  assert.equal(page.byId('agent-grant-token').value, '');
  assert.equal(page.byId('agent-grant-issued').hidden, true);
});

test('revoking another connection keeps the token; revoking its connection clears it', async () => {
  const page = await load([issued('g-new', 'exgrant_secret'), { ok: true, status: 204 }, { ok: false, status: 404 }]);
  await page.issue('agent');
  await page.refresh();
  await page.revoke('g/old');
  await page.refresh();
  assert.equal(page.byId('agent-grant-token').value, 'exgrant_secret');
  await page.revoke('g-new');
  assert.deepEqual(paths(page.api).slice(1), ['DELETE /api/agent-grants/g%2Fold', 'DELETE /api/agent-grants/g-new']);
  assert.equal(page.byId('agent-grant-token').value, '');
  assert.equal(page.byId('agent-grants-status').textContent, 'Token revoked.');
});

test('failed requests show an error and release controls after refresh failures', async () => {
  const page = await load([{ ok: false, status: 500 }]);
  await page.issue('agent');
  assert.equal(page.byId('agent-grant-issued').hidden, true);
  assert.equal(page.byId('agent-grants-status').textContent, 'Could not issue a token. Try again.');
  assert.deepEqual(page.events, ['changed']);
  await page.panel.dispatchEvent({ type: 'htmx:error', target: page.byId('agent-grants-body') });
  assert.equal(page.byId('agent-grant-issue').disabled, false);
  assert.match(page.byId('agent-grants-status').textContent, /could not refresh/);
});

test('remote config uses the configured endpoint and clears every copy of the secret', async () => {
  const page = await load([issued('g-1', 'exgrant_secret')]);
  page.panel.dataset.mcpUrl = "https://agents.example.test/connector/'$(command)";
  await page.issue('agent');
  assert.equal(page.byId('agent-grant-remote-command').textContent,
    "claude mcp add --transport http exhibit 'https://agents.example.test/connector/'\\''$(command)' --header 'Authorization: Bearer exgrant_secret'");
  assert.deepEqual(JSON.parse(page.byId('agent-grant-remote-config').textContent), {
    type: 'http', url: page.panel.dataset.mcpUrl,
    headers: { Authorization: 'Bearer exgrant_secret' }
  });
  await page.refresh();
  await page.dismiss();
  assert.equal(page.byId('agent-grant-remote-command').textContent, '');
  assert.equal(page.byId('agent-grant-remote-config').textContent, '');
});

test('revocation-only panel can revoke and leave without issuing controls', async () => {
  const page = await load([{ ok: true, status: 204 }]);
  page.panel.dataset.mcpUrl = '';
  const getElementById = page.document.getElementById;
  page.document.getElementById = (id) =>
    ['agent-grant-token', 'agent-grant-issued', 'agent-grant-form'].includes(id) ? null : getElementById(id);
  await page.revoke('existing');
  assert.deepEqual(paths(page.api), ['DELETE /api/agent-grants/existing']);
  await page.refresh();
  await page.dispatchOnWindow('pagehide');
});


test('browser connection details copy before issuing and survive token cleanup', async () => {
  const page = await load([issued('g-1', 'exgrant_secret')]);
  const url = page.byId('agent-mcp-url').value;
  const command = page.byId('agent-mcp-command').value;
  for (const [id, message] of [['agent-mcp-url', 'URL copied.'], ['agent-mcp-command', 'Command copied.']]) {
    await page.click('#' + id + '-copy', page.byId(id + '-copy'));
    assert.equal(page.byId('agent-grants-status').textContent, message);
  }
  assert.deepEqual(page.copied, [url, command]);
  assert.deepEqual(page.api.calls, []);
  assert.equal(page.byId('agent-grant-issued').hidden, true);
  await page.issue('fallback');
  await page.refresh();
  await page.dismiss();
  await page.dispatchOnWindow('pagehide');
  assert.equal(page.byId('agent-mcp-url').value, url);
  assert.equal(page.byId('agent-mcp-command').value, command);
  await page.click('#agent-mcp-command-copy', page.byId('agent-mcp-command-copy'));
  assert.deepEqual(page.copied, [url, command, command]);
});
