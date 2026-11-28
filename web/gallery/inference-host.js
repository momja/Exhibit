import { modalFor } from './modal.js';
import { viewerFetch, apiFetch } from './api-module.js';
import { prepareCompletePayload } from './complete-payload.js';

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
 * A complete() call is checked here by the server's rules for its messages
 * and images, and its images are stripped of their metadata, before anything
 * leaves the device (complete-payload.js, Exh-v81w). A call the server would
 * refuse is refused here, in the server's words, and never sent.
 *
 * It listens to every iframe marked data-ai-frame: the artifact page's frame,
 * the agent preview and a version preview. The host checks both the frame's
 * window and the channel shared with its artifact document.
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
  const states = new Map(); // viewer/artifact channel -> Promise<state>
  const decisions = new Map(); // viewer/artifact channel -> Promise<boolean> while asking
  const bannered = new Set();
  // "Not now" holds for this page load: a tool that calls again at once, or in
  // a loop, is refused without raising the dialog again. Reloading the page,
  // or allowing it under Edit → Permissions or Settings, asks afresh.
  const declined = new Set();

  function stateFor(id, channel, headers, fresh = false) {
    if (fresh || !states.has(channel)) {
      states.set(channel, viewerFetch('/api/artifacts/' + encodeURIComponent(id) + '/inference', { headers })
        .then(r => (r.ok ? r.json() : {}))
        .catch(() => ({})));
    }
    return states.get(channel);
  }

  function frameFor(source) {
    for (const frame of document.querySelectorAll('iframe[data-ai-frame]')) {
      if (frame.contentWindow === source) return frame;
    }
    return null;
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
  function ask(id, channel, headers, st, frame) {
    if (decisions.has(channel)) return decisions.get(channel);
    if (decisions.size) return Promise.resolve(false);
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
      const onClosed = () => { if (!settled) declined.add(channel); finish(false); };
      const onAllow = async () => {
        allow.disabled = true;
        const path = '/api/artifacts/' + encodeURIComponent(id);
        const res = await (st.owner
          ? apiFetch(path, { method: 'PATCH', headers, body: JSON.stringify({ inference_approved: true }) })
          : viewerFetch(path + '/inference/consent', { method: 'PUT', headers })).catch(() => null);
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
    }).finally(() => decisions.delete(channel));
    decisions.set(channel, decision);
    return decision;
  }

  async function handle(frame, d, reply) {
    const { artifactId: id, channel } = d;
    // Recheck the page-load viewer on the server, even when the browser has
    // since switched accounts and now sends a different session cookie.
    const headers = { 'X-Exhibit-Inference-Channel': channel };
    const st = await stateFor(id, channel, headers);
    const usable = st.enabled && (st.owner || st.approved);
    const notAllowed = st.enabled && !st.owner && !st.approved;
    const OWNER_NOTE = 'This tool uses AI, but its owner has not allowed that. Only the owner can turn it on.';
    if (d.op === 'available') {
      // A tool that checks first never calls, so this is where a recipient
      // learns why its AI controls are off — in Exhibit's chrome, once.
      if (notAllowed && (st.complete || st.decide || st.reason === 'no_key')) showBanner('owner', OWNER_NOTE);
      reply({ ok: true, value: {
        complete: Boolean(usable && st.complete),
        decide: Boolean(usable && st.decide),
        images: Boolean(usable && st.complete && st.images),
      } });
      return;
    }
    const fail = (code, message) => reply({ ok: false, error: { code, message: message || REFUSED[code] || 'The AI request failed.' } });
    if (!st.enabled) { fail('unavailable'); return; }
    // Only the owner can change this, so it is said before anything the
    // viewer could fix, such as adding a key.
    if (notAllowed) {
      showBanner('owner', OWNER_NOTE);
      fail('not_approved');
      return;
    }
    if (!st.complete && !st.decide && st.reason === 'no_key') {
      showBanner('no_key', 'This tool tried to use AI, but you have not added an AI key yet.', true);
      fail('no_key', 'Add your own AI key to use AI in artifacts.');
      return;
    }
    if (!st.consented) {
      if (declined.has(channel)) { fail('not_approved', 'AI use was not allowed.'); return; }
      if (!(await ask(id, channel, headers, st, frame))) { fail('not_approved', 'AI use was not allowed.'); return; }
    }
    let payload = d.payload ?? null;
    if (d.op === 'complete') {
      const checked = prepareCompletePayload(payload);
      if (!checked.ok) { fail('invalid_request', checked.message); return; }
      payload = checked.payload;
    }
    let requestBody;
    try {
      requestBody = JSON.stringify({ kind: d.op, payload });
    } catch {
      fail('invalid_request', 'This AI request could not be read.');
      return;
    }
    const res = await viewerFetch('/api/artifacts/' + encodeURIComponent(id) + '/inference', {
      method: 'POST', headers, body: requestBody,
    }).catch(() => null);
    if (!res) { fail('provider_error', 'The AI request could not be sent.'); return; }
    // The body limit answers before the inference handler, in its own shape.
    // Over it, the request was too large to send at all: the artifact can fix
    // that, for instance by sending smaller images (Exh-v81w).
    if (res.status === 413) {
      fail('invalid_request', 'This AI request is too large for this Exhibit. Send less with it, such as fewer or smaller images.');
      return;
    }
    const body = await res.json().catch(() => ({}));
    if (res.ok) { reply({ ok: true, value: body.value }); return; }
    const err = body.error && typeof body.error === 'object' ? body.error : { code: 'provider_error' };
    if (err.code === 'limit_reached') showBanner('limit', err.message || 'The AI allowance is used up for this month.');
    if (err.code === 'no_key') showBanner('no_key', 'This tool tried to use AI, but you have not added an AI key yet.', true);
    if (err.code === 'not_approved') stateFor(id, channel, headers, true); // a decision changed elsewhere
    fail(err.code || 'provider_error', err.message);
  }

  function onMessage(e) {
    const d = e.data;
    if (d?.__avInference !== true || e.ports.length !== 1) return;
    const port = e.ports[0];
    const frame = frameFor(e.source);
    const id = artifactId();
    const channel = frame?.dataset.artifactChannel;
    // A replacement document shares the frame's window. The channel is
    // inlined only in the authorized artifact render for this viewer.
    if (!id || d.artifactId !== id || !channel || d.channel !== channel ||
        (d.op !== 'available' && d.op !== 'complete' && d.op !== 'decide')) {
      port.close();
      return;
    }
    // Reply through the requesting document's port, even if the frame has
    // since navigated. Only the readiness announcement uses the window.
    handle(frame, d, result => port.postMessage(result)).finally(() => port.close());
  }
  window.addEventListener('message', onMessage);
  // The frame holds its calls until this says it is listening, so a call made
  // before this module ran is not lost. Announced to every AI frame now, and
  // again on each frame load (a reload, or a version preview htmx inserts
  // later); the frame acts on the first and ignores the rest. targetOrigin is
  // '*' because the frame's origin is opaque; the frame checks it came from
  // the app origin.
  const announce = frame => frame.contentWindow?.postMessage({ __avAIHostReady: true }, '*');
  const onFrameLoad = e => { if (e.target?.tagName === 'IFRAME' && e.target.hasAttribute?.('data-ai-frame')) announce(e.target); };
  document.querySelectorAll('iframe[data-ai-frame]').forEach(announce);
  document.addEventListener('load', onFrameLoad, true);
  // A decision made on another page (Edit → Permissions, Settings) must not
  // be outlived by this page's cached copy.
  const refresh = () => states.clear();
  document.addEventListener('visibilitychange', refresh);
  document.body.addEventListener('exhibit:capabilities-changed', refresh);
  return () => {
    window.removeEventListener('message', onMessage);
    document.removeEventListener('load', onFrameLoad, true);
    document.removeEventListener('visibilitychange', refresh);
    document.body.removeEventListener('exhibit:capabilities-changed', refresh);
  };
}
