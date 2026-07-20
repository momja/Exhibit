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
