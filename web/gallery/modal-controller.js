import { Controller } from '../stimulus/stimulus.js';

// Native dialog owns the top layer, inert background, and focus containment.
// Features own form state and veto all dismissal paths with modal:before-close.
export default class extends Controller {
  static targets = ['initialFocus'];

  open(event) {
    if (this.element.open) return;
    this.returnFocus = event.detail?.trigger || document.activeElement;
    this.element.showModal();
    if (this.hasInitialFocusTarget) this.initialFocusTarget.focus();
  }

  dismiss() {
    if (!this.element.open || this.dispatch('before-close').defaultPrevented) return;
    this.element.close();
    // Restore synchronously. A queued native close event from an earlier open
    // must not steal focus from a reopened dialog or a new htmx result.
    if (this.returnFocus?.isConnected) this.returnFocus.focus();
    this.returnFocus = null;
    this.dispatch('closed');
  }

  cancel(event) {
    event.preventDefault();
    this.dismiss();
  }

  wrapFocus(event) {
    if (event.key !== 'Tab') return;
    // Native modality makes the page inert, but Chromium otherwise tabs out
    // to browser chrome at the last field. Keep this dialog's cycle explicit.
    const fields = [...this.element.querySelectorAll('a[href], button, input, select, textarea, [tabindex]')]
      .filter(field => field.tabIndex >= 0 && !field.matches(':disabled') && field.getClientRects().length);
    const first = fields[0];
    const last = fields.at(-1);
    if (!first) {
      event.preventDefault();
      this.element.focus();
    } else if (event.shiftKey && document.activeElement === first || !event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      (event.shiftKey ? last : first).focus();
    }
  }

  backdrop(event) {
    if (event.target !== this.element) return;
    const bounds = this.element.getBoundingClientRect();
    if (event.clientX < bounds.left || event.clientX > bounds.right ||
        event.clientY < bounds.top || event.clientY > bounds.bottom) this.dismiss();
  }

  disconnect() {
    // Removal releases native modal state without emitting feature mutations
    // or trying to focus a trigger from a region that is being removed.
    if (this.element.open) this.element.close();
    this.returnFocus = null;
  }
}
