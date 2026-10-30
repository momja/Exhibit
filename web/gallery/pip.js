/* The shared floating header (Exh-ghuv).
 *
 * The artifact fills the window and the page's chrome is one pill over it,
 * which behaves like picture-in-picture on macOS and iOS: drag it by the grip,
 * let go, and it springs to a corner; let go against a side edge and it is put
 * away, leaving a tab that brings it back. Its placement is remembered on this
 * origin, so it stays where the visitor left it across artifacts.
 *
 * The rules (which corner, when to stash, where a popup goes) are in
 * pip-placement.js. This module measures the page, asks, and applies.
 */
import {
  VELOCITY_WINDOW_MS, arrowMove, cornerForTab, cornerPosition, isLeft,
  pointerVelocity, popupPlacement, releaseCorner, stashEdgeAt
} from './pip-placement.js';

const STORE_KEY = 'exhibit.pipControls';
const DRAG_SLOP_PX = 4;      // less movement than this is a press, not a drag
const TAB_PULL_PX = 6;       // pulling a tab this far turns the press into a drag
const LEAVE_MS = 220;        // the pill's slide off-screen, matched in components.css

// The Stimulus controller connects this behavior to one server-rendered
// header and disposes listeners, observers and pending work on disconnect.
export function connectFloatingHeader(root) {
  let connected = true;
  const listeners = [];
  const timers = new Set();
  const frames = new Set();
  function listen(target, type, handler, options) {
    target.addEventListener(type, handler, options);
    listeners.push(() => target.removeEventListener(type, handler, options));
  }
  function later(callback, delay) {
    const id = setTimeout(() => { timers.delete(id); if (connected) callback(); }, delay);
    timers.add(id);
    return id;
  }
  function nextFrame(callback) {
    const id = requestAnimationFrame(() => { frames.delete(id); if (connected) callback(); });
    frames.add(id);
    return id;
  }
  const pill = root.querySelector('#pip');
  const grip = root.querySelector('#pip-grip');
  const tab = root.querySelector('#pip-tab');
  const announcer = root.querySelector('#pip-announce');
  const popups = [
    { panel: root.querySelector('#pip-info'), trigger: root.querySelector('#pip-title'), align: 'pill' },
    { panel: root.querySelector('#pip-menu'), trigger: root.querySelector('#pip-more'), align: 'trigger' }
  ];

  function viewport() { return { width: innerWidth, height: innerHeight }; }
  function pillSize() { return { width: pill.offsetWidth, height: pill.offsetHeight }; }
  function inset() {
    return parseFloat(getComputedStyle(pill).getPropertyValue('--pip-inset')) || 12;
  }
  function announce(text) { announcer.textContent = text; }

  // ---------------------------------------------------------------- placement
  // {mode: 'corner', corner: 'bl'} or {mode: 'stashed', edge: 'left', yFraction}.
  let placement = loadPlacement();
  let position = { x: 0, y: 0 };

  function loadPlacement() {
    try {
      const saved = JSON.parse(localStorage.getItem(STORE_KEY));
      if (saved && saved.mode === 'corner' && /^[tb][lr]$/.test(saved.corner)) return saved;
      if (saved && saved.mode === 'stashed' && /^(left|right)$/.test(saved.edge)) return saved;
    } catch (e) { /* storage refused or unreadable: the default is fine */ }
    return { mode: 'corner', corner: 'bl' };
  }
  function savePlacement() {
    try { localStorage.setItem(STORE_KEY, JSON.stringify(placement)); } catch (e) { /* not remembered */ }
  }

  // motion: 'snap' springs, 'leave' slides off-screen, anything else jumps.
  function setPosition(x, y, motion) {
    pill.classList.toggle('pip-snapping', motion === 'snap');
    pill.classList.toggle('pip-leaving', motion === 'leave');
    pill.style.translate = x + 'px ' + y + 'px';
    position = { x: x, y: y };
  }

  function applyPlacement(motion) {
    document.body.dataset.pipCorner = placement.mode === 'corner' ? placement.corner : 'stashed';
    if (placement.mode === 'corner') {
      tab.hidden = true;
      pill.hidden = false;
      const p = cornerPosition(placement.corner, pillSize(), viewport(), inset());
      setPosition(p.x, p.y, motion);
    } else {
      pill.hidden = true;
      showTab(false);
    }
  }

  function snapTo(corner) {
    placement = { mode: 'corner', corner: corner };
    savePlacement();
    applyPlacement('snap');
  }

  function stash(edge, centerY) {
    closePopups();
    placement = { mode: 'stashed', edge: edge, yFraction: centerY / innerHeight };
    savePlacement();
    document.body.dataset.pipCorner = 'stashed';
    setPosition(edge === 'left' ? -pillSize().width - 24 : innerWidth + 24, position.y, 'leave');
    later(function () {
      if (placement.mode !== 'stashed') return;
      pill.hidden = true;
      showTab(true);
    }, LEAVE_MS);
  }

  function tabCenterY() {
    const half = tab.offsetHeight / 2 || 30;
    return Math.min(innerHeight - half - 8, Math.max(half + 8, placement.yFraction * innerHeight));
  }

  function showTab(animate) {
    tab.dataset.edge = placement.edge;
    tab.style.top = (tabCenterY() - (tab.offsetHeight || 60) / 2) + 'px';
    tab.hidden = false;
    if (!animate) return;
    tab.classList.add('pip-tab-entering');
    nextFrame(function () {
      nextFrame(function () { tab.classList.remove('pip-tab-entering'); });
    });
  }

  function restoreFromTab() {
    const edge = placement.edge;
    const y = tabCenterY();
    const corner = cornerForTab(edge, y, innerHeight);
    tab.hidden = true;
    pill.hidden = false;
    const size = pillSize();
    // Start just off-screen beside the tab, then spring into the corner.
    setPosition(edge === 'left' ? -size.width : innerWidth, y - size.height / 2, null);
    nextFrame(function () {
      snapTo(corner);
      grip.focus({ preventScroll: true });
    });
    announce('Controls restored');
  }

  // ---------------------------------------------------------------- dragging
  let drag = null;

  function beginDrag(pointerX, pointerY) {
    closePopups();
    // Freeze an in-flight snap where it is drawn, before measuring the grab.
    const [x, y] = getComputedStyle(pill).translate.split(' ').map(parseFloat);
    setPosition(x, y, null);
    drag = {
      offsetX: pointerX - position.x, offsetY: pointerY - position.y,
      startX: pointerX, startY: pointerY, pointerX: pointerX, moved: false,
      samples: [{ t: performance.now(), x: pointerX, y: pointerY }]
    };
    // Lift about the grab point, so the pill stays under the pointer as it scales.
    pill.style.transformOrigin = drag.offsetX + 'px ' + drag.offsetY + 'px';
    pill.classList.add('pip-lifted');
    document.body.classList.add('pip-dragging');
  }

  function moveDrag(pointerX, pointerY) {
    const now = performance.now();
    drag.samples.push({ t: now, x: pointerX, y: pointerY });
    while (drag.samples.length > 2 && now - drag.samples[0].t > VELOCITY_WINDOW_MS) drag.samples.shift();
    if (Math.hypot(pointerX - drag.startX, pointerY - drag.startY) > DRAG_SLOP_PX) drag.moved = true;
    drag.pointerX = pointerX;
    setPosition(pointerX - drag.offsetX, pointerY - drag.offsetY, null);
    // The only cue while dragging: over a stash zone the pill fades.
    pill.classList.toggle('pip-will-stash', drag.moved && !!stashEdgeAt(pointerX, innerWidth));
  }

  function endDrag() {
    const edge = stashEdgeAt(drag.pointerX, innerWidth);
    const corner = releaseCorner(position, pillSize(), pointerVelocity(drag.samples, performance.now()), viewport());
    const moved = drag.moved;
    drag = null;
    pill.classList.remove('pip-lifted', 'pip-will-stash');
    document.body.classList.remove('pip-dragging');
    if (!moved) { applyPlacement('snap'); return; }
    if (edge) stash(edge, position.y + pillSize().height / 2);
    else snapTo(corner);
  }

  listen(grip, 'pointerdown', function (e) {
    if (e.button !== 0) return;
    e.preventDefault();
    grip.setPointerCapture(e.pointerId);
    beginDrag(e.clientX, e.clientY);
  });

  // A press on the tab restores the controls; pulling it out drags them out.
  let tabPress = null;
  listen(tab, 'pointerdown', function (e) {
    if (e.button !== 0) return;
    tab.setPointerCapture(e.pointerId);
    tabPress = { x: e.clientX, y: e.clientY, pulled: false };
  });

  listen(window, 'pointermove', function (e) {
    if (drag) { moveDrag(e.clientX, e.clientY); return; }
    if (tabPress && !tabPress.pulled && Math.hypot(e.clientX - tabPress.x, e.clientY - tabPress.y) > TAB_PULL_PX) {
      tabPress.pulled = true;
      tab.hidden = true;
      pill.hidden = false;
      const gripCenterX = grip.offsetLeft + grip.offsetWidth / 2;
      setPosition(e.clientX - gripCenterX, e.clientY - pillSize().height / 2, null);
      beginDrag(e.clientX, e.clientY);
      drag.moved = true;
    }
  });
  function finishPointer() {
    if (drag) endDrag();
    if (tabPress) {
      const pulled = tabPress.pulled;
      tabPress = null;
      if (!pulled) restoreFromTab();
    }
  }
  listen(window, 'pointerup', finishPointer);
  listen(window, 'pointercancel', finishPointer);
  // Enter or Space on the tab arrives as a click with no pointer press behind it.
  listen(tab, 'click', function (e) { if (e.detail === 0) restoreFromTab(); });

  listen(grip, 'keydown', function (e) {
    if (placement.mode !== 'corner') return;
    const move = arrowMove(placement.corner, e.key);
    if (!move) return;
    e.preventDefault();
    if (move.corner) { snapTo(move.corner); return; }
    stash(move.edge, position.y + pillSize().height / 2);
    later(function () { tab.focus(); }, LEAVE_MS + 10);
    announce('Controls hidden. Press Enter on the tab to bring them back.');
  });

  // ---------------------------------------------------------------- popups
  // The info card (from the title) and the ⋮ menu. One open at a time.
  function openPopup(popup) {
    closePopups();
    const panel = popup.panel;
    panel.hidden = false;
    popup.trigger.setAttribute('aria-expanded', 'true');
    const pillRect = pill.getBoundingClientRect();
    const place = popupPlacement({
      corner: placement.corner,
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
      if (document.activeElement === document.querySelector('iframe')) closePopups();
    });
  });
  listen(window, 'keydown', function (e) {
    const trigger = openPopupTrigger();
    if (e.key === 'Escape' && trigger) { closePopups(); trigger.focus(); }
  });

  listen(root.querySelector('#pip-hide'), 'click', function () {
    stash(isLeft(placement.corner) ? 'left' : 'right', position.y + pillSize().height / 2);
    later(function () { tab.focus(); }, LEAVE_MS + 10);
    announce('Controls hidden. Press Enter on the tab to bring them back.');
  });

  listen(window, 'resize', function () {
    if (drag) return;
    closePopups();
    applyPlacement(null);
  });

  // The pill's width is not fixed: the posture mark gains a glyph when an
  // approval lands (the htmx swap), and the icon font can arrive after the first
  // layout. A pill docked on the right keeps its corner only if it re-docks when
  // it changes size.
  let dockedWidth = 0;
  const observer = new ResizeObserver(function () {
    const width = pill.offsetWidth;
    if (width === dockedWidth || drag || pill.hidden || placement.mode !== 'corner') return;
    dockedWidth = width;
    applyPlacement(null);
  });
  observer.observe(pill);

  applyPlacement(null);

  return () => {
    connected = false;
    for (const remove of listeners) remove();
    for (const id of timers) clearTimeout(id);
    for (const id of frames) window.cancelAnimationFrame?.(id);
    observer.disconnect();
    closePopups();
    document.body.classList.remove('pip-dragging');
    pill.classList.remove('pip-lifted', 'pip-will-stash', 'pip-snapping', 'pip-leaving');
    tab.classList.remove('pip-tab-entering');
  };
}
