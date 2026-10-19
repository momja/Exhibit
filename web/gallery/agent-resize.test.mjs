import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/agent-resize.js', import.meta.url));
const storageKey = 'exhibit.agentChatWidth';

async function load({ saved = null, blockedStorage = false } = {}) {
  const values = new Map(saved === null ? [] : [[storageKey, saved]]);
  let onResize, onMediaChange, captured = null;
  const media = { matches: false, addEventListener: (_, fn) => { onMediaChange = fn; } };
  const page = loadPageScript([], {
    matchMedia: () => media,
    ResizeObserver: class { constructor(fn) { onResize = fn; } observe() {} },
    localStorage: {
      getItem: (key) => { if (blockedStorage) throw Error('blocked'); return values.get(key) ?? null; },
      setItem: (key, value) => { if (blockedStorage) throw Error('blocked'); values.set(key, value); }
    }
  });
  const layout = page.byId('agent-layout');
  const divider = page.byId('agent-pane-resizer');
  const attributes = {};
  layout.clientWidth = 1280;
  layout.style.setProperty = (key, value) => { layout.style[key] = value; };
  divider.offsetWidth = 8;
  divider.setAttribute = (key, value) => { attributes[key] = value; };
  divider.hasPointerCapture = (id) => captured === id;
  divider.setPointerCapture = (id) => { captured = id; };
  divider.releasePointerCapture = () => { captured = null; };
  const width = () => parseFloat(layout.style['--agent-chat-width']);
  page.byId('pane-chat').getBoundingClientRect = () => ({ width: width() });
  await page.loadModule(script);
  const event = (type, props = {}) => divider.dispatchEvent({
    type, pointerId: 1, button: 0, isPrimary: true, clientX: width(), preventDefault() {}, ...props
  });
  return { ...page, layout, divider, attributes, values, width, event,
    captured: () => captured,
    resize: (size) => { layout.clientWidth = size; onResize(); },
    mobile: (matches) => { media.matches = matches; onMediaChange(); }
  };
}

test('drag across the preview clamps both panes and persists on release', async () => {
  const p = await load();
  await p.event('pointerdown');
  assert.equal(p.captured(), 1);
  assert.equal(p.layout.classList.contains('is-resizing'), true);
  await p.event('pointermove', { pointerId: 2, clientX: 900 });
  assert.equal(p.width(), 460, 'another pointer cannot move the divider');
  await p.event('pointermove', { clientX: 700 });
  assert.equal(p.width(), 700);
  await p.event('pointermove', { clientX: 2000 });
  assert.equal(p.width(), 992);
  assert.equal(p.attributes['aria-valuenow'], '992');
  await p.event('pointermove', { clientX: 0 });
  assert.equal(p.width(), 280);
  await p.event('pointerup');
  assert.equal(p.captured(), null);
  assert.equal(p.layout.classList.contains('is-resizing'), false);
  assert.equal(p.values.get(storageKey), '280');
});

test('keyboard resizing, bounds, reload, and reset', async () => {
  const p = await load({ saved: '600' });
  await p.event('keydown', { key: 'ArrowRight' });
  assert.equal(p.width(), 616);
  await p.event('keydown', { key: 'ArrowLeft', shiftKey: true });
  assert.equal(p.width(), 552);
  assert.equal((await load({ saved: p.values.get(storageKey) })).width(), 552);
  await p.event('keydown', { key: 'Home' });
  assert.equal(p.width(), 280);
  await p.event('keydown', { key: 'End' });
  assert.equal(p.width(), 992);
  await p.event('dblclick');
  assert.equal(p.width(), 460);
  assert.equal(p.values.get(storageKey), '460');
});

test('viewport changes clamp without forgetting the preferred width and mobile stops dragging', async () => {
  const p = await load({ saved: '700' });
  p.resize(641);
  assert.equal(p.width(), 353);
  assert.equal(p.attributes['aria-valuemax'], '353');
  p.resize(1280);
  assert.equal(p.width(), 700);
  await p.event('pointerdown');
  p.mobile(true);
  assert.equal(p.captured(), null);
  assert.equal(p.layout.classList.contains('is-resizing'), false);
  await p.event('pointerdown');
  await p.event('keydown', { key: 'Home' });
  assert.equal(p.width(), 700);
  assert.equal(p.captured(), null);
  p.mobile(false);
  assert.equal(p.width(), 700);
});

test('cancelled capture and window blur restore normal interaction', async () => {
  for (const end of ['pointercancel', 'lostpointercapture', 'blur']) {
    const p = await load();
    await p.event('pointerdown');
    await p.event('pointermove', { clientX: 600 });
    if (end === 'blur') await p.dispatchOnWindow('blur');
    else await p.event(end);
    assert.equal(p.layout.classList.contains('is-resizing'), false);
    await p.event('pointermove', { clientX: 800 });
    assert.equal(p.width(), 600);
  }
});

test('unavailable storage and corrupt preferences leave resizing usable', async () => {
  for (const options of [{ blockedStorage: true }, { saved: 'garbage' }, { saved: '-1' }, { saved: 'Infinity' }]) {
    const p = await load(options);
    assert.equal(p.width(), 460);
    await p.event('keydown', { key: 'ArrowRight' });
    assert.equal(p.width(), 476);
  }
});
