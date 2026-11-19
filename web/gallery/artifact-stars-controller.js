import { Controller } from '../stimulus/stimulus.js';
import { viewerFetch } from './api-module.js';

export default class extends Controller {
  connect() {
    this.pending = new Set();
    this.abort = new AbortController();
    this.element.addEventListener('exhibit:star', event => this.star(event.detail), {signal: this.abort.signal});
  }

  async star({artifactID, starred}) {
    if (!artifactID || this.pending.has(artifactID)) return;
    this.pending.add(artifactID);
    const error = this.element.querySelector('[data-gallery-action-error]');
    error.hidden = true;
    try {
      const response = await viewerFetch('/api/artifacts/' + encodeURIComponent(artifactID) + '/starred', {
        method: 'PUT', body: JSON.stringify({starred: starred === 'true'}), signal: this.abort.signal,
      });
      if (!response.ok) throw new Error('Could not save star');
      if (!this.abort.signal.aborted) this.element.dispatchEvent(new CustomEvent('exhibit:artifacts-changed', {bubbles: true}));
    } catch {
      if (!this.abort.signal.aborted) error.hidden = false;
    } finally {
      this.pending.delete(artifactID);
    }
  }

  disconnect() { this.abort.abort(); }
}
