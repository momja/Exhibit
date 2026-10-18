/* Exercise the built sharing script through its native form and htmx events. */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadPageScript, recordingApi } from './testdom.mjs';

const SHARE_JS = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery/share.js');
const ID = 'art-1';
const response = (data = {}) => ({ ok: true, status: 200, json: async () => data });
const grant = (...names) => response({ results: names.map((name) => ({ name, status: 'granted' })) });
const paths = (api) => api.calls.map((c) => c.method + ' ' + c.path);

async function loadShare({ link = '', stateMode = 'own', responses = [] } = {}) {
  const api = recordingApi(responses);
  const events = [];
  const copied = [];
  const page = loadPageScript([], {
    ID: 'unused-page-global', apiFetch: api.apiFetch, CustomEvent,
    navigator: { clipboard: { writeText: async (text) => { copied.push(text); } } }
  }, {
    'share-panel': { hidden: false },
    'share-add-input': { value: '' },
    'share-link-toggle': { dataset: link ? { shareId: link } : {}, checked: !!link },
    'share-state-mode': { dataset: { savedMode: stateMode } },
    'share-state-own': { checked: stateMode === 'own' },
    'share-state-shared': { checked: stateMode === 'shared' },
    'share-link-url': { value: 'http://render.test/s/' + link },
    'share-widget-embed': { value: '<iframe src="example"></iframe>' },
    'state-how-it-works': { open: false },
    'share-widget-details': { open: false }
  });
  page.document.body.addEventListener('exhibit:shares-changed', () => events.push('changed'));
  const panel = page.byId('share-panel');
  const controls = ['share-add-input', 'share-state-own', 'share-state-shared', 'share-link-toggle', 'share-link-replace']
    .map(page.byId);
  panel.querySelectorAll = () => controls;
  const people = { scrollTop: 0 };
  panel.querySelector = (selector) => selector === '.share-people-list' ? people
    : selector === '.share-cols' ? { dataset: { artifactId: ID } } : null;
  const click = (id, icon = false) => {
    const button = page.byId(id);
    const target = icon ? { closest: () => button } : Object.assign(button, { closest: () => button });
    if (['share-close', 'share-open'].includes(id)) return button.dispatchEvent({ type: 'click', target: button });
    target.closest = (selector) => selector === 'button' ? button : null;
    return panel.dispatchEvent({ type: 'click', target });
  };
  const mode = (value) => {
    page.byId('share-state-own').checked = value === 'own';
    page.byId('share-state-shared').checked = value === 'shared';
    return panel.dispatchEvent({ type: 'change', target: { name: 'share-state-mode', value } });
  };
  const linkOn = (checked) => {
    page.byId('share-link-toggle').checked = checked;
    return panel.dispatchEvent({ type: 'change', target: page.byId('share-link-toggle') });
  };
  const names = (value) => {
    page.byId('share-add-input').value = value;
    return panel.dispatchEvent({ type: 'input', target: page.byId('share-add-input') });
  };
  const submit = () => panel.dispatchEvent({ type: 'submit', target: page.byId('share-add-form'), preventDefault() {} });
  const enter = async (options = {}) => {
    let prevented = false;
    await panel.dispatchEvent({ type: 'keydown', key: 'Enter', target: page.byId('share-add-input'),
      preventDefault() { prevented = true; }, ...options });
    // A browser implicitly submits this one-field form unless Enter is consumed.
    if (!prevented) await submit();
  };
  const composition = (type) => panel.dispatchEvent({ type, target: page.byId('share-add-input') });
  const revoke = (id) => panel.dispatchEvent({ type: 'click', target: {
    closest: (selector) => selector === '[data-action="revoke"]' ? { dataset: { shareId: id } } : null
  } });
  const refresh = async (saved = {}) => {
    await panel.dispatchEvent({ type: 'htmx:before:swap', target: page.byId('share-body') });
    if ('stateMode' in saved) stateMode = saved.stateMode;
    if ('link' in saved) link = saved.link;
    page.byId('share-state-mode').dataset.savedMode = stateMode;
    page.byId('share-link-toggle').dataset = link ? { shareId: link } : {};
    page.byId('share-link-url').value = link ? 'http://render.test/s/' + link : '';
    // Simulate the fresh nodes' default state and radio reconciliation by htmx.
    page.byId('share-add-input').value = '';
    page.byId('share-state-own').checked = false;
    page.byId('share-state-shared').checked = false;
    page.byId('share-widget-details').open = false;
    page.byId('state-how-it-works').open = false;
    people.scrollTop = 0;
    return panel.dispatchEvent({ type: 'htmx:after:swap', target: page.byId('share-body') });
  };
  const refreshError = async (status = 500) => {
    let prevented = false;
    await panel.dispatchEvent({ type: 'htmx:after:request', target: page.byId('share-body'),
      detail: { ctx: { response: { status } } }, preventDefault() { prevented = true; } });
    return prevented;
  };
  await page.loadModule(SHARE_JS);
  return { ...page, panel, api, events, copied, controls, people, click, mode, linkOn, names, enter, submit,
    composition, revoke, refresh, refreshError };
}

test('Enter grants immediately; only htmx replaces the saved guest list', async () => {
  const page = await loadShare({ responses: [grant('alice')] });
  const serverRow = { name: 'server-rendered alice' };
  page.byId('share-grants').children = [serverRow];
  await page.names(' alice ');
  await page.enter();
  assert.deepEqual(paths(page.api), ['POST /api/shares']);
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { artifact_id: ID, recipients: ['alice'] });
  assert.deepEqual(page.events, ['changed']);
  assert.equal(page.byId('share-panel').hidden, false);
  assert.equal(page.byId('share-status').textContent, '✓ Changes saved');
  assert.equal(page.byId('share-add-input').disabled, true);
  await page.refresh();
  assert.equal(page.byId('share-add-input').value, '');
  assert.equal(page.byId('share-add-input').disabled, false);
  assert.equal(page.byId('share-add-input').focused, true);
  assert.strictEqual(page.byId('share-grants').children[0], serverRow);
});

test('native form submit supports the mobile keyboard and deduplicates pasted names', async () => {
  const page = await loadShare({ responses: [grant('ALICE', 'bob')] });
  await page.names('alice, bob, ALICE');
  await page.submit();
  assert.deepEqual(JSON.parse(page.api.calls[0].body).recipients, ['ALICE', 'bob']);
});

test('IME Enter, active composition, and blank submits do not grant access', async () => {
  const page = await loadShare();
  await page.names('alice');
  await page.enter({ isComposing: true });
  await page.enter({ keyCode: 229 });
  await page.composition('compositionstart');
  await page.enter();
  await page.submit();
  await page.composition('compositionend');
  await page.names(' , ');
  await page.submit();
  assert.deepEqual(page.api.calls, []);
});

test('link access saves immediately and its URL is supplied by the fragment', async () => {
  const page = await loadShare({ responses: [response({ share: { id: 'public' } }), response()] });
  await page.linkOn(true);
  assert.deepEqual(paths(page.api), ['POST /api/shares']);
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { artifact_id: ID });
  await page.linkOn(true);
  assert.equal(page.api.calls.length, 1, 'blocked until the fragment replaces the obsolete link id');
  await page.refresh({ link: 'public' });
  assert.equal(page.byId('share-link-url').value, 'http://render.test/s/public');
  await page.linkOn(false);
  assert.deepEqual(paths(page.api), ['POST /api/shares', 'DELETE /api/shares/public']);
  await page.refresh({ link: '' });
  assert.equal(page.byId('share-link-toggle').checked, false);
});

test('mode, revoke, and replacement each persist without a Save action', async () => {
  const page = await loadShare({ link: 'public', responses: [response(), response(), response({ share: { id: 'rotated' } })] });
  await page.mode('shared');
  await page.refresh({ stateMode: 'shared' });
  await page.revoke('old-grant');
  await page.refresh();
  await page.click('share-link-replace', true);
  assert.deepEqual(paths(page.api), ['PATCH /api/artifacts/' + ID, 'DELETE /api/shares/old-grant', 'POST /api/shares']);
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { share_state_mode: 'shared' });
  assert.deepEqual(JSON.parse(page.api.calls[2].body), { artifact_id: ID, replace: true });
  await page.refresh({ link: 'rotated' });
  await page.linkOn(false);
  assert.equal(paths(page.api).at(-1), 'DELETE /api/shares/rotated');
});

test('the mutation lock prevents duplicates both during the API request and before the swap', async () => {
  let resolve;
  const pendingResponse = new Promise((done) => { resolve = done; });
  const page = await loadShare({ responses: [pendingResponse] });
  await page.names('alice');
  const first = page.submit();
  await page.submit();
  await page.mode('shared');
  await page.linkOn(true);
  await page.revoke('grant-1');
  await page.click('share-close');
  assert.equal(page.api.calls.length, 1);
  assert.equal(page.byId('share-panel').hidden, false);
  assert.equal(page.byId('share-close').disabled, true);
  resolve(grant('alice'));
  await first;
  await page.submit();
  assert.equal(page.api.calls.length, 1);
  await page.refresh();
  assert.equal(page.byId('share-close').disabled, false);
});

test('failed grants retain typed names and can be retried after reconciliation', async () => {
  const page = await loadShare({ responses: [{ ok: false, status: 500 }, grant('alice')] });
  await page.names('alice');
  await page.submit();
  assert.match(page.byId('share-status').textContent, /Could not grant access/);
  await page.refresh();
  assert.equal(page.byId('share-add-input').value, 'alice');
  await page.submit();
  await page.refresh();
  assert.equal(page.byId('share-add-input').value, '');
  assert.equal(page.api.calls.length, 2);
});

test('per-person errors retain only unresolved names while saved people arrive from htmx', async () => {
  const page = await loadShare({ responses: [response({ results: [
    { name: 'alice', status: 'granted' }, { name: 'typo', status: 'unknown' },
    { name: 'ambiguous@example.org', status: 'ambiguous' }
  ] })] });
  await page.names('alice, typo, ambiguous@example.org');
  await page.submit();
  await page.refresh();
  assert.equal(page.byId('share-add-input').value, 'typo, ambiguous@example.org');
  assert.match(page.byId('share-status').textContent, /typo: no account/);
  assert.match(page.byId('share-status').textContent, /ambiguous@example.org: that address matches more than one account/);
});

test('failed mode and link changes restore saved choices and permit retry', async () => {
  const page = await loadShare({ responses: [{ ok: false, status: 500 }, response(), { ok: false, status: 500 }, response({ share: { id: 'new' } })] });
  await page.mode('shared');
  assert.equal(page.byId('share-state-own').checked, true);
  assert.match(page.byId('share-status').textContent, /Could not change the data mode/);
  await page.refresh();
  await page.mode('shared');
  await page.refresh({ stateMode: 'shared' });
  await page.linkOn(true);
  assert.equal(page.byId('share-link-toggle').checked, false);
  assert.match(page.byId('share-status').textContent, /Could not change link access/);
  await page.refresh();
  await page.linkOn(true);
  await page.refresh({ link: 'new' });
  assert.equal(page.byId('share-link-toggle').checked, true);
});

test('failed revoke and rotation retain the saved list and old link', async () => {
  const page = await loadShare({ link: 'public', responses: [{ ok: false, status: 500 }, { ok: false, status: 500 }] });
  const rows = [{ name: 'alice' }];
  page.byId('share-grants').children = rows;
  await page.revoke('grant-1');
  assert.strictEqual(page.byId('share-grants').children, rows);
  assert.match(page.byId('share-status').textContent, /Could not revoke access/);
  await page.refresh();
  await page.click('share-link-replace');
  await page.refresh();
  assert.equal(page.byId('share-link-url').value, 'http://render.test/s/public');
  assert.match(page.byId('share-status').textContent, /Could not replace the link/);
});

test('refresh failure preserves controls and reopening retries only the fragment request', async () => {
  const page = await loadShare({ responses: [response({ share: { id: 'public' } })] });
  await page.linkOn(true);
  assert.equal(await page.refreshError(), true, 'v4 error response must not replace the usable panel');
  assert.match(page.byId('share-status').textContent, /Close and reopen/);
  assert.equal(page.byId('share-close').disabled, false);
  assert.equal(page.byId('share-link-toggle').disabled, true);
  await page.linkOn(false);
  await page.click('share-close');
  await page.click('share-open');
  assert.deepEqual(page.events, ['changed', 'changed']);
  assert.equal(page.api.calls.length, 1);
  await page.refresh({ link: 'public' });
  assert.equal(page.byId('share-link-toggle').disabled, false);
});

test('network refresh errors also permit closing and retrying', async () => {
  const page = await loadShare();
  await page.mode('shared');
  await page.panel.dispatchEvent({ type: 'htmx:error', target: page.byId('share-body') });
  assert.match(page.byId('share-status').textContent, /could not refresh/);
  await page.click('share-close');
  assert.equal(page.byId('share-panel').hidden, true);
});

test('unfinished input, list position, and open disclosures survive a fragment swap', async () => {
  const page = await loadShare({ link: 'public' });
  await page.names('bob');
  page.byId('share-widget-details').open = true;
  page.byId('state-how-it-works').open = true;
  page.people.scrollTop = 170;
  await page.mode('shared');
  await page.refresh({ stateMode: 'shared' });
  assert.equal(page.byId('share-add-input').value, 'bob');
  assert.equal(page.byId('share-widget-details').open, true);
  assert.equal(page.byId('state-how-it-works').open, true);
  assert.equal(page.people.scrollTop, 170);
  assert.equal(page.byId('share-state-shared').checked, true);
});

test('Done closes without replaying changes or keeping unsent names for the next opening', async () => {
  const page = await loadShare();
  await page.mode('shared');
  await page.refresh({ stateMode: 'shared' });
  await page.names('unsent');
  await page.click('share-close');
  assert.equal(page.byId('share-panel').hidden, true);
  assert.equal(page.byId('share-open').focused, true);
  await page.click('share-open');
  assert.equal(page.byId('share-add-input').value, '');
  assert.equal(page.byId('share-state-shared').checked, true);
  assert.deepEqual(paths(page.api), ['PATCH /api/artifacts/' + ID]);
});

test('Escape and backdrop close the idle panel; interior clicks keep it open', async () => {
  const page = await loadShare();
  await page.click('share-open');
  page.pressKey('Escape');
  assert.equal(page.byId('share-panel').hidden, true);
  await page.click('share-open');
  await page.panel.dispatchEvent({ type: 'click', target: { id: 'inside' } });
  assert.equal(page.byId('share-panel').hidden, false);
  await page.panel.dispatchEvent({ type: 'click', target: page.panel });
  assert.equal(page.byId('share-panel').hidden, true);
});

test('both copy controls work when their Phosphor icon is clicked', async () => {
  const page = await loadShare({ link: 'public' });
  await page.click('share-link-copy', true);
  await page.click('share-widget-copy', true);
  assert.deepEqual(page.copied, ['http://render.test/s/public', '<iframe src="example"></iframe>']);
});

test('HTTP origins without a clipboard API offer manual copying', async () => {
  const page = await loadShare({ link: 'public' });
  page.window.navigator.clipboard = undefined;
  await page.click('share-link-copy');
  assert.equal(page.byId('share-link-url').selected, true);
  assert.match(page.byId('share-status').textContent, /Select the link and copy it/);
  await page.click('share-widget-copy');
  assert.equal(page.byId('share-widget-embed').selected, true);
  assert.match(page.byId('share-status').textContent, /Select the embed code and copy it/);
  assert.deepEqual(page.api.calls, []);
});

// A deleted artifact returns a server-rendered notice instead of editor fields.
// Exercise the nulls a real DOM returns, rather than the harness's lazy nodes.
test('a missing-artifact fragment replaces stale controls and Done remains usable', async () => {
  const page = await loadShare();
  await page.names('alice');
  await page.submit();
  assert.equal(page.byId('share-close').disabled, true);
  assert.equal(await page.refreshError(404), false, 'allow the server error fragment to swap');
  const absent = new Set(['share-state-mode', 'share-state-own', 'share-state-shared',
    'share-add-input', 'share-link-toggle', 'state-how-it-works', 'share-widget-details']);
  page.document.getElementById = (id) => absent.has(id) ? null : page.byId(id);
  page.panel.querySelectorAll = () => [];
  page.panel.querySelector = () => null;
  await page.panel.dispatchEvent({ type: 'htmx:after:swap', target: page.byId('share-body') });
  assert.equal(page.byId('share-close').disabled, false);
  assert.equal(page.byId('share-close').focused, true);
  assert.equal(page.byId('share-status').textContent, '');
  await page.click('share-close');
  assert.equal(page.panel.hidden, true);
  await page.click('share-open');
  assert.equal(page.panel.hidden, false);
  assert.deepEqual(page.events, ['changed'], 'reopening a missing artifact must not loop through retries');
  page.pressKey('Escape');
  assert.equal(page.panel.hidden, true);
});

test('concurrent grant and public-link revocations treat DELETE 404 as success', async () => {
  const page = await loadShare({ link: 'public', responses: [
    { ok: false, status: 404 }, { ok: false, status: 404 }
  ] });
  await page.revoke('already-gone');
  await page.refresh();
  assert.equal(page.byId('share-status').textContent, '✓ Changes saved');
  await page.linkOn(false);
  await page.refresh({ link: '' });
  assert.equal(page.byId('share-status').textContent, '✓ Changes saved');
  assert.equal(page.byId('share-link-toggle').checked, false);
  assert.deepEqual(paths(page.api), ['DELETE /api/shares/already-gone', 'DELETE /api/shares/public']);
});

test('404 responses to grants and mode changes still report failure', async () => {
  const page = await loadShare({ responses: [{ ok: false, status: 404 }, { ok: false, status: 404 }] });
  await page.names('alice');
  await page.submit();
  await page.refresh();
  assert.match(page.byId('share-status').textContent, /Could not grant access/);
  assert.equal(page.byId('share-add-input').value, 'alice');
  await page.mode('shared');
  await page.refresh();
  assert.match(page.byId('share-status').textContent, /Could not change the data mode/);
  assert.equal(page.byId('share-state-own').checked, true);
});
