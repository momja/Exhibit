import { Controller } from '../stimulus/stimulus.js';
import { apiFetch } from './api-module.js';

const confirmation = 'delete my library';

// The server owns confirmation copy and authority. One controller works both
// on /profile and in a Clerk custom page; Stimulus handles swaps and unmounts.
export default class extends Controller {
  static targets = ['trigger', 'panel', 'phrase', 'submit', 'cancel', 'status', 'finish'];
  static values = { hosted: Boolean };

  connect() { this.busy = false; }

  open() {
    if (this.busy || !this.hasPanelTarget) return;
    this.panelTarget.hidden = false;
    this.triggerTarget.setAttribute('aria-expanded', 'true');
    this.phraseTarget.focus();
  }

  cancel() {
    if (this.busy) return;
    this.panelTarget.hidden = true;
    this.triggerTarget.setAttribute('aria-expanded', 'false');
    this.phraseTarget.value = '';
    this.update();
    this.triggerTarget.focus();
  }

  update() {
    this.submitTarget.disabled = this.busy || this.phraseTarget.value.trim().toLowerCase() !== confirmation;
  }

  say(message, error = false) {
    this.statusTarget.textContent = message;
    this.statusTarget.hidden = false;
    this.statusTarget.classList.toggle('is-error', error);
  }

  async submit() {
    if (this.busy || this.phraseTarget.value.trim().toLowerCase() !== confirmation) return;
    this.busy = true;
    this.update();
    this.cancelTarget.disabled = true;
    this.say('Deleting your account…');
    try {
      const response = await apiFetch('/api/account', {
        method: 'DELETE', body: JSON.stringify({ confirm: confirmation }),
      });
      if (!this.element.isConnected) return;
      if (!response.ok) {
        const body = await response.text();
        let message = body.trim() || `Request failed (${response.status})`;
        try { message = JSON.parse(body).error || message; } catch { /* plain text */ }
        this.say(message, true);
        this.busy = false;
        this.cancelTarget.disabled = false;
        this.update();
        return;
      }
      if (response.status === 202) {
        this.say((await response.json()).message);
        this.panelTarget.hidden = true;
        this.triggerTarget.disabled = true;
        this.finishTarget.hidden = false;
        this.finishTarget.focus();
        return;
      }
      window.location.href = this.hostedValue ? '/auth/logout' : '/';
    } catch {
      if (!this.element.isConnected) return;
      this.say('Network error. The outcome is unknown — reload the page to check whether the account was deleted. Do not retry.', true);
      // Keep submission disabled: a lost response may already have erased it.
    }
  }
}
