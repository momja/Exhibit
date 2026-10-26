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

  closed() {
    this.triggerTarget.focus();
  }

  disconnect() {
    this.close();
  }
}
