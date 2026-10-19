/* Drive the floating controls' shipped module through its DOM events. Geometry
 * has its own suite; these cover focus and animation state between events. */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const PIP_JS = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery/pip.js');

async function loadControls(saved = { mode: 'corner', corner: 'bl' }) {
  const listeners = {};
  const frames = [];
  const timers = [];
  let placement = JSON.stringify(saved);
  const page = loadPageScript([], {
    innerWidth: 1280, innerHeight: 800,
    performance: { now: () => 0 },
    localStorage: {
      getItem: () => placement,
      setItem: (key, value) => { placement = value; }
    },
    addEventListener(type, fn) { (listeners[type] ||= []).push(fn); },
    requestAnimationFrame: (fn) => frames.push(fn),
    setTimeout: (fn) => timers.push(fn),
    ResizeObserver: class { observe() {} },
    getComputedStyle: (el) => ({
      getPropertyValue: () => '12px',
      translate: el.renderedTranslate || el.style.translate
    })
  }, {
    'pip': { offsetWidth: 330, offsetHeight: 40 },
    'pip-grip': { offsetLeft: 45, offsetWidth: 22, setPointerCapture() {} },
    'pip-tab': { hidden: true, offsetHeight: 60, setPointerCapture() {} },
    'pip-info': { hidden: true, offsetWidth: 330 },
    'pip-menu': { hidden: true, offsetWidth: 236 }
  });
  let active = page.document.body;
  Object.defineProperty(page.document, 'activeElement', { get: () => active });
  const rect = (x, y, width, height) => ({ left: x, top: y, right: x + width, bottom: y + height, width, height });
  const pill = page.byId('pip');
  pill.getBoundingClientRect = () => {
    const [x, y] = pill.style.translate.split(' ').map(parseFloat);
    return rect(x, y, 330, 40);
  };
  const firstAction = page.byId('first-action');
  for (const id of ['pip-grip', 'pip-tab', 'pip-title', 'pip-more', 'pip-info', 'first-action']) {
    const el = page.byId(id);
    el.focus = () => { active = el; };
    el.getBoundingClientRect ||= () => rect(305, 753, 30, 30);
    el.setAttribute = (key, value) => { el[key] = value; };
    el.contains = (other) => other === el || (id === 'pip-menu' && other === firstAction);
  }
  for (const id of ['pip-info', 'pip-menu']) {
    const panel = page.byId(id);
    panel.style.setProperty = (key, value) => { panel.style[key] = value; };
    panel.querySelector = () => id === 'pip-menu' ? firstAction : null;
    panel.contains = (other) => other === panel || (id === 'pip-menu' && other === firstAction);
  }
  await page.loadModule(PIP_JS);
  return {
    ...page,
    setActive: (el) => { active = el; },
    fire: (type, event = {}) => (listeners[type] || []).forEach(fn => fn({ type, ...event })),
    flushFrames: () => frames.splice(0).forEach(fn => fn()),
    flushTimers: () => timers.splice(0).forEach(fn => fn()),
    placement: () => JSON.parse(placement)
  };
}

for (const [triggerId, panelId] of [['pip-more', 'pip-menu'], ['pip-title', 'pip-info']]) {
  test(`${panelId} closes when focus enters the artifact frame`, async () => {
    const page = await loadControls();
    await page.byId(triggerId).click();
    assert.equal(page.byId(panelId).hidden, false);

    // The host loses focus first; activeElement identifies the iframe after
    // the browser finishes moving focus into its separate document.
    page.fire('blur');
    page.setActive(page.frame);
    page.flushFrames();
    assert.equal(page.byId(panelId).hidden, true);
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

test('restoring with the keyboard focuses the grip so arrow keys work immediately', async () => {
  const page = await loadControls({ mode: 'stashed', edge: 'left', yFraction: 0.96 });
  page.byId('pip-tab').focus();
  await page.byId('pip-tab').dispatchEvent({ type: 'click', detail: 0 });
  page.flushFrames();

  assert.equal(page.byId('pip-tab').hidden, true);
  assert.equal(page.byId('pip').hidden, false);
  assert.equal(page.document.activeElement, page.byId('pip-grip'));
  await page.document.activeElement.dispatchEvent({ type: 'keydown', key: 'ArrowUp', preventDefault() {} });
  assert.equal(page.placement().corner, 'tl');
});

test('hiding and restoring the controls keeps the keyboard focus on a visible control', async () => {
  const page = await loadControls();
  await page.byId('pip-hide').click();
  page.flushTimers();
  assert.equal(page.document.activeElement, page.byId('pip-tab'));
  await page.byId('pip-tab').dispatchEvent({ type: 'click', detail: 0 });
  page.flushFrames();
  assert.equal(page.document.activeElement, page.byId('pip-grip'));
});

test('grabbing a pill during a snap keeps its rendered position and grab offset', async () => {
  const page = await loadControls();
  const grip = page.byId('pip-grip');
  const pill = page.byId('pip');
  await grip.dispatchEvent({ type: 'keydown', key: 'ArrowRight', preventDefault() {} });
  // The snap targets x=938, but the browser is still drawing it at x=400.
  pill.renderedTranslate = '400px 748px';
  await grip.dispatchEvent({ type: 'pointerdown', button: 0, pointerId: 1,
    clientX: 460, clientY: 768, preventDefault() {} });
  assert.equal(pill.style.translate, '400px 748px');
  page.fire('pointermove', { clientX: 480, clientY: 768 });
  assert.equal(pill.style.translate, '420px 748px', 'moving 20px must move the pill 20px');
  page.fire('pointerup');
  assert.equal(page.document.body.classList.contains('pip-dragging'), false);
});
