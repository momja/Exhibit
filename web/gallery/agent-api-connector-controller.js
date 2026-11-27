import { Controller } from '../stimulus/stimulus.js';
import { respondToInteraction, refreshInteraction } from './agent-interactions.js';

export default class extends Controller {
  static targets = ['key', 'trust'];

  disconnect() { if (this.hasKeyTarget) this.keyTarget.value = ''; }

  async approve(event) {
    event.preventDefault();
    if (!this.element.reportValidity()) return;
    const secret = this.keyTarget.value;
    this.keyTarget.value = '';
    await this.send({ action: 'approve', secret, trusted_origin: this.trustTarget.dataset.origin });
  }

  async resume(event) {
    event.preventDefault();
    // The durable decision is immutable; this only redelivers its safe status.
    await this.send({ action: 'deny' });
  }

  async deny() {
    this.keyTarget.value = '';
    await this.send({ action: 'deny' });
  }

  async send(body) {
    const card = this.element.closest('[data-interaction-id]');
    const sent = await respondToInteraction(card.dataset.sessionId, card.dataset.interactionId, body);
    if (sent || !this.element.isConnected || card.dataset.pending !== 'true') return;
    const message = card.querySelector('.interaction-status').textContent;
    // A failed reply to Pi may follow a committed decision. Reconcile it now
    // so Continue can deliver the outcome without collecting another key.
    await refreshInteraction(card.dataset.sessionId, card.dataset.interactionId);
    const updated = document.getElementById(card.id);
    if (updated?.dataset.pending === 'true' && updated.querySelector('.connector-key')) {
      updated.querySelector('.interaction-status').textContent = message +
        (body.action === 'approve' ? ' Enter your key and confirm trust again to retry.' : ' You can try Decline again.');
    }
  }
}
