import { Controller } from '../stimulus/stimulus.js';

// Native dialog supplies modal focus containment and top-layer placement.
// Keep its root stable while htmx refreshes the server-rendered options.
export default class FilterSheetController extends Controller {
  static targets = ['dialog', 'trigger'];

  open() {
    if (this.dialogTarget.open) return;
    this.dialogTarget.showModal();
    this.dialogTarget.querySelector('a, button')?.focus();
  }

  close() {
    if (this.dialogTarget.open) this.dialogTarget.close();
  }

  cancel(event) {
    event.preventDefault();
    this.close();
  }

  backdrop(event) {
    if (event.target !== this.dialogTarget) return;
    const rect = this.dialogTarget.getBoundingClientRect();
    if (event.clientX < rect.left || event.clientX > rect.right ||
        event.clientY < rect.top || event.clientY > rect.bottom) this.close();
  }

  // The close event is a queued task, so the user may already have moved on
  // by the time it fires. Return focus only if nothing else has taken it.
  closed() {
    const active = document.activeElement;
    if (!active || active === document.body || this.dialogTarget.contains(active)) {
      this.triggerTarget.focus();
    }
  }

  disconnect() {
    this.close();
  }
}
