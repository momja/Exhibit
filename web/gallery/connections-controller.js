import { Controller } from '../stimulus/stimulus.js';
import { apiFetch, viewerFetch } from './api-module.js';

export default class extends Controller {
  static values = { artifactId: String };
  static targets = ['provider', 'fields', 'createForm', 'createError', 'createButton', 'feedback'];

  connect() { this.active = true; if (this.hasProviderTarget) this.providerChanged(); }

  disconnect() { this.active = false; this.vendorAbort?.abort(); }

  providerChanged() {
    const provider = this.providerTarget.value;
    for (const section of this.fieldsTargets) {
      const active = section.dataset.recipes.split(' ').includes(provider);
      section.hidden = !active;
      for (const input of section.querySelectorAll('input,select,textarea,button')) input.disabled = !active;
    }
    this.basicURLChanged();
    // Text is interaction state, not a replacement for server-rendered UI.
    this.createButtonTarget.textContent = provider === 'github' ? 'Continue to GitHub' : provider === 'vendor' ? 'Continue to provider' : 'Save connection';
  }

  basicURLChanged() {
    if (this.providerTarget.value !== 'basic') return;
    const fields = this.createFormTarget.elements;
    const usingURL = fields.access_url.value !== '';
    for (const name of ['origin', 'username', 'password']) fields[name].disabled = usingURL;
  }

  async request(path, options = {}) {
    const response = await viewerFetch(path, options);
    const data = response.status === 204 ? null : await response.json();
    if (!response.ok) throw new Error(data?.error || 'The change could not be saved.');
    return data;
  }

  changed(message) {
    this.feedbackTarget.textContent = message;
    this.feedbackTarget.classList.remove('is-error');
    document.body.dispatchEvent(new CustomEvent('exhibit:connections-changed', { bubbles: true }));
  }

  async action(event, work) {
    event.preventDefault();
    const trigger = event.submitter || event.currentTarget;
    if (trigger.disabled) return;
    trigger.disabled = true;
    try { await work(); }
    catch (error) { this.feedbackTarget.textContent = error.message; this.feedbackTarget.classList.add('is-error'); this.feedbackTarget.scrollIntoView({ block: 'nearest' }); }
    finally { trigger.disabled = false; }
  }

  async create(event) {
    await this.action(event, async () => {
      this.createErrorTarget.textContent = '';
      const data = Object.fromEntries(new FormData(this.createFormTarget));
      for (const field of ['daily_limit', 'cache_minutes']) {
        if (field in data) data[field] = Number(data[field] || 0);
      }
      const vendorWindow = data.recipe === 'vendor' ? window.open('about:blank', '_blank') : null;
      if (vendorWindow) vendorWindow.opener = null;
      try {
        if (data.recipe === 'github') {
          const result = await this.request('/api/connections/oauth/github/begin', { method: 'POST', body: JSON.stringify({ name: data.name }) });
          window.location.assign(result.url);
          return;
        }
        if (data.recipe === 'vendor') {
          if (!vendorWindow) throw new Error('Allow popups to connect this provider, then try again.');
          await this.connectVendor(data, vendorWindow);
        } else {
        await this.request('/api/connections', { method: 'POST', body: JSON.stringify(data) });
        }
        this.createFormTarget.reset();
        this.providerChanged();
        this.changed('Connection saved. Choose an artifact to grant access.');
      } catch (error) { vendorWindow?.close(); this.createErrorTarget.textContent = error.message; }
    });
  }

  reconnect(event) {
    const { connectionKind, provider, connectionName } = event.currentTarget.dataset;
    this.providerTarget.value = connectionKind === 'vendor' ? 'vendor' : provider;
    this.createFormTarget.elements.name.value = connectionName;
    if (connectionKind === 'vendor') {
      this.createFormTarget.elements.vendor_provider.value = provider;
      if (!this.createFormTarget.elements.vendor_provider.value) {
        this.feedbackTarget.textContent = 'This provider is no longer offered. Choose an available provider or revoke the saved connection.';
        this.feedbackTarget.classList.add('is-error');
      }
    }
    this.providerChanged();
    this.createFormTarget.scrollIntoView({ block: 'center', behavior: 'smooth' });
    this.createFormTarget.elements.name.focus({ preventScroll: true });
  }

  revoke(event) {
    this.action(event, async () => {
      const { connectionId, connectionName } = event.currentTarget.dataset;
      if (!window.confirm('Revoke ' + connectionName + '? Artifacts using this connection will lose access.')) return;
      await this.request('/api/connections/' + encodeURIComponent(connectionId), { method: 'DELETE' });
      this.changed('Connection revoked.');
    });
  }

  grant(event) {
    this.action(event, async () => {
      const data = new FormData(event.currentTarget);
      const connectionId = data.get('connection');
      if (data.get('approve_origins') === 'on') {
        const result = await this.request('/api/connections');
        const connection = result.connections.find(item => item.id === connectionId);
        if (!connection) throw new Error('Choose a current connection.');
        for (const origin of connection.origins) {
          const response = await apiFetch('/api/artifacts/' + encodeURIComponent(this.artifactIdValue) + '/origins', { method: 'POST', body: JSON.stringify({ origin, decision: 'allow', source: 'connection-consent' }) });
          if (!response.ok) throw new Error('Could not approve the connection origin.');
        }
      }
      await this.request(this.artifactPath('/connections'), { method: 'POST', body: JSON.stringify({ connection_id: connectionId, share_with_recipients: data.get('sharing') === 'on' }) });
      this.changed('Connection access granted.');
    });
  }

  ungrant(event) {
    this.action(event, async () => {
      await this.request(this.artifactPath('/connections/' + encodeURIComponent(event.currentTarget.dataset.connectionId)), { method: 'DELETE' });
      this.changed('Connection access removed.');
    });
  }

  clearData(event) {
    this.action(event, async () => {
      if (!window.confirm('Clear this artifact’s saved data? This also clears saved version data when you own the artifact. Connection grants and network approvals stay in place. Open pages may still hold copies.')) return;
      await this.request(this.artifactPath('/connection-data'), { method: 'DELETE' });
      this.changed('Saved data cleared.');
    });
  }

  saveRelay(event) {
    this.action(event, async () => {
      const data = new FormData(event.currentTarget);
      await this.request(this.artifactPath('/relay-grant'), { method: 'PUT', body: JSON.stringify({ allowed: data.get('allowed') === 'on' }) });
      this.changed('Relay permissions saved.');
    });
  }

  schedule(event) {
    this.action(event, async () => {
      const data = Object.fromEntries(new FormData(event.currentTarget));
      data.interval_hours = Number(data.interval_hours);
      await this.request(this.artifactPath('/relay-schedules'), { method: 'POST', body: JSON.stringify(data) });
      this.changed('Refresh scheduled. The artifact can be closed.');
    });
  }

  deleteSchedule(event) {
    this.action(event, async () => {
      await this.request(this.artifactPath('/relay-schedules/' + encodeURIComponent(event.currentTarget.dataset.scheduleId)), { method: 'DELETE' });
      this.changed('Scheduled refresh removed.');
    });
  }

  dismissRequest(event) {
    this.action(event, async () => {
      await this.request(this.artifactPath('/connection-requests/' + encodeURIComponent(event.currentTarget.dataset.requestId)), { method: 'DELETE' });
      this.changed('Request dismissed.');
    });
  }

  async connectVendor(data, popup) {
    this.vendorAbort = new AbortController();
    const result = await this.request('/api/connections/vendor/begin', { method: 'POST', body: JSON.stringify({ provider: data.vendor_provider }), signal: this.vendorAbort.signal });
    popup.location.replace(result.url);
    this.feedbackTarget.textContent = 'Complete consent in the provider window. This page will update when your account is connected.';
    const deadline = Date.now() + 10 * 60 * 1000;
    while (this.active && Date.now() < deadline) {
      const response = await this.request('/api/connections/vendor/complete', { method: 'POST', body: JSON.stringify({ session_id: result.session_id, name: data.name }), signal: this.vendorAbort.signal });
      if (response.status === 'connected') { popup.close(); return; }
      if (popup.closed) throw new Error('The provider window was closed before connecting. Try again when ready.');
      await new Promise(resolve => setTimeout(resolve, 2000));
    }
    throw new Error('The connection session expired. Start again to reconnect.');
  }

  artifactPath(suffix) { return '/api/artifacts/' + encodeURIComponent(this.artifactIdValue) + suffix; }
}
