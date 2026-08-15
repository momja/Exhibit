/* Home-screen (standalone) zoom behaviour (av-8zqr), loaded by the shared
 * pwaHead partial on every app-origin page.
 *
 * In a browser tab nothing here runs: pinch-to-zoom is a page-level
 * accessibility affordance and a tab is a page. Launched from the home screen
 * the same markup is an app shell, where a pinch is nearly always a stray
 * second finger on a scrolling grid rather than a request to zoom — and
 * unlike a tab there is no visible browser chrome to make the resulting zoom
 * state obvious or easy to undo. So everything below is scoped to that
 * display mode alone.
 *
 * Two halves, and the second is what makes the first shippable:
 *
 *   1. The pinch is disabled — the viewport meta pins minimum/maximum scale
 *      together (what Chrome honours in an installed PWA) and WebKit's
 *      non-standard gesturestart/gesturechange/gestureend are cancelled,
 *      which is the only thing that actually stops the zoom on iOS. The
 *      earlier attempt (av-s9ti) had just the meta, which is why it never
 *      worked there.
 *   2. Text stays resizable to 200% through the textScale control the header
 *      already carries, revealed here. Removing the pinch without replacing
 *      it would fail WCAG 1.4.4 (Resize Text) — the objection that got
 *      av-s9ti reverted — so the control is not a nicety attached to this
 *      guard, it is the half that earns it.
 *
 * Both halves drive the *same* mechanism: the viewport meta's scale. It is
 * the browser's own page zoom — precisely what the pinch would have done —
 * so text gets physically bigger and the reader pans. It does NOT reflow:
 * `width=device-width` keeps the layout viewport at 390px on a 390px phone
 * whatever the scale, and only the visual viewport narrows (measured at
 * scale 2: documentElement.clientWidth 390, visualViewport.width 195).
 * Reflowing instead would mean pinning a numeric `width=<device-width/scale>`,
 * which trades the browser's device handling for arithmetic on `screen.width`
 * that has to survive rotation — not obviously worth it, and not what the
 * gesture being replaced did either.
 *
 * Two things were measured before settling on it, both against a 390px mobile
 * viewport in a standalone window:
 *
 *   - CSS `zoom: 2` on the root magnifies *and* stretches the document to
 *     791px inside a 390px screen, so the page scrolls sideways at every
 *     scale. The meta scale leaves layout untouched and lets the browser do
 *     the magnifying, so nothing overflows that did not overflow at 100%.
 *   - The scale is only honoured when it is in the meta *at parse time*.
 *     Rewriting the tag afterwards updates the attribute and changes nothing
 *     on screen. So a scale change is stored and applied by reloading, rather
 *     than pretended at live.
 *
 * A pinch that lands inside a rendered artifact's iframe is not covered, and
 * deliberately so: that frame is another origin whose events never reach this
 * document, and an artifact's viewport is the artifact's to set (architecture
 * §1).
 */
(function() {
  /* 100% to 200% — the top step is WCAG 1.4.4's requirement, and the ones
   * between it and 100% are what make the control usable rather than a
   * toggle. */
  var SCALES = [1, 1.25, 1.5, 1.75, 2];
  var STORAGE_KEY = 'exhibit.text-scale';

  /* matchMedia must be called on window — a detached reference throws. */
  function displayMode(mode) {
    return typeof window.matchMedia === 'function' &&
      window.matchMedia('(display-mode: ' + mode + ')').matches;
  }

  var standalone =
    window.navigator.standalone === true ||   // iOS home-screen launch
    displayMode('standalone') ||              // installed PWA (Android/desktop)
    displayMode('fullscreen');
  if (!standalone) return;

  /* This is app-origin localStorage — the real one. The storage shim that
   * backs artifact state (docs/architecture.md §6) lives in the render
   * origin's sandbox and never sees this key. Private-mode browsers throw on
   * access rather than returning null, so both directions are guarded. */
  function readStoredIndex() {
    try {
      var i = SCALES.indexOf(parseFloat(window.localStorage.getItem(STORAGE_KEY)));
      return i === -1 ? 0 : i;
    } catch (e) {
      return 0;
    }
  }

  function store(scale) {
    try {
      window.localStorage.setItem(STORAGE_KEY, String(scale));
    } catch (e) {
      /* Preference is lost at reload; the scale still applies for this run. */
    }
  }

  var index = readStoredIndex();

  /* Pinning minimum = maximum = the chosen scale is what disables the pinch:
   * there is no range left to zoom through, at any scale the control picks. */
  function applyScale(scale) {
    var viewport = document.querySelector('meta[name="viewport"]');
    if (!viewport) return;
    viewport.setAttribute('content',
      'width=device-width,initial-scale=' + scale +
      ',minimum-scale=' + scale + ',maximum-scale=' + scale +
      ',user-scalable=no');
  }

  /* Runs while the head is parsing, so the stored scale is in place before
   * first paint rather than after a visible reflow. */
  applyScale(SCALES[index]);

  /* preventDefault on the first gesture event is what cancels the pinch on
   * WebKit, so these are registered explicitly non-passive — a passive
   * listener's preventDefault is a no-op. */
  ['gesturestart', 'gesturechange', 'gestureend'].forEach(function(type) {
    document.addEventListener(type, function(e) {
      e.preventDefault();
    }, { passive: false });
  });

  function wireControl() {
    var control = document.querySelector('[data-text-scale]');
    if (!control) return;
    var value = control.querySelector('[data-text-scale-value]');
    var buttons = control.querySelectorAll('[data-text-scale-step]');

    function render() {
      if (value) value.textContent = Math.round(SCALES[index] * 100) + '%';
      buttons.forEach(function(button) {
        var next = index + parseInt(button.getAttribute('data-text-scale-step'), 10);
        button.disabled = next < 0 || next >= SCALES.length;
      });
    }

    /* A scale only takes effect at parse time, so changing it means reloading.
     * That is free on a page you are reading and destructive on one you are
     * typing into — the edit buffer, a pasted body, an agent conversation —
     * and no page tracks whether it is dirty. Rather than build that, the
     * pages that can hold unsaved work declare it in their markup and get a
     * confirm; the rest reload silently. Declining leaves everything exactly
     * as it was, including the label, so the control never claims a size the
     * page is not rendering at. */
    var warnOnReload = document.body.hasAttribute('data-scale-reload-warn');

    buttons.forEach(function(button) {
      button.addEventListener('click', function() {
        var next = index + parseInt(button.getAttribute('data-text-scale-step'), 10);
        if (next < 0 || next >= SCALES.length) return;
        if (warnOnReload &&
            !window.confirm('Reloading to change the text size will discard unsaved changes on this page. Continue?')) {
          return;
        }
        index = next;
        store(SCALES[index]);
        window.location.reload();
      });
    });

    render();
    control.hidden = false;   // hidden in the markup: tabs keep browser zoom
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wireControl);
  } else {
    wireControl();
  }
})();
