import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript, nativeDialog } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/modal.js', import.meta.url));
async function fixture() {
  const page = loadPageScript([], { CustomEvent });
  const save = page.byId('save');
  const cancel = page.byId('cancel');
  const unavailable = page.byId('unavailable');
  save.disabled = cancel.disabled = false;
  unavailable.disabled = true;
  const dialog = nativeDialog(page, page.byId('dialog'), [save, cancel, unavailable]);
  const { modalFor } = await page.loadModule(script);
  return { ...page, save, cancel, unavailable, dialog, modal: modalFor(dialog), modalFor };
}

test('the stable handle opens synchronously, without a controller lookup or readiness event', async () => {
  const p = await fixture();
  const trigger = p.byId('trigger');
  trigger.isConnected = true;
  assert.equal(p.modalFor(p.dialog), p.modal);
  assert.equal(p.modal.show(trigger), true);
  assert.equal(p.dialog.open, true);
  assert.equal(p.modal.show(), false);
  assert.equal(p.modal.close(), true);
  assert.equal(p.dialog.open, false);
  assert.equal(p.document.activeElement, trigger);
});

test('pending covers duplicate calls and every dismissal, and restores existing disabled controls and input', async () => {
  const p = await fixture();
  p.modal.show();
  const field = p.byId('unsaved');
  field.value = 'unsaved input';
  let finish;
  let calls = 0;
  const first = p.modal.run(() => { calls++; return new Promise(resolve => { finish = resolve; }); });
  assert.equal(calls, 1, 'operation starts in the original call stack');
  assert.equal(p.modal.pending, true);
  assert.equal(p.save.disabled, true);
  assert.equal(p.cancel.disabled, true);
  assert.equal(p.modal.close(), false);
  const cancel = new Event('cancel', { cancelable: true });
  await p.dialog.dispatchEvent(cancel);
  assert.equal(cancel.defaultPrevented, true);
  assert.equal(p.dialog.open, true);
  assert.equal(await p.modal.run(() => { calls++; }), undefined);
  assert.equal(calls, 1);
  finish('saved');
  assert.equal(await first, 'saved');
  assert.equal(p.modal.pending, false);
  assert.equal(p.save.disabled, false);
  assert.equal(p.cancel.disabled, false);
  assert.equal(p.unavailable.disabled, true);
  assert.equal(field.value, 'unsaved input');
});

test('failed operations keep feature errors and permit a retry; the modal never consumes errors', async () => {
  const p = await fixture();
  p.modal.show();
  const error = new Error('feature error');
  await assert.rejects(p.modal.run(() => { throw error; }), error);
  assert.equal(p.dialog.open, true);
  assert.equal(p.modal.pending, false);
  assert.equal(p.save.disabled, false);
  assert.equal(await p.modal.run(async () => 'retry'), 'retry');
});

test('event-driven work keeps the dialog pending until its completion event', async () => {
  const p = await fixture();
  p.modal.show();
  p.dialog.addEventListener('feature:started', () => { p.modal.pending = true; });
  p.dialog.addEventListener('feature:finished', () => { p.modal.pending = false; });
  await p.dialog.dispatchEvent(new Event('feature:started'));
  await p.dialog.dispatchEvent(new Event('feature:started'));
  assert.equal(p.save.disabled, true);
  assert.equal(p.modal.close(), false);
  assert.equal(await p.modal.run(() => assert.fail('duplicate operation')), undefined);
  await p.dialog.dispatchEvent(new Event('feature:finished'));
  assert.equal(p.modal.pending, false);
  assert.equal(p.save.disabled, false);
  assert.equal(p.cancel.disabled, false);
  assert.equal(p.unavailable.disabled, true);
  assert.equal(p.modal.close(), true);
});

test('two handles have independent pending state and feature-specific dismissal guards remain optional', async () => {
  const p = await fixture();
  const second = nativeDialog(p, p.byId('second'));
  const other = p.modalFor(second);
  p.modal.show();
  other.show();
  const guard = event => event.preventDefault();
  second.addEventListener('modal:before-close', guard);
  assert.equal(other.close(), false);
  second.removeEventListener('modal:before-close', guard);
  await p.modal.run(async () => {
    assert.equal(other.pending, false);
    assert.equal(other.close(), true);
  });
  assert.equal(p.dialog.open, true);
});

test('replacing focused content during a pending operation restores focus inside the open dialog', async () => {
  const p = await fixture();
  const field = p.byId('replacement');
  p.dialog.querySelector = selector => selector === '[autofocus]' ? field : null;
  p.dialog.contains = element => element === field || element === p.dialog;
  p.modal.show();
  await p.modal.run(async () => { p.byId('outside').focus(); });
  assert.equal(p.document.activeElement, field);
  p.dialog.querySelector = () => null;
  await p.modal.run(async () => { p.byId('outside').focus(); });
  assert.equal(p.document.activeElement, p.dialog, 'no surviving field falls back to the dialog');
});
