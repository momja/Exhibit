import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/message-actions.js', import.meta.url));

async function fixture(clipboard) {
  const timers = [];
  const page = loadPageScript([], {
    navigator: { clipboard },
    setTimeout: fn => { timers.push(fn); return timers.length; },
    clearTimeout: () => {}
  });
  const content = { textContent: '  First line\n\n<code> & **markdown**\n' };
  const status = { textContent: '', setAttribute() {}, removeAttribute() {} };
  const message = { querySelector: selector => selector === '[data-message-content]' ? content : status };
  const button = { dataset: { messageAction: 'copy' }, disabled: false, closest: () => message };
  const icon = { closest: () => button };
  await page.loadModule(script);
  return { ...page, content, status, button, icon, timers,
    click: () => page.byId('pane-chat').click(icon) };
}

test('copies only the selected message text, preserving whitespace and literal markup', async () => {
  const copied = [];
  const p = await fixture({ writeText: async text => copied.push(text) });
  await p.click();
  assert.deepEqual(copied, [p.content.textContent]);
  assert.equal(p.status.textContent, 'Copied');
  assert.equal(p.button.disabled, false);
  p.timers.at(-1)();
  assert.equal(p.status.textContent, '');
  // Deltas can continue after copying; a subsequent copy reads current content.
  p.content.textContent += 'Next delta';
  await p.click();
  assert.equal(copied[1], p.content.textContent);
});

test('clipboard denial leaves a useful error and allows retry', async () => {
  let denied = true;
  const p = await fixture({ writeText: async () => { if (denied) throw Error('denied'); } });
  await p.click();
  assert.match(p.status.textContent, /Could not copy/);
  assert.equal(p.button.disabled, false);
  denied = false;
  await p.click();
  assert.equal(p.status.textContent, 'Copied');
});

test('the delegated listener handles messages inserted later and ignores unknown actions', async () => {
  const copied = [];
  const p = await fixture({ writeText: async text => copied.push(text) });
  p.button.dataset.messageAction = 'unknown';
  await p.click();
  assert.equal(copied.length, 0);
  p.button.dataset.messageAction = 'copy';
  p.content.textContent = 'Historical response moved into live chat';
  await p.click();
  assert.deepEqual(copied, [p.content.textContent]);
});

test('HTTP fallback copies synchronously and restores focus and selection on failure', async () => {
  const p = await fixture(undefined);
  const field = { style: {}, setAttribute() {}, select() { this.selected = true; }, remove() { this.removed = true; } };
  p.context.document.createElement = () => field;
  let restored = false;
  let refocused = false;
  Object.defineProperty(p.context.document, 'activeElement', { get: () => ({ focus() { refocused = true; } }) });
  p.context.window.getSelection = () => ({ rangeCount: 0, removeAllRanges() { restored = true; } });
  p.context.document.execCommand = command => { assert.equal(command, 'copy'); return false; };
  await p.click();
  assert.equal(field.value, p.content.textContent);
  assert.equal(field.selected, true);
  assert.equal(field.removed, true);
  assert.equal(restored, true);
  assert.equal(refocused, true);
  assert.match(p.status.textContent, /Could not copy/);
  p.context.document.execCommand = () => true;
  await p.click();
  assert.equal(p.status.textContent, 'Copied');
});
