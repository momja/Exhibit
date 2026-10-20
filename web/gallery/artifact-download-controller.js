import { Controller } from '../stimulus/stimulus.js';
import { apiFetch } from './api-module.js';
import { downloadBlob } from './download.js';

// One stable host per page, with choices rendered by artifactDownloadDialog.
// Menu events continue bubbling here after htmx replaces or appends cards.
export default class extends Controller {
  static targets = ['dialog', 'artifactTitle', 'choices', 'format', 'submit', 'cancel', 'status'];

  connect() {
    this.busy = false;
  }

  disconnect() {
    this.request?.abort();
    this.request = undefined;
    this.dialogTarget.close();
  }

  open(event) {
    event.preventDefault();
    if (this.busy || this.dialogTarget.open) return;
    const details = event.detail?.artifactID ? event.detail : event.params;
    const artifactID = details?.artifactID || details?.artifactId;
    if (!artifactID) return;
    this.artifactID = artifactID;
    this.trigger = details.trigger || document.getElementById(details.triggerId) || event.currentTarget;
    this.artifactTitleTarget.textContent = details.title ?? event.currentTarget.dataset.artifactDownloadTitle;
    this.formatTargets.forEach(input => { input.checked = input.value === 'zip'; });
    this.statusTarget.textContent = '';
    this.statusTarget.classList.remove('error');
    this.submitTarget.textContent = 'Download';
    this.controls();
    this.dialogTarget.showModal();
    this.formatTargets.find(input => input.checked).focus();
  }

  close(event) {
    event?.preventDefault();
    if (this.busy) return;
    this.dialogTarget.close();
    // Native close events are queued. Restore now so an older event cannot
    // steal focus from a menu the user has already reopened.
    this.restoreFocus();
  }

  restoreFocus() {
    if (!this.dialogTarget.open && this.trigger?.isConnected) this.trigger.focus({ preventScroll: true });
  }

  backdrop(event) {
    if (event.target !== this.dialogTarget) return;
    const bounds = this.dialogTarget.getBoundingClientRect();
    if (event.clientX < bounds.left || event.clientX > bounds.right ||
        event.clientY < bounds.top || event.clientY > bounds.bottom) this.close(event);
  }

  controls() {
    this.choicesTarget.disabled = this.busy;
    this.submitTarget.disabled = this.busy;
    this.cancelTarget.disabled = this.busy;
    this.dialogTarget.setAttribute('aria-busy', String(this.busy));
  }

  async download(event) {
    event.preventDefault();
    if (this.busy || !this.dialogTarget.open) return;
    const format = this.formatTargets.find(input => input.checked)?.value;
    if (!['zip', 'html'].includes(format)) return;
    this.busy = true;
    const request = this.request = new AbortController();
    this.controls();
    this.statusTarget.classList.remove('error');
    this.statusTarget.textContent = 'Preparing download…';
    try {
      const response = await apiFetch('/api/artifacts/' + encodeURIComponent(this.artifactID) + '/export?format=' + format,
        { signal: request.signal });
      if (!response.ok) {
        const content = await response.json().catch(() => ({}));
        throw new Error(content.error || 'HTTP ' + response.status);
      }
      const blob = await response.blob();
      // A disconnected host must not start a download after its page is gone.
      if (request.signal.aborted) return;
      const filename = response.headers.get('Content-Disposition')?.match(/filename=(?:"([^"]+)"|([^;]+))/i);
      downloadBlob(blob, filename?.[1] || filename?.[2]?.trim() || this.artifactID + '.' + format);
      this.busy = false;
      this.close();
    } catch (error) {
      if (request.signal.aborted) return;
      this.statusTarget.textContent = 'Could not download artifact: ' + error.message;
      this.statusTarget.classList.add('error');
      this.submitTarget.textContent = 'Try again';
    } finally {
      if (this.request === request) {
        this.busy = false;
        this.controls();
        if (this.dialogTarget.open && !request.signal.aborted) this.submitTarget.focus();
        this.request = undefined;
      }
    }
  }
}
