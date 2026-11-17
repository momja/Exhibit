// Sign-in descent (Exh-vsw0): decide whether the animation can start cleanly.
//
// The server loads this module only on a visit it marked to animate. The layer
// images sit inert inside a <template>; this file clones them in (which is what
// starts their download), waits until every one is decoded, and only then adds
// .login-descent-go to the body, which is what plays the CSS animation. If they
// are not ready within the deadline, the page simply stays on the static
// composite it painted first: late scenery popping in mid-animation, or after it
// ended, is worse than no animation. The cookie that makes this a once-only
// event is already spent by then; that trade-off is deliberate.
//
// Hosted sign-in (data-descent-after="clerk") holds the downloads back until
// Clerk's scripts have loaded, so the scenery never competes with them, and gets
// a longer deadline to cover the wait.

const DEADLINE_MS = 1500;
const CLERK_DEADLINE_MS = 3000;

function clerkLoaded() {
  if (window.Clerk?.loaded) return Promise.resolve();
  return new Promise((resolve) => window.addEventListener('exhibit:clerk-loaded', resolve, { once: true }));
}

async function layersDecoded(stage, waitForClerk, abandoned) {
  if (waitForClerk) await clerkLoaded();
  if (abandoned()) return;
  stage.append(stage.querySelector('template').content.cloneNode(true));
  await Promise.all([...stage.querySelectorAll('img')].map((img) => img.decode()));
}

async function start() {
  const body = document.body;
  if (!body.classList.contains('login-descend')) return;
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  const stage = document.querySelector('.login-layers-stage');
  if (!stage) return;

  const waitForClerk = body.dataset.descentAfter === 'clerk';
  const deadline = waitForClerk ? CLERK_DEADLINE_MS : DEADLINE_MS;
  let timer;
  const late = new Promise((resolve) => { timer = setTimeout(resolve, deadline, false); });
  let gaveUp = false;
  const ready = layersDecoded(stage, waitForClerk, () => gaveUp).then(() => !gaveUp, () => false);

  const inTime = await Promise.race([ready, late]);
  clearTimeout(timer);
  if (inTime) {
    body.classList.add('login-descent-go');
  } else {
    // Stay on the composite; do not let the layers arrive later and show.
    gaveUp = true;
    stage.replaceChildren();
  }
}

start();
