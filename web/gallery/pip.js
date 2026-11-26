/* The artifact bar (Exh-i8q4, docs/artifact-bar.md).
 *
 * The artifact fills the window and the page's controls are one bar pinned to
 * the bottom centre, like Safari's address bar on iOS. While the artifact is
 * scrolled down or used, the bar shrinks to a strip showing only the title;
 * scrolling back up, reaching either end, tapping it, focusing into it or (with
 * a mouse) pointing at it brings the full controls back.
 *
 * By default the frame ends above the bar (a reserved band, body.pip-inset)
 * and follows it as it changes size, so the bar never covers the artifact. An
 * artifact that opts in with <meta name="exhibit-bar" content="overlay"> gets
 * the whole window instead, and is told the bar's height, which it reads as
 * var(--exhibit-bar).
 *
 * Scrolling happens inside the sandboxed frame, which this page cannot see.
 * The render preamble's bar script posts {__avBar: 'scroll' | 'use' | 'mode'}.
 * Its identity is the frame's window, and the most a forged message can do is
 * resize this bar or the band: the artifact affecting its own presentation.
 */
import { popupPlacement } from './pip-placement.js';
import { moveMenuFocus } from './menu-focus.js';

// Where the shipped pill remembered its corner. Read by nothing now; removed
// once so stale placements do not linger in visitors' storage.
const LEGACY_PLACEMENT_KEY = 'exhibit.pipControls';
// Leaving a bar that hover expanded compacts it again after this long, which
// also forgives a slightly unsteady pointer (WCAG 1.4.13).
const HOVER_LEAVE_MS = 500;
// A click this soon after a press that expanded the compact strip is that
// press's own click, and is swallowed.
const PRESS_CLICK_MS = 800;
// The bar's transitions are 0.32s (components.css); the band follows a little
// longer so it lands on the final position.
const TRANSITION_FOLLOW_MS = 400;
// A window this short (400% zoom on a laptop, a phone in landscape) starts
// compact: the full band would take a third of it.
const SHORT_VIEWPORT_PX = 450;

// The Stimulus controller connects this behavior to one server-rendered
// header and disposes listeners, observers and pending work on disconnect.
export function connectFloatingHeader(root) {
  const listeners = [];
  function listen(target, type, handler, options) {
    target.addEventListener(type, handler, options);
    listeners.push(() => target.removeEventListener(type, handler, options));
  }
  let frameID = 0;
  function nextFrame(callback) {
    frameID = requestAnimationFrame(() => { frameID = 0; callback(); });
  }
  const pill = root.querySelector('#pip');
  const popups = [
    { panel: root.querySelector('#pip-info'), trigger: root.querySelector('#pip-title'), align: 'pill', side: 'left' },
    { panel: root.querySelector('#pip-menu'), trigger: root.querySelector('#pip-more'), align: 'trigger', side: 'right' }
  ];
  try { localStorage.removeItem(LEGACY_PLACEMENT_KEY); } catch (e) { /* storage refused: nothing to clear */ }

  function artifactFrame() { return document.querySelector('.panels > iframe'); }
  function anyPopupOpen() { return popups.some(p => !p.panel.hidden); }
  const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)');

  // ------------------------------------------------------------ band/overlay
  // The space the bar takes from the bottom of the window: from the bar's top
  // edge down, plus the same gap again above it. Measured rather than
  // computed, so larger text (a bigger rem) grows the band with the bar, and
  // it includes the safe-area inset the bar sits above.
  let overlay = false;
  let reserve = 0;
  function measureReserve() {
    const gap = parseFloat(getComputedStyle(pill).getPropertyValue('--pip-gap')) || 0;
    return Math.max(0, Math.round(innerHeight - pill.getBoundingClientRect().top + gap));
  }
  // ask: the frame answers with its mode (on connect, in case the frame
  // announced it before this listener existed).
  function applyBar(ask) {
    document.body.classList.toggle('pip-inset', !overlay);
    // Under reduced motion the band stays at its full height while the bar
    // shrinks, so the artifact is never resized by a state change.
    const frozen = !overlay && compact && reducedMotion?.matches && reserve > 0;
    if (!frozen) reserve = measureReserve();
    document.body.style.setProperty('--pip-reserve', reserve + 'px');
    const frame = artifactFrame();
    // In band mode the frame is told 0, so an artifact that opted in somewhere
    // else still lays out correctly here. The frame's origin is opaque, so '*'.
    if (frame && frame.contentWindow) frame.contentWindow.postMessage(ask === true ? { __avBarHeight: overlay ? reserve : 0, ask: true } : { __avBarHeight: overlay ? reserve : 0 }, '*');
  }
  // The bar's size changes with its state (animated, so on every frame of the
  // transition), with the text size, and when the posture mark gains a glyph.
  const observer = new ResizeObserver(() => applyBar());
  // The bar also moves while it changes state (its gap to the window
  // animates), which a ResizeObserver does not see, so a state change follows
  // the bar on every frame of its transition.
  let followFrame = 0;
  function followTransition() {
    window.cancelAnimationFrame?.(followFrame);
    const until = Date.now() + TRANSITION_FOLLOW_MS;
    const step = () => {
      applyBar();
      followFrame = Date.now() < until ? requestAnimationFrame(step) : 0;
    };
    followFrame = requestAnimationFrame(step);
  }
  observer.observe(pill);

  // ---------------------------------------------------------------- compact
  let compact = false;
  let hoverOpened = false;
  let leaveTimer = 0;
  function setCompact(next) {
    if (next && (anyPopupOpen() || pill.contains(document.activeElement))) return;
    if (next === compact) return;
    compact = next;
    pill.classList.toggle('pip-compact', compact);
    applyBar();
    followTransition();
  }
  function expand() { hoverOpened = false; clearTimeout(leaveTimer); setCompact(false); }

  listen(window, 'message', function (e) {
    const frame = artifactFrame();
    if (!frame || e.source !== frame.contentWindow) return;
    const d = e.data;
    if (!d || typeof d.__avBar !== 'string') return;
    if (d.__avBar === 'mode') {
      overlay = d.mode === 'overlay';
      applyBar();
    } else if (d.__avBar === 'use' || (d.__avBar === 'scroll' && d.dir === 'down')) {
      // A press inside the frame has already taken focus there; the popup it
      // dismisses (window blur) must not hold the bar open.
      if (d.__avBar === 'use' && document.activeElement === frame) closePopups();
      hoverOpened = false;
      setCompact(true);
    } else if (d.__avBar === 'scroll' && (d.dir === 'up' || d.dir === 'top' || d.dir === 'end')) {
      setCompact(false);
    }
  });

  // A pointer press on the compact strip expands it and does nothing else:
  // the title under the finger should not also open the info card. The bar
  // expands on the press itself (a tap does not always end in a click), and
  // only a click that press started, moments later, is swallowed. A click with
  // no press behind it (a screen reader's or voice control's activation)
  // expands the bar and also activates the control, so nobody activates twice
  // without being told why.
  let compactPressAt = -Infinity;
  listen(pill, 'pointerdown', function () {
    hoverOpened = false;
    clearTimeout(leaveTimer);
    if (!compact) return;
    compactPressAt = Date.now();
    setCompact(false);
  }, true);
  listen(pill, 'click', function (e) {
    const fromCompactPress = Date.now() - compactPressAt < PRESS_CLICK_MS;
    compactPressAt = -Infinity;
    if (fromCompactPress) { e.preventDefault(); e.stopPropagation(); return; }
    if (compact) expand();
  }, true);
  // Compacted controls stay in the tab order; focusing one brings them back.
  listen(pill, 'focusin', expand);

  // With a mouse, pointing at the strip brings the controls back, and leaving
  // again tucks them away if hovering was all that opened them.
  listen(pill, 'pointerenter', function (e) {
    if (e.pointerType !== 'mouse') return;
    clearTimeout(leaveTimer);
    if (compact) { setCompact(false); hoverOpened = true; }
  });
  listen(pill, 'pointerleave', function (e) {
    if (e.pointerType !== 'mouse' || !hoverOpened) return;
    leaveTimer = setTimeout(function () { hoverOpened = false; setCompact(true); }, HOVER_LEAVE_MS);
  });

  // ---------------------------------------------------------------- popups
  // The info card (from the title) and the ⋮ menu. One open at a time, both
  // opening upward from the bar.
  function viewport() { return { width: innerWidth, height: innerHeight }; }
  function inset() {
    return parseFloat(getComputedStyle(pill).getPropertyValue('--pip-gap')) || 12;
  }
  function openPopup(popup) {
    closePopups();
    expand();
    const panel = popup.panel;
    panel.hidden = false;
    popup.trigger.setAttribute('aria-expanded', 'true');
    const pillRect = pill.getBoundingClientRect();
    const place = popupPlacement({
      side: popup.side,
      pill: pillRect,
      anchor: popup.align === 'pill' ? pillRect : popup.trigger.getBoundingClientRect(),
      align: popup.align,
      panelWidth: panel.offsetWidth,
      viewport: viewport(),
      inset: inset()
    });
    if (place.width !== null) panel.style.width = place.width + 'px';
    panel.style.maxHeight = place.maxHeight + 'px';
    for (const side of ['top', 'bottom', 'left', 'right']) {
      panel.style[side] = place[side] === '' ? '' : place[side] + 'px';
    }
    panel.style.setProperty('--pip-popup-origin', place.origin);
    (panel.querySelector('a[href],button') && popup.align === 'trigger'
      ? panel.querySelector('a[href],button') : panel).focus({ preventScroll: true });
  }

  function closePopups() {
    for (const popup of popups) {
      if (popup.panel.hidden) continue;
      popup.panel.hidden = true;
      popup.trigger.setAttribute('aria-expanded', 'false');
    }
  }

  function openPopupTrigger() {
    const open = popups.find(function (p) { return !p.panel.hidden; });
    return open ? open.trigger : null;
  }

  for (const popup of popups) {
    listen(popup.trigger, 'click', function () {
      if (popup.panel.hidden) openPopup(popup); else closePopups();
    });
    // A menu item that acts on this page closes the menu; one that navigates
    // leaves the page anyway.
    listen(popup.panel, 'click', function (e) {
      if (e.target.closest('a[href],button')) closePopups();
    });
  }
  listen(document, 'focusin', function (e) {
    const open = popups.find(p => !p.panel.hidden);
    if (open && !open.panel.contains(e.target) && !open.trigger.contains(e.target)) closePopups();
  });
  listen(window, 'pointerdown', function (e) {
    const inside = popups.some(function (p) {
      return p.panel.contains(e.target) || p.trigger.contains(e.target);
    });
    if (!inside) closePopups();
  }, true);
  // Events inside the sandboxed frame do not reach this window. Wait until the
  // focus transfer finishes so activeElement identifies the frame, and dismiss
  // the popup without moving focus away from the artifact.
  listen(window, 'blur', function () {
    nextFrame(function () {
      if (document.activeElement === artifactFrame()) closePopups();
    });
  });
  listen(window, 'keydown', function (e) {
    const trigger = openPopupTrigger();
    if (e.key === 'Escape' && trigger) { closePopups(); trigger.focus(); return; }
    const menu = popups.find(p => p.align === 'trigger');
    if (e.target !== menu.trigger && (menu.panel.hidden || !menu.panel.contains(e.target))) return;
    const items = [...menu.panel.querySelectorAll('a[href],button')].filter(item => !item.disabled && !item.hidden);
    if (e.target === menu.trigger && ['ArrowDown', 'ArrowUp'].includes(e.key)) {
      e.preventDefault();
      openPopup(menu);
      (e.key === 'ArrowUp' ? items.at(-1) : items[0])?.focus();
    } else if (!menu.panel.hidden && menu.panel.contains(e.target)) {
      if (e.key === 'Tab') { closePopups(); trigger?.focus(); }
      else moveMenuFocus(e, items);
    }
  });
  listen(window, 'resize', function () {
    closePopups();
    applyBar();
  });

  applyBar(true);
  if (innerHeight < SHORT_VIEWPORT_PX) setCompact(true);

  return () => {
    for (const remove of listeners) remove();
    if (frameID) window.cancelAnimationFrame?.(frameID);
    if (followFrame) window.cancelAnimationFrame?.(followFrame);
    clearTimeout(leaveTimer);
    observer.disconnect();
    closePopups();
    pill.classList.remove('pip-compact');
    document.body.classList.remove('pip-inset');
    document.body.style.removeProperty('--pip-reserve');
  };
}
