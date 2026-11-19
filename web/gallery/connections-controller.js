import { Controller } from '../stimulus/stimulus.js';
import { apiFetch, viewerFetch } from './api-module.js';

export default class extends Controller {
  static values = { artifactId: String, returnUrl: String };
  static targets = ['provider', 'fields', 'createForm', 'createError', 'createButton', 'feedback', 'destination', 'destinationHost', 'createWarning', 'createWarningText', 'confirmButton'];

  connect() { this.active = true; if (this.hasProviderTarget) this.providerChanged(); }

  disconnect() { this.active = false; this.vendorAbort?.abort(); }

  providerChanged() {
    const provider = this.providerTarget.value.split(':')[0];
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
    if (this.providerTarget.value !== 'basic') { this.destinationChanged(); return; }
    const fields = this.createFormTarget.elements;
    const usingURL = fields.access_url.value !== '';
    for (const name of ['origin', 'username', 'password']) fields[name].disabled = usingURL;
    this.destinationChanged();
  }

  // Name the destination in the form itself, before the credential is pasted.
  // The credential is only ever sent to one host, and a person deciding whether
  // to trust this setup should see which host that is without submitting.
  // Read the recipe that is active now: a Basic access_url keeps its value when
  // the person switches to another recipe, but that field is disabled and is
  // not part of what the next submit sends.
  destinationChanged() {
    if (!this.hasDestinationTarget) return;
    const fields = this.createFormTarget.elements;
    const recipe = this.providerTarget.value.split(':')[0];
    let raw = '';
    if (recipe === 'basic') raw = fields.access_url?.value || fields.origin?.value || '';
    else if (recipe === 'api-key' || recipe === 'bearer') raw = fields.origin?.value || '';
    const host = hostOf(raw);
    this.destinationTarget.hidden = host === '';
    this.destinationHostTarget.textContent = host;
  }

  // A warning is about the values that were submitted. Once any field changes,
  // those values are no longer on screen, so the old confirmation cannot stand:
  // hide it and make the person submit the form again.
  formEdited() {
    this.formRevision = (this.formRevision || 0) + 1;
    this.clearCreateWarning();
  }

  async request(path, options = {}) {
    const response = await viewerFetch(path, options);
    const data = response.status === 204 ? null : await response.json().catch(() => null);
    if (!response.ok) {
      const error = new Error(data?.error || 'The change could not be saved.');
      // Answerable refusals carry a code and the details the copy needs; the
      // callers that care read them, everything else sees the message alone.
      if (data && typeof data === 'object') Object.assign(error, { code: data.code, details: data });
      throw error;
    }
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
      this.clearCreateWarning();
      const revision = this.formRevision || 0;
      this.createErrorTarget.textContent = '';
      const data = Object.fromEntries(new FormData(this.createFormTarget));
      const [recipe, vendorProvider] = data.recipe.split(':');
      data.recipe = recipe;
      data.vendor_provider = vendorProvider;
      data.artifact_id = this.artifactIdValue;
      for (const field of ['daily_limit', 'cache_minutes']) {
        if (field in data) data[field] = Number(data[field] || 0);
      }
      const vendorWindow = data.recipe === 'vendor' ? window.open('about:blank', '_blank') : null;
      if (vendorWindow) vendorWindow.opener = null;
      try {
        if (data.recipe === 'github') {
          const result = await this.request('/api/connections/oauth/github/begin', { method: 'POST', body: JSON.stringify({ name: data.name, artifact_id: this.artifactIdValue }) });
          window.location.assign(result.url);
          return;
        }
        if (data.recipe === 'vendor') {
          if (!vendorWindow) throw new Error('Allow popups to connect this provider, then try again.');
          const result = await this.connectVendor(data, vendorWindow);
          this.setupFinished(result.connection.id);
        } else {
          await this.saveNative(data, { revision });
        }
      } catch (error) { vendorWindow?.close(); this.createErrorTarget.textContent = error.message; }
    });
  }

  // The destination warning is a decision, not a failure: saving the same form
  // again with the person's confirmation is the only way past it.
  async saveNative(data, { confirmed = false, revision = this.formRevision || 0 } = {}) {
    try {
      const connection = await this.request('/api/connections', { method: 'POST', body: JSON.stringify(confirmed ? { ...data, confirm_origin_mismatch: true } : data) });
      this.setupFinished(connection.id);
    } catch (error) {
      if (error.code !== 'credential_origin_mismatch' || confirmed) throw error;
      // The answer describes the form as it was at submit time. If the person
      // edited it while the request was in flight, that warning is obsolete.
      if (revision !== (this.formRevision || 0)) return;
      this.pendingCreate = { data, revision };
      this.showCreateWarning(error.details);
    }
  }

  showCreateWarning(details) {
    const destination = hostOf(details.origin) || details.origin;
    const expected = hostOf(details.expected_origin) || details.expected_origin;
    this.createWarningTextTarget.textContent = 'This looks like a ' + (details.credential || 'credential') +
      ', which normally goes to ' + expected + '. Saving it here sends it to ' + destination + ' instead.';
    this.confirmButtonTarget.textContent = 'Send to ' + destination + ' anyway';
    this.createWarningTarget.hidden = false;
    this.createWarningTarget.scrollIntoView({ block: 'nearest' });
  }

  clearCreateWarning() {
    if (this.hasCreateWarningTarget) this.createWarningTarget.hidden = true;
    this.pendingCreate = null;
  }

  confirmCreate(event) {
    this.action(event, async () => {
      const pending = this.pendingCreate;
      this.clearCreateWarning();
      if (pending) await this.saveNative(pending.data, { confirmed: true, revision: pending.revision });
    });
  }

  reconnect(event) {
    this.action(event, async () => {
      const { connectionId, connectionKind, provider, connectionName } = event.currentTarget.dataset;
      const context = { artifact_id: this.artifactIdValue, connection_id: connectionId };
      if (connectionKind === 'oauth') {
        const result = await this.request('/api/connections/oauth/github/begin', {
          method: 'POST', body: JSON.stringify({ ...context, name: connectionName }),
        });
        window.location.assign(result.url);
      } else if (connectionKind === 'vendor') {
        const popup = window.open('about:blank', '_blank');
        if (!popup) throw new Error('Allow popups to reconnect this account, then try again.');
        popup.opener = null;
        try {
          const result = await this.connectVendor({ name: connectionName, vendor_provider: provider, connection_id: connectionId }, popup);
          this.setupFinished(result.connection.id);
        } catch (error) { popup.close(); throw error; }
      }
    });
  }

  repair(event) {
    this.action(event, async () => {
      const form = event.currentTarget;
      const id = form.dataset.connectionId;
      const credentials = Object.fromEntries(new FormData(form));
      const connection = await this.request('/api/connections/' + encodeURIComponent(id) + '/reconnect', {
        method: 'POST', body: JSON.stringify(credentials),
      });
      form.reset();
      this.setupFinished(connection.id);
    });
  }

  setupFinished(connectionId) {
    // The base URL is rendered by Exhibit, never supplied by an artifact.
    const destination = new URL(this.returnUrlValue, window.location.href);
    destination.searchParams.set('connection', connectionId);
    destination.searchParams.set('connected', '1');
    destination.hash = this.artifactIdValue ? 'connection-access' : '';
    window.location.assign(destination.href);
  }

  revoke(event) {
    this.action(event, async () => {
      const { connectionId, connectionName } = event.currentTarget.dataset;
      if (!window.confirm('Disconnect ' + connectionName + '? Artifacts using this account will lose access. Saved results will remain.')) return;
      await this.request('/api/connections/' + encodeURIComponent(connectionId), { method: 'DELETE' });
      this.changed('Account disconnected. Saved results remain.');
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
          if (!response.ok) throw new Error('Could not allow the account’s service.');
        }
      }
      await this.request(this.artifactPath('/connections'), { method: 'POST', body: JSON.stringify({ connection_id: connectionId }) });
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
      if (!window.confirm('Permanently clear saved data for this artifact? If you own it, this also clears data in saved versions; otherwise, it clears your saved data. This cannot be undone. Account and network permissions stay unchanged. Open pages may still hold copies.')) return;
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
    const result = await this.request('/api/connections/vendor/begin', { method: 'POST', body: JSON.stringify({ provider: data.vendor_provider, artifact_id: this.artifactIdValue, connection_id: data.connection_id }), signal: this.vendorAbort.signal });
    popup.location.replace(result.url);
    this.feedbackTarget.textContent = 'Complete consent in the provider window. This page will update when your account is connected.';
    const deadline = Date.now() + 10 * 60 * 1000;
    while (this.active && Date.now() < deadline) {
      const response = await this.request('/api/connections/vendor/complete', { method: 'POST', body: JSON.stringify({ session_id: result.session_id, name: data.name }), signal: this.vendorAbort.signal });
      if (response.status === 'connected') { popup.close(); return response; }
      if (popup.closed) throw new Error('The provider window was closed before connecting. Try again when ready.');
      await new Promise(resolve => setTimeout(resolve, 2000));
    }
    throw new Error('The connection session expired. Start again to reconnect.');
  }

  artifactPath(suffix) { return '/api/artifacts/' + encodeURIComponent(this.artifactIdValue) + suffix; }
}

// hostOf is the host an origin or credential URL names, for display only. It
// never leaves the browser, and it strips any credentials a pasted Basic URL
// carries rather than echoing them into the page.
function hostOf(value) {
  try {
    return new URL(value).host.toLowerCase();
  } catch {
    return '';
  }
}
