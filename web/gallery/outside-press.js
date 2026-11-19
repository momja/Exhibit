// A press outside an open popup menu dismisses it and does nothing else, as
// native menus do on phones and desktops alike. Without this the same press
// also activates whatever it landed on: on a phone the space around a card's
// menu is mostly other cards, so closing the menu opened an artifact (Exh-bb37).
//
// The decision is made at pointerdown, while the popup is still open. By the
// time the click arrives, the popup has closed (Clerk closes its own popover
// on pointerdown; focus leaving an action menu closes it before the click),
// so a click-time check would see nothing open and let the click through.
// The pointerdown itself is left alone: focus and scrolling behave normally,
// and only the click it would have produced is consumed.

const popups = new Set();
let consumeNextClick = false;

// popup: { isOpen(), contains(target), dismiss() }. Returns the release
// function; call it when the popup's markup goes away.
export function consumeOutsidePress(popup) {
  popups.add(popup);
  return () => popups.delete(popup);
}

// Window capture runs before any document or element listener, including
// floating-ui's dismissal inside Clerk, so every popup is still open here.
window.addEventListener('pointerdown', event => {
  consumeNextClick = false;
  for (const popup of popups) {
    if (!popup.isOpen() || popup.contains(event.target)) continue;
    popup.dismiss();
    consumeNextClick = true;
  }
}, true);

// A touch that turns into a scroll produces no click. Forget the press so a
// later, unrelated click is not the one consumed.
window.addEventListener('pointercancel', () => { consumeNextClick = false; }, true);

window.addEventListener('click', event => {
  if (!consumeNextClick) return;
  consumeNextClick = false;
  event.preventDefault();
  event.stopImmediatePropagation();
}, true);
