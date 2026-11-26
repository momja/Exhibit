/* Drive the artifact bar's shipped module (Exh-i8q4) through its DOM events
 * and the frame–host postMessage boundary (docs/artifact-bar.md §3, §4, §7).
 * Geometry has its own suite; these cover state between events. */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const PIP_JS = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery/pip.js');

async function loadControls({ height = 800, reducedMotion = false } = {}) {
  const listeners = {};
  const frames = [];
  const timers = new Map();
  let nextTimer = 1;
  const observers = [];
  const removed = [];
  const page = loadPageScript([], {
    innerWidth: 1280, innerHeight: height,
    localStorage: { removeItem: (key) => removed.push(key) },
    matchMedia: () => ({ matches: reducedMotion }),
    addEventListener(type, fn) { (listeners[type] ||= []).push(fn); },
    removeEventListener(type, fn) { listeners[type] = (listeners[type] || []).filter(listener => listener !== fn); },
    requestAnimationFrame: (fn) => frames.push(fn),
    setTimeout: (fn) => { const id = nextTimer++; timers.set(id, fn); return id; },
    clearTimeout: (id) => timers.delete(id),
    ResizeObserver: class {
      constructor(fn) { this.fn = fn; this.live = true; observers.push(this); }
      observe() {}
      disconnect() { this.live = false; }
    },
    getComputedStyle: (el) => ({
      getPropertyValue: (name) => name === '--pip-gap' ? (el.classList.contains('pip-compact') ? '4px' : '12px') : ''
    })
  }, {
    'pip-info': { hidden: true, offsetWidth: 330 },
    'pip-menu': { hidden: true, offsetWidth: 236 }
  });
  let active = page.document.body;
  Object.defineProperty(page.document, 'activeElement', { get: () => active });
  const baseQuery = page.document.querySelector;
  page.document.querySelector = (sel) => sel === '.panels > iframe' ? page.frame : baseQuery(sel);
  const body = page.document.body;
  const bodyStyle = {};
  body.style.setProperty = (key, value) => { bodyStyle[key] = value; };
  body.style.removeProperty = (key) => { delete bodyStyle[key]; };
  body.classList.add('pip-inset');
  const rect = (x, y, width, h) => ({ left: x, top: y, right: x + width, bottom: y + h, width, height: h });
  const pill = page.byId('pip');
  const pillListeners = {};
  // Listeners on the bar keep their capture flag, so a test can deliver a
  // click to the capture phase first and see whether it was stopped.
  pill.addEventListener = (type, fn, capture) => { (pillListeners[type] ||= []).push({ fn, capture: capture === true }); };
  pill.removeEventListener = (type, fn) => { pillListeners[type] = (pillListeners[type] || []).filter(l => l.fn !== fn); };
  pill.getBoundingClientRect = () => pill.classList.contains('pip-compact') ? rect(560, height - 4 - 30, 160, 30) : rect(475, height - 12 - 46, 330, 46);
  const ids = ['pip-title', 'pip-more', 'pip-info', 'pip-menu', 'first-action', 'pip-edit'];
  const firstAction = page.byId('first-action');
  for (const id of ids) {
    const el = page.byId(id);
    el.focus = () => { active = el; };
    el.getBoundingClientRect ||= () => rect(765, height - 12 - 41, 36, 36);
    el.setAttribute = (key, value) => { el[key] = value; };
    el.contains = (other) => other === el || (id === 'pip-menu' && other === firstAction);
  }
  pill.contains = (other) => ['pip-title', 'pip-more', 'pip-edit'].some(id => page.byId(id) === other);
  for (const id of ['pip-info', 'pip-menu']) {
    const panel = page.byId(id);
    panel.style.setProperty = (key, value) => { panel.style[key] = value; };
    panel.querySelector = () => id === 'pip-menu' ? firstAction : null;
  }
  const module = await page.loadModule(PIP_JS);
  const dispose = module.connectFloatingHeader({ querySelector: selector => page.byId(selector.slice(1)) });

  // A click on the bar: capture listeners on the bar, then (unless stopped)
  // the target's own. `pointer` presses first, as a finger or mouse does.
  async function clickBar(target, { pointer = false, pointerType = 'touch' } = {}) {
    if (pointer) for (const l of pillListeners.pointerdown || []) l.fn({ type: 'pointerdown', pointerType, target });
    let stopped = false, prevented = false;
    const event = { type: 'click', target, preventDefault() { prevented = true; }, stopPropagation() { stopped = true; } };
    for (const l of (pillListeners.click || []).filter(l => l.capture)) l.fn(event);
    if (!stopped && target !== pill) await target.click(target);
    return { stopped, prevented };
  }
  return {
    ...page, dispose, pill, removed, bodyStyle, observers, timers,
    compact: () => pill.classList.contains('pip-compact'),
    setActive: (el) => { active = el; },
    fire: (type, event = {}) => (listeners[type] || []).forEach(fn => fn({ type, ...event })),
    firePill: (type, event = {}) => (pillListeners[type] || []).forEach(l => l.fn({ type, ...event })),
    pillListenerCount: () => Object.values(pillListeners).reduce((n, list) => n + list.length, 0),
    windowListenerCount: () => Object.values(listeners).reduce((n, list) => n + list.length, 0),
    flushFrames: () => frames.splice(0).forEach(fn => fn()),
    runTimers: () => { const due = [...timers.values()]; timers.clear(); due.forEach(fn => fn()); },
    bar: async (data, source = page.frame.contentWindow) => (listeners.message || []).forEach(fn => fn({ type: 'message', data: { artifactId: 'a1', ...data }, source, origin: 'null' })),
    heights: () => page.frame.contentWindow.posted.filter(m => '__avBarHeight' in m).map(m => m.__avBarHeight),
    clickBar
  };
}

test('scrolling down or using the artifact compacts the bar', async () => {
  for (const message of [{ __avBar: 'scroll', dir: 'down' }, { __avBar: 'use' }]) {
    const page = await loadControls();
    assert.equal(page.compact(), false);
    await page.bar(message);
    assert.equal(page.compact(), true, JSON.stringify(message));
  }
});

test('scrolling up or reaching either end brings the full bar back', async () => {
  for (const dir of ['up', 'top', 'end']) {
    const page = await loadControls();
    await page.bar({ __avBar: 'scroll', dir: 'down' });
    await page.bar({ __avBar: 'scroll', dir });
    assert.equal(page.compact(), false, dir);
  }
});

test('messages from anything but the artifact frame are ignored', async () => {
  const page = await loadControls();
  await page.bar({ __avBar: 'scroll', dir: 'down' }, { postMessage() {} });
  await page.bar({ __avBar: 'use' }, page.window);
  await page.bar({ __avBar: 'mode', mode: 'overlay' }, { postMessage() {} });
  assert.equal(page.compact(), false);
  assert.equal(page.document.body.classList.contains('pip-inset'), true);
});

test('the bar does not compact with a popup open or focus inside it', async () => {
  const page = await loadControls();
  await page.byId('pip-more').click();
  await page.bar({ __avBar: 'scroll', dir: 'down' });
  assert.equal(page.compact(), false, 'a popup holds the bar open');

  const focused = await loadControls();
  focused.setActive(focused.byId('pip-edit'));
  await focused.bar({ __avBar: 'scroll', dir: 'down' });
  assert.equal(focused.compact(), false, 'focus inside the bar holds it open');
});

test('a press inside the artifact closes an open popup and compacts the bar', async () => {
  const page = await loadControls();
  await page.byId('pip-more').click();
  page.setActive(page.frame);
  await page.bar({ __avBar: 'use' });
  assert.equal(page.byId('pip-menu').hidden, true);
  assert.equal(page.compact(), true);
});

test('a pointer tap on the compact strip only expands it', async () => {
  const page = await loadControls();
  await page.bar({ __avBar: 'use' });
  // The title has pointer-events:none while compact, so the tap lands on the bar.
  const { stopped, prevented } = await page.clickBar(page.pill, { pointer: true });
  assert.equal(page.compact(), false);
  // A tap that never became a click still expanded the bar on the press.
  await page.bar({ __avBar: 'use' });
  page.firePill('pointerdown', { pointerType: 'touch' });
  assert.equal(page.compact(), false, 'the press alone expands the strip');
  assert.equal(stopped && prevented, true, 'the click goes no further');
  assert.equal(page.byId('pip-info').hidden, true, 'the info card stays shut');
});

test('a click with no press behind it expands the bar and activates the control', async () => {
  // Some screen-reader and voice-control activations deliver a click without
  // focus or a pointer press (docs/artifact-bar.md §6.1).
  const page = await loadControls();
  await page.bar({ __avBar: 'use' });
  const { stopped } = await page.clickBar(page.byId('pip-title'));
  assert.equal(stopped, false);
  assert.equal(page.compact(), false);
  assert.equal(page.byId('pip-info').hidden, false, 'the card opens on the first activation');
});

test('a pointer press on the full bar is not swallowed', async () => {
  const page = await loadControls();
  const { stopped } = await page.clickBar(page.byId('pip-title'), { pointer: true });
  assert.equal(stopped, false);
  assert.equal(page.byId('pip-info').hidden, false);
});

test('focusing a compacted control expands the bar', async () => {
  const page = await loadControls();
  await page.bar({ __avBar: 'use' });
  page.firePill('focusin', { target: page.byId('pip-edit') });
  assert.equal(page.compact(), false);
});

test('hover expands the strip and leaving compacts it again after a grace period', async () => {
  const page = await loadControls();
  await page.bar({ __avBar: 'use' });
  page.firePill('pointerenter', { pointerType: 'mouse' });
  assert.equal(page.compact(), false);
  page.firePill('pointerleave', { pointerType: 'mouse' });
  assert.equal(page.compact(), false, 'not at once');
  page.runTimers();
  assert.equal(page.compact(), true);

  // Pressing something in the bar while it is open keeps it open.
  page.firePill('pointerenter', { pointerType: 'mouse' });
  page.firePill('pointerdown', { pointerType: 'mouse' });
  page.firePill('pointerleave', { pointerType: 'mouse' });
  page.runTimers();
  assert.equal(page.compact(), false);
});

test('touch never counts as hover', async () => {
  const page = await loadControls();
  await page.bar({ __avBar: 'use' });
  page.firePill('pointerenter', { pointerType: 'touch' });
  assert.equal(page.compact(), true);
});

test('band mode tells the frame 0 and reserves the band; overlay drops it and sends heights', async () => {
  const page = await loadControls();
  assert.equal(page.document.body.classList.contains('pip-inset'), true);
  assert.equal(page.bodyStyle['--pip-reserve'], (46 + 12 + 12) + 'px', 'bar height plus the gap on either side');
  assert.deepEqual(page.heights(), [0]);

  await page.bar({ __avBar: 'use' });
  assert.equal(page.bodyStyle['--pip-reserve'], (30 + 4 + 4) + 'px');
  assert.equal(page.heights().at(-1), 0);

  await page.bar({ __avBar: 'mode', mode: 'overlay' });
  assert.equal(page.document.body.classList.contains('pip-inset'), false);
  assert.equal(page.heights().at(-1), 38);
  await page.bar({ __avBar: 'scroll', dir: 'up' });
  assert.equal(page.heights().at(-1), 70);

  // A later announcement (the frame reloaded) re-decides.
  await page.bar({ __avBar: 'mode', mode: 'band' });
  assert.equal(page.document.body.classList.contains('pip-inset'), true);
  assert.equal(page.heights().at(-1), 0);
});

test('the band follows the bar whenever it resizes', async () => {
  const page = await loadControls();
  const pill = page.pill;
  pill.getBoundingClientRect = () => ({ top: 800 - 12 - 92, height: 92 });
  page.observers[0].fn();
  assert.equal(page.bodyStyle['--pip-reserve'], (92 + 24) + 'px', 'larger text grows the band with the bar');
});

test('under reduced motion the band keeps its full height while the bar shrinks', async () => {
  const page = await loadControls({ reducedMotion: true });
  await page.bar({ __avBar: 'use' });
  assert.equal(page.compact(), true);
  assert.equal(page.bodyStyle['--pip-reserve'], '70px', 'the artifact is never resized by a state change');
});

test('a short window starts compact', async () => {
  const page = await loadControls({ height: 256 });
  assert.equal(page.compact(), true);
});

test('connecting asks the frame for its mode, once', async () => {
  const page = await loadControls();
  const asks = page.frame.contentWindow.posted.filter(m => m.ask === true);
  assert.equal(asks.length, 1);
  await page.bar({ __avBar: 'use' });
  assert.equal(page.frame.contentWindow.posted.filter(m => m.ask === true).length, 1, 'later heights do not ask again');
});

test('the shipped pill\'s saved placement is cleared on connect', async () => {
  const page = await loadControls();
  assert.deepEqual(page.removed, ['exhibit.pipControls']);
});

for (const [triggerId, panelId] of [['pip-more', 'pip-menu'], ['pip-title', 'pip-info']]) {
  test(`${panelId} opens upward from the bar and closes when focus enters the artifact frame`, async () => {
    const page = await loadControls();
    await page.byId(triggerId).click();
    const panel = page.byId(panelId);
    assert.equal(panel.hidden, false);
    assert.equal(panel.style.bottom, (12 + 46 + 8) + 'px');
    assert.equal(panel.style.top, '');

    // The host loses focus first; activeElement identifies the iframe after
    // the browser finishes moving focus into its separate document.
    page.fire('blur');
    page.setActive(page.frame);
    page.flushFrames();
    assert.equal(panel.hidden, true);
    assert.equal(page.byId(triggerId)['aria-expanded'], 'false');
    assert.equal(page.document.activeElement, page.frame, 'dismissal must not steal focus from the tool');
  });
}

test('switching to browser chrome does not mistake it for focusing the artifact', async () => {
  const page = await loadControls();
  await page.byId('pip-more').click();
  page.fire('blur');
  page.setActive(page.document.body);
  page.flushFrames();
  assert.equal(page.byId('pip-menu').hidden, false);
});

test('Escape closes a popup and returns focus to its trigger', async () => {
  const page = await loadControls();
  await page.byId('pip-more').click();
  page.fire('keydown', { key: 'Escape' });
  assert.equal(page.byId('pip-menu').hidden, true);
  assert.equal(page.document.activeElement, page.byId('pip-more'));
});

test('the disposer leaves no classes, listeners, timers or observers behind', async () => {
  const page = await loadControls();
  await page.bar({ __avBar: 'use' });
  page.firePill('pointerenter', { pointerType: 'mouse' });
  page.firePill('pointerleave', { pointerType: 'mouse' });
  assert.equal(page.timers.size, 1);
  page.dispose();
  assert.equal(page.timers.size, 0, 'the hover timer is cleared');
  assert.equal(page.observers[0].live, false);
  assert.equal(page.compact(), false);
  assert.equal(page.document.body.classList.contains('pip-inset'), false);
  assert.equal('--pip-reserve' in page.bodyStyle, false);
  assert.equal(page.pillListenerCount(), 0);
  assert.equal(page.windowListenerCount(), 0);
  await page.byId('pip-more').click();
  assert.equal(page.byId('pip-menu').hidden, true);
});
