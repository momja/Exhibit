/* Home-screen (standalone) pinch-zoom guard (av-8zqr), loaded by the shared
 * pwaHead partial on every app-origin page.
 *
 * In a browser tab nothing here runs: pinch-to-zoom is a page-level
 * accessibility affordance and a tab is a page. Launched from the home screen
 * the same markup is an app shell, where a pinch is nearly always a stray
 * second finger on a scrolling card grid rather than a request to zoom — and
 * unlike a tab there is no visible browser chrome to make the resulting zoom
 * state obvious or easy to undo. So the guard is scoped to that display mode
 * alone.
 *
 * Two mechanisms, because neither engine is covered by one:
 *   - the viewport meta gains maximum-scale=1,user-scalable=no, which is what
 *     Chrome honours in an installed PWA on Android;
 *   - gesturestart/gesturechange/gestureend are cancelled, because WebKit
 *     treats that meta as advisory and preventing its non-standard pinch
 *     events is what actually stops the zoom in an iOS home-screen launch.
 *
 * A pinch that lands inside a rendered artifact's iframe is not covered, and
 * deliberately so: that frame is another origin whose events never reach this
 * document, and an artifact's viewport is the artifact's to set (architecture
 * §1).
 */
(function() {
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

  /* Every app-origin page declares its viewport meta ahead of pwaHead, so the
   * tag is already parsed by the time this script runs. */
  var viewport = document.querySelector('meta[name="viewport"]');
  if (viewport) {
    viewport.setAttribute('content',
      'width=device-width,initial-scale=1,maximum-scale=1,user-scalable=no');
  }

  /* preventDefault on the first gesture event is what cancels the pinch, so
   * these are registered explicitly non-passive — a passive listener's
   * preventDefault is a no-op. */
  ['gesturestart', 'gesturechange', 'gestureend'].forEach(function(type) {
    document.addEventListener(type, function(e) {
      e.preventDefault();
    }, { passive: false });
  });
})();
