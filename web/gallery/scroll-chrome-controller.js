import { Controller } from '../stimulus/stimulus.js';
import { eventScope } from './event-scope.js';
import { initialScrollChrome, nextScrollChrome } from './scroll-chrome.js';

// The bars this hides. The header lives for the whole document; the space
// tabs are replaced by every space switch, so both are looked up on use.
const BARS = '[data-app-header], .gallery-rail';

// Phone galleries give the screen back to the cards while the user scrolls
// down, and return the header and space tabs on the way back up
// (scroll-chrome.js decides which). This only sets data-chrome-hidden on its
// element: index.css decides what that means, and only at phone widths, so a
// desktop window ignores it. The attribute sits on #gallery-shell rather than
// on the bars so a space switch's freshly swapped tabs inherit it.
export default class extends Controller {
  connect() {
    this.state = initialScrollChrome(this.position().y);
    this.scrollY = window.scrollY;
    this.events = eventScope();
    this.events.on(window, 'scroll', () => {
      this.scrollY = window.scrollY;
      this.schedule();
    }, { passive: true });
    // Keyboard focus and the / shortcut must never land on a bar the user
    // cannot see.
    this.events.on(this.element, 'focusin', event => {
      if (event.target.closest(BARS)) this.reveal();
    });
  }

  disconnect() {
    this.events.disconnect();
    cancelAnimationFrame(this.frame);
    this.frame = 0;
    this.element.removeAttribute('data-chrome-hidden');
  }

  // Scroll events arrive faster than frames; one decision per frame is enough.
  schedule() {
    if (this.frame) return;
    this.frame = requestAnimationFrame(() => {
      this.frame = 0;
      this.update();
    });
  }

  update() {
    const { y, maxY } = this.position();
    const header = this.element.querySelector('[data-app-header]');
    this.state = nextScrollChrome(this.state, y, maxY, header?.offsetHeight ?? 0);
    // Hiding a search field mid-word, or a control a keyboard user is on,
    // would take it away while it is in use. A tapped button is not
    // :focus-visible, so it does not pin the bars after the tap.
    if (this.state.hidden && this.focusedBar()) this.state = initialScrollChrome(y);
    this.element.toggleAttribute('data-chrome-hidden', this.state.hidden);
  }

  // Focus has already scrolled the page by the time focusin fires: the browser
  // brings the off-screen field into view, and for the sticky header that
  // means the top of the page. Put the reader back where they were, in the
  // same task so the jump never paints; once shown, the bar needs no scroll.
  // scrollY is the last position a scroll event reported, which focus's own
  // scroll has not yet dispatched.
  reveal() {
    if (!this.element.hasAttribute('data-chrome-hidden')) return;
    if (window.scrollY !== this.scrollY) window.scrollTo(window.scrollX, this.scrollY);
    this.state = initialScrollChrome(this.position().y);
    this.element.removeAttribute('data-chrome-hidden');
  }

  focusedBar() {
    return Array.from(this.element.querySelectorAll(BARS)).some(bar => bar.querySelector(':focus-visible'));
  }

  // Clamped, because iOS reports rubber-band overscroll as positions beyond
  // either end; springing back from them is not the user scrolling.
  position() {
    const page = document.scrollingElement || document.documentElement;
    const maxY = Math.max(0, page.scrollHeight - window.innerHeight);
    return { y: Math.min(Math.max(window.scrollY, 0), maxY), maxY };
  }
}
