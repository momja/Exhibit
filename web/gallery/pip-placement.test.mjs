/* The detail page's floating controls (Exh-ghuv): the placement rules.
 *
 * pip-placement.js is plain geometry with no DOM, so these call it directly.
 * Each test is one rule the design settled on in review, phrased as the
 * situation it decides. It loads the built, embedded copy the browser is
 * served, like the other page-script tests.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const placement = await import(pathToFileURL(path.join(here, '../../internal/api/assets/gallery/pip-placement.js')).href);
const {
  STASH_ZONE_PX, INFO_MIN_WIDTH_PX, POPUP_GAP_PX, arrowMove, cornerForTab, cornerPosition,
  pointerVelocity, popupPlacement, releaseCorner, stashEdgeAt
} = placement;

const VIEW = { width: 1280, height: 800 };
const PILL = { width: 330, height: 40 };
const rect = (x, y, w, h) => ({ left: x, top: y, right: x + w, bottom: y + h, width: w, height: h });

test('a docked pill sits the inset in from both edges of its corner', () => {
  assert.deepEqual(cornerPosition('tl', PILL, VIEW, 12), { x: 12, y: 12 });
  assert.deepEqual(cornerPosition('br', PILL, VIEW, 12), { x: 1280 - 12 - 330, y: 800 - 12 - 40 });
});

test('only a release inside a side edge zone stashes, wherever the throw was headed', () => {
  assert.equal(stashEdgeAt(0, 1280), 'left');
  assert.equal(stashEdgeAt(STASH_ZONE_PX, 1280), 'left');
  assert.equal(stashEdgeAt(STASH_ZONE_PX + 1, 1280), null);
  assert.equal(stashEdgeAt(1280 - STASH_ZONE_PX, 1280), 'right');
  assert.equal(stashEdgeAt(1100, 1280), null, 'a fast flick released inside the window docks instead');
});

test('the grip of a pill docked on the left starts outside the stash zone', () => {
  // Picking the pill up and dropping it straight back must not put it away.
  const backButtonAndDivider = 4 + 30 + 2 + 7;
  const gripCentre = 12 + 1 + backButtonAndDivider + 11;
  assert.equal(stashEdgeAt(gripCentre, 1280), null);
});

test('a slow drop docks in the quadrant it was dropped in', () => {
  const still = { x: 0, y: 0 };
  assert.equal(releaseCorner({ x: 900, y: 40 }, PILL, still, VIEW), 'tr');
  assert.equal(releaseCorner({ x: 100, y: 600 }, PILL, still, VIEW), 'bl');
});

test('a flick docks where the throw carries the centre', () => {
  // Released top-right of centre but thrown hard down: bottom-right.
  assert.equal(releaseCorner({ x: 900, y: 300 }, PILL, { x: 0, y: 2 }, VIEW), 'br');
});

test('velocity is zero once the pointer rested before release', () => {
  const samples = [{ t: 0, x: 0, y: 0 }, { t: 40, x: 80, y: 0 }];
  assert.deepEqual(pointerVelocity(samples, 45), { x: 2, y: 0 });
  assert.deepEqual(pointerVelocity(samples, 200), { x: 0, y: 0 });
  assert.deepEqual(pointerVelocity(samples.slice(0, 1), 10), { x: 0, y: 0 });
});

test('arrow keys move between corners and past a side edge put the controls away', () => {
  assert.deepEqual(arrowMove('bl', 'ArrowUp'), { corner: 'tl' });
  assert.deepEqual(arrowMove('tl', 'ArrowRight'), { corner: 'tr' });
  assert.deepEqual(arrowMove('tr', 'ArrowRight'), { edge: 'right' });
  assert.deepEqual(arrowMove('bl', 'ArrowLeft'), { edge: 'left' });
  assert.equal(arrowMove('bl', 'Enter'), null);
});

test('a tab brings the controls back to the nearer corner on its side', () => {
  assert.equal(cornerForTab('left', 100, 800), 'tl');
  assert.equal(cornerForTab('right', 700, 800), 'br');
});

test('the info card is flush with the pill and as wide as it, in every corner', () => {
  for (const corner of ['tl', 'tr', 'bl', 'br']) {
    const p = cornerPosition(corner, PILL, VIEW, 12);
    const pill = rect(p.x, p.y, PILL.width, PILL.height);
    const place = popupPlacement({ corner, pill, anchor: pill, align: 'pill', panelWidth: 0, viewport: VIEW, inset: 12 });
    assert.equal(place.width, PILL.width, corner);
    if (corner[1] === 'l') assert.equal(place.left, pill.left, corner);
    else assert.equal(VIEW.width - place.right, pill.right, corner);
    if (corner[0] === 't') assert.equal(place.top, pill.bottom + POPUP_GAP_PX, corner);
    else assert.equal(VIEW.height - place.bottom, pill.top - POPUP_GAP_PX, corner);
  }
});

test('the ⋮ menu hangs off its own button and stays inside the window', () => {
  const pill = rect(12, 748, 330, 40);
  const more = rect(305, 753, 30, 30);
  const place = popupPlacement({ corner: 'bl', pill, anchor: more, align: 'trigger', panelWidth: 236, viewport: VIEW, inset: 12 });
  assert.equal(place.left, 305);
  assert.equal(place.width, null, 'the menu keeps its own width');

  const phone = { width: 390, height: 844 };
  const phoneMore = rect(320, 797, 30, 30);
  const clamped = popupPlacement({ corner: 'bl', pill: rect(12, 792, 293, 40), anchor: phoneMore, align: 'trigger', panelWidth: 236, viewport: phone, inset: 12 });
  assert.equal(clamped.left, 390 - 236 - POPUP_GAP_PX);
});

test('a short pill still gets a readable card, and a phone never a wider one than fits', () => {
  const short = rect(12, 12, 200, 40);
  assert.equal(popupPlacement({ corner: 'tl', pill: short, anchor: short, align: 'pill', panelWidth: 0, viewport: VIEW, inset: 12 }).width, INFO_MIN_WIDTH_PX);
  const tiny = { width: 260, height: 600 };
  assert.equal(popupPlacement({ corner: 'tl', pill: short, anchor: short, align: 'pill', panelWidth: 0, viewport: tiny, inset: 12 }).width, 260 - 2 * POPUP_GAP_PX);
});

test('a popup never runs past the inset of a short window', () => {
  const pill = rect(12, 12, 330, 40);
  const place = popupPlacement({ corner: 'tl', pill, anchor: pill, align: 'pill', panelWidth: 0, viewport: { width: 900, height: 300 }, inset: 12 });
  assert.equal(place.top + place.maxHeight, 300 - 12);
});
