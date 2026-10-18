/* Shared gallery component behavior — served from the app origin at
 * /assets/gallery/components.js, loaded on every page that uses a shared
 * component (index.tmpl, detail.tmpl, edit.tmpl, agent.tmpl) alongside
 * components.css. Three behaviors, each in its own block below: the capability
 * posture popover (av-41se), the widget tile health watcher (av-fafu), and the
 * version viewer's unsaved-changes notice (av-vw7r).
 *
 * The capability posture popover:
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
  // Ignore a null relatedTarget: Safari reports one when the Manage link is
  // tapped, and closing here would swallow the link's click.
  document.addEventListener('focusout', function(e) {
    var wrap = e.target.closest && e.target.closest('.capability-wrap');
    if (!wrap || !wrap.classList.contains('is-open')) return;
    if (e.relatedTarget && !wrap.contains(e.relatedTarget)) setOpen(wrap, false);
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

  function setFailed(frame, failed) {
    var well = frame.closest('.card-widget');
    if (well) well.classList.toggle('widget-failed', failed);
  }

  function watch(frame) {
    if (frame.dataset.widgetWatched) return;
    frame.dataset.widgetWatched = '1';

    var timer = null;
    function startDeadline() {
      timer = setTimeout(function() { setFailed(frame, true); }, DEADLINE_MS);
    }
    frame.addEventListener('__avresolved', function() { clearTimeout(timer); });

    // The clock must not start until the frame is actually allowed to load.
    // A tile inside a closed <details>, or below the fold under
    // loading="lazy", has not been fetched at all — starting the deadline at
    // page load would time it out for never answering a question it was never
    // asked, and a long gallery would show monograms for everything below the
    // fold. Intersection is the same condition loading="lazy" itself waits on,
    // so the two stay in step.
    if (typeof IntersectionObserver !== 'function') {
      startDeadline();
      return;
    }
    var io = new IntersectionObserver(function(entries) {
      for (var i = 0; i < entries.length; i++) {
        if (!entries[i].isIntersecting) continue;
        io.disconnect();
        startDeadline();
        return;
      }
    });
    io.observe(frame);
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
  document.addEventListener('htmx:after:swap', function(e) { watchAll(e.target); });

  window.addEventListener('message', function(e) {
    var d = e.data;
    if (!d || d.__avWidget !== true) return;
    // The frame's origin is opaque ('null'), so identity is the source window,
    // never e.origin — same rule as every other frame message on these pages.
    var frames = document.querySelectorAll('.card-widget-frame');
    for (var i = 0; i < frames.length; i++) {
      if (frames[i].contentWindow !== e.source) continue;
      frames[i].dispatchEvent(new CustomEvent('__avresolved'));
      // A 'ready' also CLEARS the failed state, not just suppresses the
      // deadline: a tile that was marked failed and then answers for itself
      // (a reload, an htmx re-render into the same well) has earned its frame
      // back.
      setFailed(frames[i], d.status !== 'ready');
      return;
    }
  });
})();

/* Version viewer notice (av-vw7r) — the one thing a version's frame is heard for.
 *
 * A version is shown running, with the data it left behind, and nothing done in
 * it is saved. The render shim therefore refuses every write, and a tool that
 * saves quietly looks exactly like one that has — so the first write it refuses
 * is reported here (__avVersionUnsaved, sent by internal/render's warnUnsaved)
 * and this reveals the viewer's own banner. The banner is already in the page,
 * hidden, with its sentence written (the versionViewer partial): page chrome,
 * never the artifact's DOM, which the artifact could forge.
 *
 * What makes it safe to listen to a frame that runs somebody's code is how little
 * this does with what it hears. The message carries nothing that is read, and its
 * whole effect is un-hiding a fixed sentence, so a notice the artifact forges by
 * hand is no worse than the notice appearing. It is not a bridge: nothing here
 * reaches the API, and the chat page's state, network and picker bridges still
 * listen to the live artifact frame and nothing else — a version frame stays a
 * frame the page takes no orders from.
 *
 * Identity is the source window, as for every frame message on these pages (a
 * sandboxed frame's origin is the string "null" and proves nothing). It is the
 * viewer's frame in particular, looked up when the message arrives because an
 * htmx swap replaces the viewer: a message from the live artifact frame, or from
 * a viewer that has since been closed, is not a version's to send.
 */
(function() {
  window.addEventListener('message', function(e) {
    var d = e.data;
    if (!d || d.__avVersionUnsaved !== true) return;
    var frame = document.querySelector('#version-viewer .version-viewer-frame');
    if (!frame || e.source !== frame.contentWindow) return;
    var notice = document.getElementById('version-viewer-unsaved');
    if (notice) notice.hidden = false;
  });
})();
