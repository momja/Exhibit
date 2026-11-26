/* Where the artifact bar's popups go (Exh-i8q4), as plain geometry.
 *
 * No DOM here: the function takes rectangles and numbers and returns a
 * decision, so the rule can be tested by calling it. pip.js measures the
 * page, asks, and applies the answer.
 *
 * The bar is pinned to the bottom centre of the window (docs/artifact-bar.md),
 * so every popup opens upward from it.
 */

// Both popups open this far off the bar.
export const POPUP_GAP_PX = 8;

// The info card matches the bar's width so the two stack as one column, and
// never drops below this, so a short title still gets a readable card.
export const INFO_MIN_WIDTH_PX = 280;

// Where a popup goes: above the bar, POPUP_GAP_PX off it, kept POPUP_GAP_PX
// inside the window. `side` is the edge it is aligned to: 'left' lines its
// left edge up with `anchor`'s left edge (the info card, anchored to the bar),
// 'right' its right edge with `anchor`'s right edge (the ⋮ menu, anchored to
// its own button).
//
// align "pill" makes the popup the bar's width as well (the info card);
// align "trigger" keeps the panel's own width (the ⋮ menu).
//
// Returns CSS values in px; a side that does not apply is ''.
export function popupPlacement({ side, pill, anchor, align, panelWidth, viewport, inset }) {
  const edge = POPUP_GAP_PX;
  const width = align === 'pill'
    ? Math.min(Math.max(pill.width, INFO_MIN_WIDTH_PX), viewport.width - 2 * edge)
    : panelWidth;
  const left = side === 'left';
  const maxOffset = viewport.width - width - edge;
  return {
    width: align === 'pill' ? width : null,
    maxHeight: Math.max(0, pill.top - POPUP_GAP_PX - inset),
    top: '',
    bottom: viewport.height - pill.top + POPUP_GAP_PX,
    left: left ? Math.max(edge, Math.min(anchor.left, maxOffset)) : '',
    right: left ? '' : Math.max(edge, Math.min(viewport.width - anchor.right, maxOffset)),
    origin: 'bottom ' + side
  };
}
