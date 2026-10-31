/* Exercise the built External agents script (Exh-l2xj) through its form,
 * its revoke buttons and the htmx refresh that follows each. */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadPageScript, recordingApi } from './testdom.mjs';

const SCRIPT = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery/agent-grants.js');
const ID = 'art-1';
const issued = (id, token) => ({ ok: true, status: 201, json: async () => ({ grant: { id, label: 'x' }, token }) });
const paths = (api) => api.calls.map((c) => c.method + ' ' + c.path);

async function load(responses = []) {
  const api = recordingApi(responses);
  const events = [];
  const copied = [];
  const page = loadPageScript([], {
    apiFetch: api.apiFetch, CustomEvent,
    navigator: { clipboard: { writeText: async (text) => { copied.push(text); } } }
  }, {
    'agent-grants-panel': { hidden: true },
    'agent-grant-issued': { hidden: true },
    'agent-grant-label': { value: '' }
  });
  page.document.body.addEventListener('exhibit:agent-grants-changed', () => events.push('changed'));
  const panel = page.byId('agent-grants-panel');
  panel.querySelector = (selector) => selector === '.agent-grants' ? { dataset: { artifactId: ID } } : null;
  const controls = [page.byId('agent-grant-label'), page.byId('agent-grant-issue')];
  panel.querySelectorAll = () => controls;
  const issue = (label) => {
    page.byId('agent-grant-label').value = label;
    return panel.dispatchEvent({ type: 'submit', target: page.byId('agent-grant-form'), preventDefault() {} });
  };
  const revoke = (grantId) => panel.dispatchEvent({ type: 'click', target: {
    closest: (selector) => selector === '[data-action="revoke-agent-grant"]' ? { dataset: { grantId } } : null
  } });
  const copy = () => panel.dispatchEvent({ type: 'click', target: {
    closest: (selector) => selector === '#agent-grant-copy' ? page.byId('agent-grant-copy') : null
  } });
  const refresh = () => panel.dispatchEvent({ type: 'htmx:after:swap', target: page.byId('agent-grants-body') });
  await page.loadModule(SCRIPT);
  return { ...page, api, events, copied, panel, controls, issue, revoke, copy, refresh };
}

test('issuing shows the token once, outside the refreshed list, and copies it', async () => {
  const page = await load([issued('g-1', 'exgrant_secret')]);
  await page.byId('agent-grants-open').click();
  assert.equal(page.panel.hidden, false);
  await page.issue('  Claude Code  ');
  assert.deepEqual(paths(page.api), ['POST /api/artifacts/art-1/agent-grants']);
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { label: 'Claude Code' });
  assert.deepEqual(page.events, ['changed']);
  assert.equal(page.byId('agent-grant-issued').hidden, false);
  assert.equal(page.byId('agent-grant-token').value, 'exgrant_secret');
  assert.equal(page.byId('agent-grant-label').disabled, true, 'locked until the list refreshes');

  await page.refresh();
  assert.equal(page.byId('agent-grant-label').disabled, false);
  assert.equal(page.byId('agent-grant-token').value, 'exgrant_secret', 'the refresh does not erase it');

  await page.copy();
  assert.deepEqual(page.copied, ['exgrant_secret']);
  assert.equal(page.byId('agent-grants-status').textContent, '✓ Token copied');
});

test('closing the panel forgets the token', async () => {
  const page = await load([issued('g-1', 'exgrant_secret')]);
  await page.byId('agent-grants-open').click();
  await page.issue('agent');
  await page.refresh();
  await page.byId('agent-grants-close').click();
  assert.equal(page.panel.hidden, true);
  assert.equal(page.byId('agent-grant-token').value, '');
  assert.equal(page.byId('agent-grant-issued').hidden, true);
  assert.equal(page.byId('agent-grants-open').focused, true);
});

test('revoking another grant keeps the new token; revoking it removes it', async () => {
  const page = await load([issued('g-new', 'exgrant_secret'), { ok: true, status: 204 }, { ok: false, status: 404 }]);
  await page.issue('agent');
  await page.refresh();
  await page.revoke('g-old');
  await page.refresh();
  assert.equal(page.byId('agent-grant-token').value, 'exgrant_secret');
  await page.revoke('g-new');
  assert.deepEqual(paths(page.api).slice(1), [
    'DELETE /api/artifacts/art-1/agent-grants/g-old',
    'DELETE /api/artifacts/art-1/agent-grants/g-new',
  ]);
  assert.equal(page.byId('agent-grant-token').value, '', 'a concurrent revoke (404) still revoked it');
  assert.equal(page.byId('agent-grants-status').textContent, '✓ Token revoked');
});

test('a failed issue says so and shows no token', async () => {
  const page = await load([{ ok: false, status: 500, json: async () => ({}) }]);
  await page.issue('agent');
  assert.equal(page.byId('agent-grant-issued').hidden, true);
  assert.equal(page.byId('agent-grants-status').textContent, 'Could not issue a token. Try again.');
  assert.deepEqual(page.events, ['changed']);
});
