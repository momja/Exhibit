/* Where the detail page's floating controls go (Exh-ghuv), as plain geometry.
 *
 * No DOM here: every function takes rectangles and numbers and returns a
 * decision, so the rules can be tested by calling them. pip.js measures the
 * page, asks these, and applies the answer.
 *
 * Corners are two letters, vertical then horizontal: "tl", "tr", "bl", "br".
 * Edges are "left" and "right", the two sides the controls can be put away on.
 */

// A release with the pointer this close to a side edge puts the controls away.
// Positional only: how fast the pill was thrown never stashes it, because a
// throw that projected past the edge stashed pills people meant to keep.
export const STASH_ZONE_PX = 20;

// A throw lands where its centre would be this long after release, so a flick
// toward a corner reaches it while a slow drop lands where it was dropped.
export const PROJECTION_MS = 200;

// Velocity is read from the pointer's last moments, and counts as zero once the
// pointer has rested this long before release.
export const VELOCITY_WINDOW_MS = 80;
export const VELOCITY_REST_MS = 60;

// Both popups open this far off the pill.
export const POPUP_GAP_PX = 8;

// The info card matches the pill's width so the two stack as one column, and
// never drops below this, so a short title still gets a readable card.
export const INFO_MIN_WIDTH_PX = 280;

export function isTop(corner) { return corner[0] === 't'; }
export function isLeft(corner) { return corner[1] === 'l'; }

// The pill's top-left point when docked in a corner, `inset` in from both edges.
export function cornerPosition(corner, size, viewport, inset) {
  return {
    x: isLeft(corner) ? inset : viewport.width - inset - size.width,
    y: isTop(corner) ? inset : viewport.height - inset - size.height
  };
}

// The side a release puts the controls away on, or null to dock in a corner.
export function stashEdgeAt(pointerX, viewportWidth) {
  if (pointerX <= STASH_ZONE_PX) return 'left';
  if (pointerX >= viewportWidth - STASH_ZONE_PX) return 'right';
  return null;
}

// Pointer velocity in px/ms from samples [{t, x, y}] oldest first, or zero if
// the pointer came to rest before `now`.
export function pointerVelocity(samples, now) {
  if (samples.length < 2) return { x: 0, y: 0 };
  const first = samples[0];
  const last = samples[samples.length - 1];
  const dt = last.t - first.t;
  if (now - last.t > VELOCITY_REST_MS || dt < 8) return { x: 0, y: 0 };
  return { x: (last.x - first.x) / dt, y: (last.y - first.y) / dt };
}

// The corner a release docks in: the quadrant the throw carries the pill's
// centre into.
export function releaseCorner(position, size, velocity, viewport) {
  const cx = position.x + size.width / 2 + velocity.x * PROJECTION_MS;
  const cy = position.y + size.height / 2 + velocity.y * PROJECTION_MS;
  return (cy < viewport.height / 2 ? 't' : 'b') + (cx < viewport.width / 2 ? 'l' : 'r');
}

// What an arrow key on the grip does from a corner: move to the neighbouring
// corner, or, pushed past a side edge, put the controls away on that side.
// Returns {corner}, {edge}, or null for a key that does nothing.
export function arrowMove(corner, key) {
  const v = corner[0];
  const h = corner[1];
  if (key === 'ArrowUp') return { corner: 't' + h };
  if (key === 'ArrowDown') return { corner: 'b' + h };
  if (key === 'ArrowLeft') return h === 'l' ? { edge: 'left' } : { corner: v + 'l' };
  if (key === 'ArrowRight') return h === 'r' ? { edge: 'right' } : { corner: v + 'r' };
  return null;
}

// The corner the controls come back to from a tab: the nearer one on its side.
export function cornerForTab(edge, tabCenterY, viewportHeight) {
  return (tabCenterY < viewportHeight / 2 ? 't' : 'b') + (edge === 'left' ? 'l' : 'r');
}

// Where a popup goes. It opens toward the middle of the window, POPUP_GAP_PX
// off the pill, on the side the pill is docked: aligned to `anchor`'s outer
// edge, and kept POPUP_GAP_PX inside the window. The side comes from the
// docked corner rather than from the pill's geometry, because on a phone the
// pill spans most of the width and its centre says little about where it sits.
//
// align "pill" makes the popup the pill's width as well (the info card);
// align "trigger" hangs it off its own button (the ⋮ menu).
//
// Returns CSS values in px; a side that does not apply is ''.
export function popupPlacement({ corner, pill, anchor, align, panelWidth, viewport, inset }) {
  const edge = POPUP_GAP_PX;
  const width = align === 'pill'
    ? Math.min(Math.max(pill.width, INFO_MIN_WIDTH_PX), viewport.width - 2 * edge)
    : panelWidth;
  const top = isTop(corner);
  const left = isLeft(corner);
  const room = top ? viewport.height - pill.bottom : pill.top;
  const maxLeft = viewport.width - width - edge;
  return {
    width: align === 'pill' ? width : null,
    maxHeight: Math.max(0, room - POPUP_GAP_PX - inset),
    top: top ? pill.bottom + POPUP_GAP_PX : '',
    bottom: top ? '' : viewport.height - pill.top + POPUP_GAP_PX,
    left: left ? Math.max(edge, Math.min(anchor.left, maxLeft)) : '',
    right: left ? '' : Math.max(edge, Math.min(viewport.width - anchor.right, maxLeft)),
    origin: (top ? 'top ' : 'bottom ') + (left ? 'left' : 'right')
  };
}
