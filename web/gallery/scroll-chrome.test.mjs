/* Phone gallery bars that hide on scroll (Exh-se4n): the direction rules.
 *
 * scroll-chrome.js has no DOM, so these call it directly, against the built,
 * embedded copy the browser is served, like the other page-script tests.
 * The browser half (attribute, CSS, focus) is TestGalleryPhoneChromeBrowser.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const { SCROLL_CHROME_TOLERANCE: TOL, initialScrollChrome, nextScrollChrome } =
  await import(pathToFileURL(path.join(here, '../../internal/api/assets/gallery/scroll-chrome.js')).href);

const MAX = 5000;
const TOP = 56;

// Feeds positions in order and returns whether the bars were hidden after each.
function scroll(...positions) {
  let state = initialScrollChrome(positions[0]);
  return positions.slice(1).map(y => (state = nextScrollChrome(state, y, MAX, TOP)).hidden);
}

test('scrolling down past the header hides the bars', () => {
  assert.deepEqual(scroll(0, 100, 200, 400), [true, true, true]);
});

test('a scroll up of more than the tolerance brings them back', () => {
  assert.deepEqual(scroll(0, 400, 1000, 1000 - TOL, 1000 - TOL - 1), [true, true, true, false]);
});

test('the distance back is measured from the furthest point reached', () => {
  // Small wobbles on the way down neither reveal nor reset where "up" starts.
  assert.deepEqual(scroll(0, 400, 395, 800, 800 - TOL - 1), [true, true, true, false]);
});

test('jitter below the tolerance does not hide them either', () => {
  assert.deepEqual(scroll(1000, 1000 + TOL, 1000 + TOL - 5, 1000 + TOL), [false, false, false]);
  assert.deepEqual(scroll(1000, 990, 990 + TOL, 990 + TOL + 1), [false, false, true]);
});

test('near the top they are always shown', () => {
  assert.deepEqual(scroll(400, 1000, TOP), [true, false]);
  assert.deepEqual(scroll(0, TOP, TOP - 1), [false, false]);
});

test('at the end of the list they are shown, for switching spaces', () => {
  assert.deepEqual(scroll(0, 1000, MAX - 1), [true, false]);
  // The first step back up from the end does not hide them again.
  assert.deepEqual(scroll(0, 1000, MAX, MAX - 200), [true, false, false]);
});

test('a page shorter than the header never hides them', () => {
  let state = initialScrollChrome(0);
  for (const y of [0, 10, 30, 40]) {
    state = nextScrollChrome(state, y, 40, TOP);
    assert.equal(state.hidden, false);
  }
});
