import { modalFor } from './modal.js';
import { viewerFetch, apiFetch } from './api-module.js';

/* Host half of window.exhibit.ai (Exh-zue3).
 *
 * The artifact frame cannot reach the API: its preamble posts each AI call
 * here, pinned to the app origin. This module answers it with the page's own
 * credential, after the two consents the server also enforces:
 *
 *   - the owner allows the tool to use AI at all (inference_approved), which is
 *     also their agreement to spend their own allowance;
 *   - a named recipient of an allowed tool agrees to spend THEIR allowance.
 *
 * A recipient of a tool its owner never allowed is not asked anything: the
 * banner says only the owner can turn AI on. Every refusal settles the
 * artifact's promise with a stable code, so a tool can always degrade.
 *
 * It listens to every iframe marked data-ai-frame: the artifact page's frame,
 * the agent preview and a version preview. A frame is identified by its
 * window, never its origin, which is opaque.
 *
 *   connectInferenceHost({ artifactId: () => id, title: () => text })
 */

const REFUSED = {
  unavailable: 'AI is not available here.',
  not_approved: 'This tool has not been allowed to use AI.',
};

export function connectInferenceHost({ artifactId, title }) {
  const dialog = document.getElementById('ai-modal');
  const banner = document.getElementById('ai-banner');
  const states = new Map(); // artifact id -> Promise<state>
  const decisions = new Map(); // artifact id -> Promise<boolean> while asking
  const bannered = new Set();

  function stateFor(id, fresh = false) {
    if (fresh || !states.has(id)) {
      states.set(id, viewerFetch('/api/artifacts/' + encodeURIComponent(id) + '/inference')
        .then(r => (r.ok ? r.json() : {}))
        .catch(() => ({})));
    }
    return states.get(id);
  }

  function frameFor(source) {
    for (const frame of document.querySelectorAll('iframe[data-ai-frame]')) {
      if (frame.contentWindow === source) return frame;
    }
    return null;
  }

  function reply(frame, id, artifact, result) {
    frame.contentWindow?.postMessage({ __avInferenceResult: true, artifactId: artifact, id, ...result }, '*');
  }

  function showBanner(key, text, withKeyLink = false) {
    if (!banner || bannered.has(key)) return;
    bannered.add(key);
    document.getElementById('ai-banner-text').textContent = text;
    document.getElementById('ai-banner-action').hidden = !withKeyLink;
    banner.hidden = false;
  }
  document.getElementById('ai-banner-dismiss')?.addEventListener('click', () => { banner.hidden = true; });

  // Asks once per artifact however many calls arrive while the dialog is up;
  // every waiting call settles with the one answer.
  function ask(id, st, frame) {
    if (decisions.has(id)) return decisions.get(id);
    const decision = new Promise(resolve => {
      if (!dialog) { resolve(false); return; }
      const modal = modalFor(dialog);
      const allow = document.getElementById('ai-allow');
      const status = dialog.querySelector('[data-permission-status]');
      dialog.dataset.aiMode = st.owner ? 'owner' : 'recipient';
      const titleEl = dialog.querySelector('[data-ai-artifact-title]');
      if (titleEl && title) titleEl.textContent = title() || titleEl.textContent;
      dialog.querySelector('[data-ai-payer]').textContent = st.payer === 'own_key'
        ? 'Requests are billed to your own AI key.'
        : (st.owner ? 'Requests use your Exhibit AI allowance.' : 'Requests use your own Exhibit AI allowance, not the owner’s.');
      status.textContent = '';
      allow.disabled = false;
      let settled = false;
      const finish = value => {
        if (settled) return;
        settled = true;
        dialog.removeEventListener('modal:closed', onClosed);
        allow.removeEventListener('click', onAllow);
        resolve(value);
      };
      const onClosed = () => finish(false);
      const onAllow = async () => {
        allow.disabled = true;
        const path = '/api/artifacts/' + encodeURIComponent(id);
        const res = await (st.owner
          ? apiFetch(path, { method: 'PATCH', body: JSON.stringify({ inference_approved: true }) })
          : viewerFetch(path + '/inference/consent', { method: 'PUT' })).catch(() => null);
        if (!res || !res.ok) {
          status.textContent = '✗ Could not save your choice. Try again.';
          allow.disabled = false;
          return;
        }
        st.approved = true;
        st.consented = true;
        finish(true);
        modal.close();
        if (st.owner) document.body.dispatchEvent(new CustomEvent('exhibit:capabilities-changed', { bubbles: true }));
      };
      dialog.addEventListener('modal:closed', onClosed);
      allow.addEventListener('click', onAllow);
      modal.show(frame);
    }).finally(() => decisions.delete(id));
    decisions.set(id, decision);
    return decision;
  }

  async function handle(frame, d) {
    const id = String(d.artifactId);
    const callID = String(d.id);
    if (!artifactId() || id !== artifactId()) return;
    const st = await stateFor(id);
    const usable = st.enabled && (st.owner || st.approved);
    if (d.op === 'available') {
      reply(frame, callID, id, { ok: true, value: { complete: Boolean(usable && st.complete), decide: Boolean(usable && st.decide) } });
      return;
    }
    if (d.op !== 'complete' && d.op !== 'decide') return;
    const fail = (code, message) => reply(frame, callID, id, { ok: false, error: { code, message: message || REFUSED[code] || 'The AI request failed.' } });
    if (!st.enabled) { fail('unavailable'); return; }
    if (!st.complete && !st.decide && st.reason === 'no_key') {
      showBanner('no_key', 'This tool tried to use AI, but you have not added an AI key yet.', true);
      fail('no_key', 'Add your own AI key to use AI in artifacts.');
      return;
    }
    if (!st.approved && !st.owner) {
      showBanner('owner', 'This tool tried to use AI, but its owner has not allowed that. Only the owner can turn it on.');
      fail('not_approved');
      return;
    }
    if (!st.consented) {
      if (!(await ask(id, st, frame))) { fail('not_approved', 'AI use was not allowed.'); return; }
    }
    const res = await viewerFetch('/api/artifacts/' + encodeURIComponent(id) + '/inference', {
      method: 'POST', body: JSON.stringify({ kind: d.op, payload: d.payload ?? null }),
    }).catch(() => null);
    if (!res) { fail('provider_error', 'The AI request could not be sent.'); return; }
    const body = await res.json().catch(() => ({}));
    if (res.ok) { reply(frame, callID, id, { ok: true, value: body.value }); return; }
    const err = body.error && typeof body.error === 'object' ? body.error : { code: 'provider_error' };
    if (err.code === 'limit_reached') showBanner('limit', err.message || 'The AI allowance is used up for this month.');
    if (err.code === 'no_key') showBanner('no_key', 'This tool tried to use AI, but you have not added an AI key yet.', true);
    if (err.code === 'not_approved') stateFor(id, true); // a decision changed elsewhere
    fail(err.code || 'provider_error', err.message);
  }

  function onMessage(e) {
    const d = e.data;
    if (!d || d.__avInference !== true) return;
    const frame = frameFor(e.source);
    if (!frame) return;
    handle(frame, d);
  }
  window.addEventListener('message', onMessage);
  // A decision made on another page (Edit → Permissions, Settings) must not
  // be outlived by this page's cached copy.
  const refresh = () => states.clear();
  document.addEventListener('visibilitychange', refresh);
  document.body.addEventListener('exhibit:capabilities-changed', refresh);
  return () => {
    window.removeEventListener('message', onMessage);
    document.removeEventListener('visibilitychange', refresh);
    document.body.removeEventListener('exhibit:capabilities-changed', refresh);
  };
}
