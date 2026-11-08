// Whether a phone gallery's header and space tabs should be out of the way,
// given where the page has scrolled. Pure, so it is tested without a browser;
// scroll-chrome-controller.js feeds it positions and applies the answer.
//
// The bars hide after a deliberate scroll down and return after a deliberate
// scroll up, the way mobile browsers treat their own toolbars. "Deliberate"
// is a distance measured from the furthest point reached in the current
// direction, so a hand resting on the glass cannot flicker them.

// Pixels of travel against the current state before it flips.
export const SCROLL_CHROME_TOLERANCE = 12;

// state:  { hidden, anchor } from the previous call; anchor is the extreme
//         position reached since the last flip (lowest while shown, highest
//         while hidden). Start from initialScrollChrome(y).
// y:      the scroll position, already clamped to [0, maxY] so a rubber-band
//         overscroll is not read as travel in the other direction.
// maxY:   the furthest the page can scroll.
// topZone: positions this close to the top always show the bars, so the
//         header returns before the user reaches the content it sat above.
export function nextScrollChrome(state, y, maxY, topZone) {
  // At either end there is nothing further to reveal: the top is where the
  // header belongs, and the end of the list is where people switch spaces.
  if (y <= topZone || y >= maxY - 1) return { hidden: false, anchor: y };
  if (state.hidden) {
    if (y < state.anchor - SCROLL_CHROME_TOLERANCE) return { hidden: false, anchor: y };
    return { hidden: true, anchor: Math.max(state.anchor, y) };
  }
  if (y > state.anchor + SCROLL_CHROME_TOLERANCE) return { hidden: true, anchor: y };
  return { hidden: false, anchor: Math.min(state.anchor, y) };
}

export function initialScrollChrome(y) {
  return { hidden: false, anchor: y };
}
