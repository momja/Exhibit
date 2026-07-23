/* Not-found page behavior (av-at2v) — served from the app origin at
 * /assets/gallery/notfound.js. The whole script is the replay: the frame's
 * one knock on load is pure CSS (notfound.css), and poking it knocks it
 * again.
 *
 * Restarting a CSS animation needs the animation removed, a reflow to commit
 * that, then the declaration handed back — so the inline override is set and
 * cleared rather than a class being toggled, which keeps the keyframes and
 * their timing entirely in the stylesheet where the motion spec lives.
 *
 * A knock already in flight is left alone, so sweeping the pointer across the
 * frame doesn't stutter it back to the start; and under prefers-reduced-motion
 * nothing is wired up at all, matching the media query that parks the frame
 * crooked and still.
 */
(function() {
  var frame = document.querySelector('.exhibit-404__frame');
  if (!frame) return;
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  var knocking = true; // the load-time knock is already running
  frame.addEventListener('animationend', function() { knocking = false; });

  function replay() {
    if (knocking) return;
    knocking = true;
    frame.style.animation = 'none';
    void frame.offsetWidth; // reflow, so the restart actually takes
    frame.style.animation = '';
  }

  frame.addEventListener('click', replay);
  frame.addEventListener('mouseenter', replay);
})();
