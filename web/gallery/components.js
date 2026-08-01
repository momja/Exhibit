/* Shared gallery component behavior — served from the app origin at
 * /assets/gallery/components.js, loaded on every page that uses the
 * capability-cluster component (index.tmpl, detail.tmpl) alongside
 * components.css. Currently just the capability posture popover (av-41se).
 *
 * The popover opens only on explicit activation, never on hover or on mere
 * keyboard focus: click/tap, or Enter/Space while the trigger is focused.
 * Hovering or focusing the trigger just shows a plain affordance highlight
 * (background/color change, pure CSS — see .capability-cluster:hover /
 * :focus-visible in components.css) so it reads as clickable without
 * popping content open unasked. This script owns all of the open/close
 * state:
 *   - click/tap toggles it (also serves keyboard users' Enter/Space below,
 *     which synthesizes the same toggle);
 *   - Escape, or focus/click leaving the trigger+popover pair entirely,
 *     closes it;
 *   - aria-expanded tracks the actual open state, not just focus.
 */
(function() {
  function setOpen(wrap, open) {
    wrap.classList.toggle('is-open', open);
    var trigger = wrap.querySelector('[data-capability-trigger]');
    if (trigger) trigger.setAttribute('aria-expanded', String(open));
  }

  function closeAll(except) {
    document.querySelectorAll('.capability-wrap.is-open').forEach(function(wrap) {
      if (wrap !== except) setOpen(wrap, false);
    });
  }

  // Click/tap toggles the popover open or closed; clicking anywhere else
  // closes whatever was open (outside-click dismissal).
  document.addEventListener('click', function(e) {
    var trigger = e.target.closest('[data-capability-trigger]');
    if (!trigger) {
      closeAll();
      return;
    }
    var wrap = trigger.closest('.capability-wrap');
    var open = !wrap.classList.contains('is-open');
    closeAll(open ? wrap : null);
    setOpen(wrap, open);
  });

  // Enter/Space activates the trigger the same way a click does, so
  // keyboard-only users get the identical explicit-activation behavior
  // (rather than the popover opening automatically just from tabbing to it).
  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
      var active = document.activeElement;
      var openWrap = active && active.closest ? active.closest('.capability-wrap') : null;
      if (!openWrap) openWrap = document.querySelector('.capability-wrap.is-open');
      if (!openWrap) return;
      setOpen(openWrap, false);
      if (active && openWrap.contains(active) && active.blur) active.blur();
      return;
    }
    if (e.key !== 'Enter' && e.key !== ' ') return;
    var trigger = e.target.closest && e.target.closest('[data-capability-trigger]');
    if (!trigger) return;
    e.preventDefault();
    var wrap = trigger.closest('.capability-wrap');
    var open = !wrap.classList.contains('is-open');
    closeAll(open ? wrap : null);
    setOpen(wrap, open);
  });

  // Closing when focus leaves the trigger+popover pair entirely (e.g.
  // tabbing past the Manage link to the next control on the page) — since
  // opening no longer rides :focus-within, nothing else would close it here.
  document.addEventListener('focusout', function(e) {
    var wrap = e.target.closest && e.target.closest('.capability-wrap');
    if (!wrap || !wrap.classList.contains('is-open')) return;
    if (!e.relatedTarget || !wrap.contains(e.relatedTarget)) setOpen(wrap, false);
  });
})();

/* Widget tile health (av-fafu) — fall back to the monogram when a widget
 * doesn't come up.
 *
 * A card's widget frame is cross-origin and opaque, so from out here a 404
 * page, a widget whose script threw, and a widget that rendered perfectly all
 * fire the same `load` event and are otherwise indistinguishable. The frame
 * therefore reports on itself: the render preamble posts __avWidget with
 * status 'ready' or 'error' (internal/render, widgetHealthScript).
 *
 * Two ways to fail, one outcome. An explicit 'error' falls back immediately.
 * Hearing NOTHING within the deadline falls back too, which is the case that
 * matters most — it covers everything no in-frame script can report: a
 * document that never loaded, a parse failure, a script that hung the thread.
 *
 * Falling back is just hiding the frame: the default tile is always rendered
 * beneath it (the cardWidget partial), so the monogram is already there. No
 * markup is built here.
 */
(function() {
  // Generous: a slow first paint is not a failure, and the cost of waiting is
  // a blank tile for a moment, while the cost of being hasty is replacing a
  // widget that was about to work.
  var DEADLINE_MS = 6000;

  function fail(frame) {
    var well = frame.closest('.card-widget');
    if (well) well.classList.add('widget-failed');
  }

  function watch(frame) {
    if (frame.dataset.widgetWatched) return;
    frame.dataset.widgetWatched = '1';
    var timer = setTimeout(function() { fail(frame); }, DEADLINE_MS);
    frame.addEventListener('__avresolved', function() { clearTimeout(timer); });
  }

  function watchAll(root) {
    (root && root.querySelectorAll ? root : document)
      .querySelectorAll('.card-widget-frame').forEach(watch);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function() { watchAll(); });
  } else {
    watchAll();
  }
  // Tiles also arrive by htmx swap (the edit page's preview, the agent pane),
  // so pick those up as they land rather than only at first paint.
  document.addEventListener('htmx:afterSwap', function(e) { watchAll(e.target); });

  window.addEventListener('message', function(e) {
    var d = e.data;
    if (!d || d.__avWidget !== true) return;
    // The frame's origin is opaque ('null'), so identity is the source window,
    // never e.origin — same rule as every other frame message on these pages.
    var frames = document.querySelectorAll('.card-widget-frame');
    for (var i = 0; i < frames.length; i++) {
      if (frames[i].contentWindow !== e.source) continue;
      frames[i].dispatchEvent(new CustomEvent('__avresolved'));
      if (d.status !== 'ready') fail(frames[i]);
      return;
    }
  });
})();
