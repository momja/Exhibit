import { Controller } from '../stimulus/stimulus.js';
import { viewerFetch } from './api-module.js';
import { requestRelay } from './relay-request.js';

// The host pins artifact identity and frame identity. Frame messages supply
// request data only; they cannot select another artifact or grant themselves
// authority. Upstream bytes are returned as data and never touch app markup.
export default class extends Controller {
  static values = { artifactId: String, frameSelector: String, policy: String };
  static targets = ['consent', 'destination', 'error', 'allow', 'framePolicy'];

  connect() {
    // An initially unbound agent page can create its first artifact. The
    // server supplies an exact policy once that identity exists, and this
    // additional CSP intersects the page header before its frame starts.
    this.requests = new Map();
    this.active = true;
    this.generation = 0;
    this.acceptChannel = true;
    this.hasLoaded = false;
    this.consentQueue = Promise.resolve();
    if (this.hasFramePolicyTarget) {
      const next = this.framePolicyTarget.content.querySelector('meta');
      const existing = document.head.querySelector('meta[data-exhibit-frame-policy]');
      if (existing && existing.content !== next?.content) {
        // CSP policies persist for this document's lifetime. Replacing a
        // template cannot widen its earlier exact private-policy path.
        this.active = false;
        window.location.reload();
        return;
      }
      const policy = this.framePolicyTarget.content.cloneNode(true);
      policy.querySelector('meta')?.setAttribute('data-exhibit-frame-policy', '');
      document.head.appendChild(policy);
    }
    this.onMessage = event => this.receive(event);
    window.addEventListener('message', this.onMessage);
    this.frame = this.element.querySelector(this.frameSelectorValue);
    this.onLoad = () => {
      if (!this.hasLoaded) { this.hasLoaded = true; return; }
      this.resetChannel(false);
    };
    this.frame?.addEventListener('load', this.onLoad);
    this.navigationObserver = new MutationObserver(records => {
      if (records.some(record => record.attributeName === 'src')) this.resetChannel(true);
    });
    if (this.frame) {
      this.navigationObserver.observe(this.frame, { attributes: true, attributeFilter: ['src'] });
      if (this.frame.dataset.relaySrc) this.frame.src = this.frame.dataset.relaySrc;
    }
  }

  disconnect() {
    this.active = false;
    this.navigationObserver?.disconnect();
    this.frame?.removeEventListener('load', this.onLoad);
    window.removeEventListener('message', this.onMessage);
    this.resetChannel(false);
  }

  resetChannel(accept) {
    this.generation++;
    this.acceptChannel = accept;
    this.hasLoaded = false;
    this.port?.close();
    this.port = null;
    for (const controller of this.requests.values()) controller.abort();
    this.requests.clear();
    if (this.resolveConsent) this.finishConsent(false);
    if (this.hasConsentTarget && this.consentTarget.open) this.consentTarget.close();
  }

  receive(event) {
    const frame = this.element.querySelector(this.frameSelectorValue);
    if (!frame || event.source !== frame.contentWindow || event.origin !== 'null') return;
    if (!this.active || !this.acceptChannel || event.data?.__exhibitRelayChannel !== true || !event.ports?.[0]) return;
    this.acceptChannel = false;
    this.port = event.ports[0];
    const port = this.port;
    const generation = this.generation;
    this.port.onmessage = message => {
      if (this.port === port && this.generation === generation && this.active) this.receiveRequest(message.data);
    };
    this.port.start();
  }

  async receiveRequest(data) {
    const frame = this.frame;
    const port = this.port;
    const generation = this.generation;
    if (!this.active || !port || !frame?.isConnected) return;
    if (!data || typeof data.id !== 'string' || data.id.length > 120) return;
    if (data.__exhibitConnection === true) {
      await this.connectionRequest(data, port, generation);
      return;
    }
    if (data.__exhibitRelayAbort === true) {
      this.requests.get(data.id)?.abort();
      return;
    }
    if (data.__exhibitRelay !== true || this.requests.has(data.id)) return;
    const input = data.request;
    if (!input || typeof input.url !== 'string' || input.url.length > 8192 ||
        typeof input.method !== 'string' || typeof input.body_base64 !== 'string' ||
        input.body_base64.length > 1398104 || this.requests.size >= 16) return;
    const controller = new AbortController();
    this.requests.set(data.id, controller);
    try {
      const send = () => viewerFetch('/api/artifacts/' + encodeURIComponent(this.artifactIdValue) + '/relay', {
        method: 'POST', body: JSON.stringify(input), signal: controller.signal,
        headers: { 'X-Exhibit-Relay-Policy': this.policyValue }
      });
      let { response, result } = await requestRelay(send, input.method, controller.signal);
      if (result.code === 'relay_consent_required') {
        const consent = this.consentQueue.then(() => this.active && generation === this.generation && !controller.signal.aborted ? this.askConsent(input, controller.signal) : false);
        this.consentQueue = consent.catch(() => false);
        if (!await consent) throw new Error('Relay access was declined.');
        if (controller.signal.aborted) return;
        ({ response, result } = await requestRelay(send, input.method, controller.signal));
      }
      if (!response.ok) throw new Error(result.error || 'The relay request was refused.');
      if (controller.signal.aborted || generation !== this.generation || !this.active) return;
      port.postMessage({ __exhibitRelayResponse: true, id: data.id, response: result });
    } catch (error) {
      if (!controller.signal.aborted && frame.isConnected && generation === this.generation && this.active) {
        port.postMessage({ __exhibitRelayResponse: true, id: data.id, error: error.message || 'Relay request failed.' });
      }
    } finally {
      this.requests.delete(data.id);
    }
  }

  async connectionRequest(data, port, generation) {
    if (this.requests.has(data.id) || this.requests.size >= 16) return;
    const controller = new AbortController();
    this.requests.set(data.id, controller);
    try {
      let path = '/api/artifacts/' + encodeURIComponent(this.artifactIdValue) + '/connections';
      let options = { signal: controller.signal, headers: { 'X-Exhibit-Relay-Policy': this.policyValue } };
      if (data.action === 'standin' && typeof data.connection_id === 'string' && data.connection_id.length <= 100) {
        path += '/' + encodeURIComponent(data.connection_id) + '/standin';
        options.method = 'POST';
        options.body = '{}';
      } else if (data.action !== 'list') throw new Error('Unknown connection request.');
      const response = await viewerFetch(path, options);
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || 'Connection access was refused.');
      const value = data.action === 'list' ? result.grants.map(grant => grant.connection) : result;
      if (this.active && generation === this.generation && !controller.signal.aborted) port.postMessage({ __exhibitRelayResponse: true, id: data.id, connection: value });
    } catch (error) {
      if (this.active && generation === this.generation && !controller.signal.aborted) port.postMessage({ __exhibitRelayResponse: true, id: data.id, error: error.message });
    } finally { this.requests.delete(data.id); }
  }

  async askConsent(input, signal) {
    // Another request may have obtained the needed consent while queued.
    const response = await viewerFetch('/api/artifacts/' + encodeURIComponent(this.artifactIdValue) + '/relay-grant');
    if (!response.ok) throw new Error('Could not read relay permissions.');
    const grant = await response.json();
    if (!this.active || signal.aborted) return false;
    if (grant.allowed) return true;
    if (!this.hasConsentTarget) return false;
    this.destinationTarget.textContent = new URL(input.url).origin;
    this.errorTarget.textContent = '';
    this.returnFocus = document.activeElement;
    return new Promise(resolve => {
      const abort = () => this.finishConsent(false);
      this.resolveConsent = value => { signal.removeEventListener('abort', abort); resolve(value); };
      signal.addEventListener('abort', abort, { once: true });
      this.consentTarget.showModal();
    });
  }

  async approve() {
    const resolve = this.resolveConsent;
    const generation = this.generation;
    this.allowTarget.disabled = true;
    try {
      const response = await viewerFetch('/api/artifacts/' + encodeURIComponent(this.artifactIdValue) + '/relay-grant', {
        method: 'PUT', body: JSON.stringify({ allowed: true })
      });
      if (!response.ok) throw new Error('Could not save relay permission.');
      if (this.resolveConsent === resolve && this.generation === generation) this.finishConsent(true);
    } catch (error) { this.errorTarget.textContent = error.message; }
    finally { this.allowTarget.disabled = false; }
  }

  decline(event) {
    event?.preventDefault();
    this.finishConsent(false);
  }

  finishConsent(approved) {
    if (this.consentTarget.open) this.consentTarget.close();
    const resolve = this.resolveConsent;
    this.resolveConsent = null;
    this.returnFocus?.focus?.();
    resolve?.(approved);
  }
}
