/* The artifact bar's popup placement (Exh-i8q4).
 *
 * pip-placement.js is plain geometry with no DOM, so these call it directly.
 * It loads the built, embedded copy the browser is served, like the other
 * page-script tests.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const placement = await import(pathToFileURL(path.join(here, '../../internal/api/assets/gallery/pip-placement.js')).href);
const { INFO_MIN_WIDTH_PX, POPUP_GAP_PX, popupPlacement } = placement;

const VIEW = { width: 1280, height: 800 };
const rect = (x, y, w, h) => ({ left: x, top: y, right: x + w, bottom: y + h, width: w, height: h });

test('the info card opens upward from the bar, aligned to its left edge and as wide as it', () => {
  const bar = rect(475, 742, 330, 46);
  const place = popupPlacement({ side: 'left', pill: bar, anchor: bar, align: 'pill', panelWidth: 0, viewport: VIEW, inset: 12 });
  assert.equal(place.width, 330);
  assert.equal(place.left, 475);
  assert.equal(place.right, '');
  assert.equal(place.top, '');
  assert.equal(VIEW.height - place.bottom, bar.top - POPUP_GAP_PX);
  assert.equal(place.maxHeight, bar.top - POPUP_GAP_PX - 12);
  assert.equal(place.origin, 'bottom left');
});

test('the ⋮ menu hangs off its own button and stays inside the window', () => {
  const bar = rect(475, 742, 330, 46);
  const more = rect(765, 747, 36, 36);
  const place = popupPlacement({ side: 'right', pill: bar, anchor: more, align: 'trigger', panelWidth: 236, viewport: VIEW, inset: 12 });
  assert.equal(VIEW.width - place.right, more.right);
  assert.equal(place.left, '');
  assert.equal(place.width, null, 'the menu keeps its own width');
  assert.equal(place.origin, 'bottom right');

  const phone = { width: 390, height: 844 };
  const clamped = popupPlacement({ side: 'right', pill: rect(12, 778, 366, 54), anchor: rect(380, 783, 44, 44), align: 'trigger', panelWidth: 236, viewport: phone, inset: 12 });
  assert.equal(clamped.right, POPUP_GAP_PX, 'a trigger past the edge is clamped inside the window');
});

test('a short bar still gets a readable card, and a phone never a wider one than fits', () => {
  const short = rect(540, 742, 200, 46);
  assert.equal(popupPlacement({ side: 'left', pill: short, anchor: short, align: 'pill', panelWidth: 0, viewport: VIEW, inset: 12 }).width, INFO_MIN_WIDTH_PX);
  const tiny = { width: 260, height: 600 };
  const place = popupPlacement({ side: 'left', pill: short, anchor: short, align: 'pill', panelWidth: 0, viewport: tiny, inset: 12 });
  assert.equal(place.width, 260 - 2 * POPUP_GAP_PX);
  assert.equal(place.left, POPUP_GAP_PX, 'and it is pulled back inside the window');
});
