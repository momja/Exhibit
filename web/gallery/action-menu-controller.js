import { Controller } from '../stimulus/stimulus.js';

// Markup comes from the actionMenu Go partial. Each instance owns only its
// open/focus state; feature actions are bubbling events, not API calls here.
export default class extends Controller {
  static targets = ['trigger', 'menu', 'item'];

  connect() {
    this.close();
  }

  disconnect() {
    this.close();
  }

  get items() {
    return this.itemTargets.filter(item => !item.disabled && !item.hidden);
  }

  toggle() {
    if (this.menuTarget.hidden) this.open();
    else this.close(true);
  }

  open(last = false) {
    this.dispatch('opened');
    this.menuTarget.hidden = false;
    this.triggerTarget.setAttribute('aria-expanded', 'true');
    this.element.classList.add('is-open');
    (last ? this.items.at(-1) : this.items[0])?.focus();
  }

  close(restoreFocus = false) {
    if (this.hasMenuTarget) this.menuTarget.hidden = true;
    if (this.hasTriggerTarget) this.triggerTarget.setAttribute('aria-expanded', 'false');
    this.element.classList.remove('is-open');
    this.search = '';
    clearTimeout(this.searchTimer);
    if (restoreFocus && this.hasTriggerTarget) this.triggerTarget.focus();
  }

  closeOther(event) {
    if (event.target !== this.element) this.close();
  }

  dismiss(event) {
    if (!this.element.contains(event.target)) this.close();
  }

  focusOut(event) {
    // Safari may report no destination while tapping a menu item. Let that
    // click finish; a known destination outside the component dismisses it.
    if (event.relatedTarget && !this.element.contains(event.relatedTarget)) this.close();
  }

  select(event) {
    if (event.currentTarget.getAttribute('aria-disabled') === 'true') {
      event.preventDefault();
      event.stopImmediatePropagation();
      return;
    }
    this.close(true);
    if (event.params.event) {
      this.element.dispatchEvent(new CustomEvent(event.params.event, {
        bubbles: true,
        detail: { ...event.params.payload, trigger: this.triggerTarget }
      }));
    }
  }

  navigate(event) {
    const { key } = event;
    if (event.target === this.triggerTarget) {
      if (key === 'ArrowDown' || key === 'ArrowUp') {
        event.preventDefault();
        this.open(key === 'ArrowUp');
      }
      return;
    }
    if (this.menuTarget.hidden) return;
    if (key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      this.close(true);
    } else if (key === 'Tab') {
      // Continue the browser's tab sequence from the visible trigger.
      this.close(true);
    } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(key)) {
      event.preventDefault();
      const items = this.items;
      const current = items.indexOf(document.activeElement);
      const next = key === 'Home' ? 0 : key === 'End' ? items.length - 1
        : (current + (key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
      items[next]?.focus();
    } else if (key === ' ' && event.target.closest('a')) {
      event.preventDefault();
      event.target.closest('a').click();
    } else if (key.length === 1 && key !== ' ' && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      clearTimeout(this.searchTimer);
      this.search += key.toLocaleLowerCase();
      const items = this.items;
      const current = items.indexOf(document.activeElement);
      [...items.slice(current + 1), ...items.slice(0, current + 1)]
        .find(item => item.textContent.trim().toLocaleLowerCase().startsWith(this.search))?.focus();
      this.searchTimer = setTimeout(() => { this.search = ''; }, 500);
    }
  }
}
