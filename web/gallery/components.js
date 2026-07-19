/* Shared gallery component behavior — served from the app origin at
 * /assets/gallery/components.js, loaded on every page that uses the
 * capability-cluster component (index.tmpl, detail.tmpl) alongside
 * components.css. Currently just the capability posture popover (av-41se).
 *
 * Opening on hover and on keyboard focus (including tabbing forward from the
 * trigger into the popover's own "Manage" link) is pure CSS
 * (.capability-wrap:hover/:focus-within in components.css) — no JS needed for
 * either. This script adds only what CSS can't express on its own:
 *   - a tap/click toggle, since some touch browsers don't reliably focus a
 *     plain element on tap the way :focus-within would need;
 *   - Escape to dismiss, regardless of how the popover was opened;
 *   - aria-expanded bookkeeping on the trigger.
 */
(function() {
  function closeAll(except) {
    document.querySelectorAll('.capability-wrap.is-open').forEach(function(wrap) {
      if (wrap === except) return;
      wrap.classList.remove('is-open');
      var trigger = wrap.querySelector('[data-capability-trigger]');
      if (trigger) trigger.setAttribute('aria-expanded', 'false');
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
    wrap.classList.toggle('is-open', open);
    trigger.setAttribute('aria-expanded', String(open));
  });

  // Escape closes whichever popover currently holds focus or is click-opened,
  // and blurs its trigger so a CSS :focus-within-driven open also closes.
  document.addEventListener('keydown', function(e) {
    if (e.key !== 'Escape') return;
    var active = document.activeElement;
    var wrap = active && active.closest ? active.closest('.capability-wrap') : null;
    if (!wrap) wrap = document.querySelector('.capability-wrap.is-open');
    if (!wrap) return;
    wrap.classList.remove('is-open');
    var trigger = wrap.querySelector('[data-capability-trigger]');
    if (trigger) trigger.setAttribute('aria-expanded', 'false');
    if (active && wrap.contains(active) && active.blur) active.blur();
  });

  // Keep aria-expanded in sync with keyboard focus moving into and out of the
  // trigger/popover pair, independent of the click-toggle state above.
  document.addEventListener('focusin', function(e) {
    var trigger = e.target.closest && e.target.closest('[data-capability-trigger]');
    if (trigger) trigger.setAttribute('aria-expanded', 'true');
  });
  document.addEventListener('focusout', function(e) {
    var trigger = e.target.closest && e.target.closest('[data-capability-trigger]');
    if (!trigger) return;
    var wrap = trigger.closest('.capability-wrap');
    if (wrap && (!e.relatedTarget || !wrap.contains(e.relatedTarget))) {
      trigger.setAttribute('aria-expanded', 'false');
    }
  });
})();
