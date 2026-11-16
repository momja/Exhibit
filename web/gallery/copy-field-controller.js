import { Controller } from '../stimulus/stimulus.js';

export default class extends Controller {
  static targets = ['input', 'button', 'icon', 'status'];

  connect() {
    this.connection = Symbol();
  }

  disconnect() {
    this.connection = null;
    clearTimeout(this.resetTimer);
    this.reset();
  }

  reset() {
    this.buttonTarget.disabled = false;
    this.iconTarget.className = 'ph ph-copy';
    this.statusTarget.textContent = '';
  }

  async copy() {
    if (this.buttonTarget.disabled) return;
    const connection = this.connection;
    clearTimeout(this.resetTimer);
    this.reset();
    this.buttonTarget.disabled = true;
    try {
      await navigator.clipboard.writeText(this.inputTarget.value);
      if (this.connection !== connection) return;
      this.iconTarget.className = 'ph ph-check';
      this.statusTarget.textContent = 'Copied.';
      this.resetTimer = setTimeout(() => this.reset(), 2000);
    } catch {
      if (this.connection !== connection) return;
      this.inputTarget.focus();
      this.inputTarget.select();
      this.statusTarget.textContent = 'Could not copy. Copy the selected text manually.';
    } finally {
      if (this.connection === connection) this.buttonTarget.disabled = false;
    }
  }
}
