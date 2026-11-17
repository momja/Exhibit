// Sign-in descent (Exh-vsw0): decide whether the animation can start cleanly.
//
// The server loads this module only on a visit it marked to animate; such a
// page starts on the plain sky, not the finished picture. The layer images sit
// inert inside a <template>; this file clones them in (which is what starts
// their download), waits until every one is decoded, and only then adds
// .login-descent-go to the body, which plays the CSS animation. If they are not
// ready within the deadline, or reduced motion is (or becomes) on, the run is
// skipped for good: the layers are removed and .login-descent-skip crossfades
// the static composite in. Scenery popping in mid-animation, or after it ended,
// is worse than none. The cookie that makes this a once-only event is already
// spent by then; that trade-off is deliberate.
//
// Hosted sign-in (data-descent-after="clerk") holds the downloads back until
// Clerk's scripts have loaded, so the scenery never competes with them, and gets
// a longer deadline to cover the wait.

const DEADLINE_MS = 1500;
const CLERK_DEADLINE_MS = 3000;

function clerkLoaded(signal) {
  if (window.Clerk?.loaded) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const onAbort = () => reject(new DOMException('cancelled', 'AbortError'));
    signal.addEventListener('abort', onAbort, { once: true });
    window.addEventListener('exhibit:clerk-loaded', () => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, { once: true, signal });
  });
}

async function layersDecoded(stage, waitForClerk, signal) {
  if (waitForClerk) await clerkLoaded(signal);
  stage.append(stage.querySelector('template').content.cloneNode(true));
  await Promise.all([...stage.querySelectorAll('img')].map((img) => img.decode()));
}

async function start() {
  const body = document.body;
  if (!body.classList.contains('login-descend')) return;
  const reduce = matchMedia('(prefers-reduced-motion: reduce)');
  const stage = document.querySelector('.login-layers-stage');

  const controller = new AbortController();
  let cancelled = false;
  let timer;
  // Permanent: nothing restarts a cancelled run.
  const cancel = () => {
    if (cancelled) return;
    cancelled = true;
    clearTimeout(timer);
    controller.abort();
    stage?.replaceChildren();
    body.classList.remove('login-descent-go');
    body.classList.add('login-descent-skip');
  };

  if (reduce.matches || !stage) { cancel(); return; }
  reduce.addEventListener('change', (event) => { if (event.matches) cancel(); });

  const deadline = body.dataset.descentAfter === 'clerk' ? CLERK_DEADLINE_MS : DEADLINE_MS;
  timer = setTimeout(cancel, deadline);
  try {
    await layersDecoded(stage, body.dataset.descentAfter === 'clerk', controller.signal);
  } catch {
    cancel();
    return;
  }
  if (cancelled) return;
  clearTimeout(timer);
  body.classList.add('login-descent-go');
}

start();
