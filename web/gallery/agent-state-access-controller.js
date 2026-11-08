import { Controller } from '../stimulus/stimulus.js';
import { apiFetch } from './api-module.js';

// Saves the owner's answer to whether agents may read and change saved data
// (Exh-dabd) the moment it changes. Settings uses it for the account default
// (a checkbox, sent as a boolean); the edit page for one artifact's override (a
// select, sent as its value). The server holds the answer; on a failed save the
// control goes back to what was last saved, so it never shows a choice that
// did not take.
export default class extends Controller {
  static targets = ['input', 'status'];
  static values = { url: String, method: String, field: String };

  connect() { this.saved = this.current(); }

  current() {
    const input = this.inputTarget;
    return input.type === 'checkbox' ? input.checked : input.value;
  }

  show(value) {
    const input = this.inputTarget;
    if (input.type === 'checkbox') input.checked = value;
    else input.value = value;
  }

  say(message, error = false) {
    if (!this.hasStatusTarget) return;
    this.statusTarget.textContent = message;
    this.statusTarget.hidden = false;
    this.statusTarget.classList.toggle('is-error', error);
  }

  async save() {
    const value = this.current();
    this.inputTarget.disabled = true;
    this.say('Saving…');
    try {
      const response = await apiFetch(this.urlValue, {
        method: this.methodValue, body: JSON.stringify({ [this.fieldValue]: value }),
      });
      if (!this.element.isConnected) return;
      if (!response.ok) {
        const body = await response.text();
        let message = body.trim() || `Request failed (${response.status})`;
        try { message = JSON.parse(body).error || message; } catch { /* plain text */ }
        throw new Error(message);
      }
      this.saved = value;
      this.say('Saved.');
    } catch (err) {
      if (!this.element.isConnected) return;
      this.show(this.saved);
      this.say(`Not saved: ${err.message}`, true);
    } finally {
      this.inputTarget.disabled = false;
    }
  }
}
