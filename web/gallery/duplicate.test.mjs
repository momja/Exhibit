import { test } from 'node:test';
import assert from 'node:assert/strict';
import { webcrypto } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/index.js', import.meta.url));

async function fixture(respond = async () => Response.json({ id: 'created-copy' }, { status: 201 })) {
  const calls = [];
  const page = loadPageScript([], {
    Event, CustomEvent,
    // Deliberately no randomUUID: self-hosted HTTP has only getRandomValues.
    crypto: { getRandomValues: array => webcrypto.getRandomValues(array) },
    apiFetch: async (path, options) => {
      calls.push({ path, body: JSON.parse(options.body) });
      return respond(calls.length);
    }
  });
  page.window.location.href = 'http://t3code:18380/?q=original';
  const el = page.byId;
  const root = el('gallery');
  const dialog = el('dialog');
  const form = el('form');
  const title = el('title');
  const data = el('data');
  const share = el('share');
  const status = el('status');
  const submit = el('submit');
  const cancel = el('cancel');
  const trigger = el('trigger');
  const search = el('search');
  const query = el('query');
  const newLink = el('new-link');
  query.value = 'original';
  dialog.open = false;
  dialog.showModal = () => { dialog.open = true; };
  dialog.close = () => { dialog.open = false; };
  dialog.getBoundingClientRect = () => ({ left: 100, right: 580, top: 150, bottom: 510 });
  dialog.querySelector = () => form;
  form.elements = { title, copy_data: data, copy_share_settings: share };
  form.querySelector = selector => ({
    '[type="submit"]': submit, '[data-duplicate-cancel]': cancel, '[data-duplicate-status]': status
  })[selector] || null;
  root.querySelector = selector => ({
    '[data-duplicate-dialog]': dialog,
    '[data-gallery-search]': search, '[data-gallery-query]': query,
    '[data-gallery-clear]': el('clear'), '[data-gallery-search-error]': el('search-error'),
    '.card-title[href="/artifacts/created-copy"]': newLink
  })[selector] || null;
  root.contains = () => true;
  search.closest = selector => selector === '[data-gallery]' ? root : null;
  page.document.querySelectorAll = () => [root];
  let refreshes = 0;
  search.addEventListener('exhibit:gallery-search', () => refreshes++);
  await page.loadModule(script);
  const open = () => root.dispatchEvent({ type: 'exhibit:duplicate', detail: {
    artifactID: 'original-id', title: 'Original <title>', trigger
  } });
  const save = () => form.dispatchEvent({ type: 'submit', preventDefault() {} });
  return { ...page, root, dialog, form, title, data, share, submit, cancel, status, trigger, query, newLink, calls, open, save, refreshes: () => refreshes };
}

test('opens from a menu action event with editable title and unchecked options; cancel restores menu focus', async () => {
  const p = await fixture();
  await p.open();
  assert.equal(p.dialog.open, true);
  assert.equal(p.title.value, 'Original <title> (copy)');
  assert.equal(p.title.selected, true);
  assert.equal(p.data.checked, false);
  assert.equal(p.share.checked, false);
  await p.cancel.click();
  assert.equal(p.dialog.open, false);
  assert.equal(p.trigger.focused, true);
  assert.equal(p.calls.length, 0);
  await p.open();
  let prevented = false;
  await p.dialog.dispatchEvent({ type: 'cancel', preventDefault() { prevented = true; } });
  assert.equal(prevented, true);
  assert.equal(p.dialog.open, false);
});

test('saves selected options, stays on gallery and requests a fresh first page even after renaming out of the search', async () => {
  const p = await fixture();
  await p.open();
  p.title.value = '  Renamed copy  ';
  p.data.checked = true;
  p.share.checked = true;
  await p.save();
  assert.equal(p.calls.length, 1);
  assert.equal(p.calls[0].path, '/api/artifacts/original-id/duplicate');
  assert.deepEqual({ ...p.calls[0].body, request_id: undefined }, {
    title: 'Renamed copy', copy_data: true, copy_share_settings: true, request_id: undefined
  });
  assert.match(p.calls[0].body.request_id, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  assert.equal(p.dialog.open, false);
  assert.equal(p.refreshes(), 1);
  assert.equal(p.query.value, '');
  assert.equal(p.window.location.href, 'http://t3code:18380/?q=original', 'the script does not navigate or reload');
  await p.root.dispatchEvent({ type: 'htmx:after:swap' });
  assert.equal(p.newLink.focused, true);
});

test('dialog padding preserves edits, while clicks outside each edge dismiss the modal', async () => {
  const p = await fixture();
  for (const [inside, outside] of [
    [[105, 330], [95, 330]],
    [[575, 330], [585, 330]],
    [[340, 155], [340, 145]],
    [[340, 505], [340, 515]]
  ]) {
    await p.open();
    p.title.value = 'My edited title';
    p.data.checked = true;
    await p.dialog.dispatchEvent({ type: 'click', target: p.dialog, clientX: inside[0], clientY: inside[1] });
    assert.equal(p.dialog.open, true, 'padding belongs to the dialog');
    assert.equal(p.title.value, 'My edited title');
    assert.equal(p.data.checked, true);
    await p.dialog.dispatchEvent({ type: 'click', target: p.dialog, clientX: outside[0], clientY: outside[1] });
    assert.equal(p.dialog.open, false, 'the backdrop still dismisses');
    assert.equal(p.trigger.focused, true);
  }
  assert.equal(p.calls.length, 0);
});

test('an uncertain failure retries the same identity and options, with no duplicate submissions while busy', async () => {
  let resolve;
  const p = await fixture(call => call === 1 ? new Promise(done => { resolve = done; }) : Response.json({ id: 'created-copy' }));
  await p.open();
  const saving = p.save();
  await p.save();
  assert.equal(p.calls.length, 1);
  assert.equal(p.submit.disabled, true);
  await p.cancel.click();
  assert.equal(p.dialog.open, true);
  resolve(new Response('temporary failure', { status: 500 }));
  await saving;
  assert.equal(p.dialog.open, true);
  assert.equal(p.submit.disabled, false);
  assert.equal(p.title.disabled, true, 'an uncertain retry retains its original payload');
  assert.match(p.status.textContent, /Could not confirm/);
  await p.save();
  assert.deepEqual(p.calls[1].body, p.calls[0].body);
  assert.equal(p.dialog.open, false);
  assert.equal(p.refreshes(), 1);
});

test('a quota refusal keeps the modal open and allows an edited retry with a new identity', async () => {
  const p = await fixture(call => call === 1 ? Response.json({ error: 'Storage quota exceeded' }, { status: 413 }) : Response.json({ id: 'created-copy' }));
  await p.open();
  await p.save();
  assert.equal(p.dialog.open, true);
  assert.equal(p.title.disabled, false);
  assert.equal(p.status.textContent, 'Storage quota exceeded');
  assert.equal(p.refreshes(), 0);
  p.title.value = 'Different title';
  await p.save();
  assert.notEqual(p.calls[0].body.request_id, p.calls[1].body.request_id);
  assert.equal(p.calls[1].body.title, 'Different title');
});

test('network failures leave a retryable modal and preserve the request identity', async () => {
  const p = await fixture(call => { if (call === 1) throw Error('lost response'); return Response.json({ id: 'created-copy' }); });
  await p.open();
  await p.save();
  assert.equal(p.dialog.open, true);
  assert.equal(p.submit.disabled, false);
  await p.save();
  assert.equal(p.calls[1].body.request_id, p.calls[0].body.request_id);
});

test('closing an uncertain request reconciles the gallery without repeating the mutation', async () => {
  const p = await fixture(() => { throw Error('lost response'); });
  await p.open();
  await p.save();
  await p.cancel.click();
  assert.equal(p.dialog.open, false);
  assert.equal(p.calls.length, 1);
  assert.equal(p.refreshes(), 1);
});

test('whitespace-only titles receive feedback without issuing a mutation', async () => {
  const p = await fixture();
  await p.open();
  p.title.value = '   ';
  await p.save();
  assert.equal(p.calls.length, 0);
  assert.match(p.status.textContent, /Enter a title/);
});
