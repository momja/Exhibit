import { apiFetch } from './api-module.js';

// Delivery and lifecycle belong here. Individual interaction controllers own
// their draft inputs; the server owns all durable state and rendered markup.
const messages = document.getElementById('messages');
const loadError = document.getElementById('interaction-load-error');
const loads = new Map();
const failedLoads = new Map();
const submissions = new Set();
const focusAfterResponse = new Set();
const controllers = new Set();
let activeSessionId;
let sessionGeneration = 0;

function isCurrent(sessionId, generation) {
  return sessionId === activeSessionId && generation === sessionGeneration;
}

function interactionCard(requestId) {
  return document.getElementById('agent-interaction-' + requestId);
}

function changeSession(sessionId) {
  if (sessionId === activeSessionId) return;
  const previous = activeSessionId;
  activeSessionId = sessionId;
  sessionGeneration++;
  controllers.forEach(controller => controller.abort());
  controllers.clear();
  loads.clear();
  failedLoads.clear();
  focusAfterResponse.clear();
  if (loadError) loadError.hidden = true;
  if (previous) messages.querySelectorAll('[data-interaction-id]').forEach(card => {
    if (card.dataset.sessionId !== previous || card.dataset.pending !== 'true') return;
    card.dataset.pending = 'false';
    card.querySelectorAll('input, textarea, button').forEach(field => { field.disabled = true; });
    card.querySelector('.interaction-status').textContent = 'This conversation has ended.';
  });
}

document.addEventListener('exhibit:agent-session-changed', event => changeSession(event.detail.sessionId));
document.addEventListener('htmx:before:swap', event => {
  const request = event.detail?.ctx?.interactionRequest;
  if (request && !isCurrent(request.sessionId, request.generation)) event.preventDefault();
});

function requestPath(sessionId, requestId) {
  return '/api/agent/sessions/' + encodeURIComponent(sessionId) + '/interactions/' + encodeURIComponent(requestId);
}

export function refreshInteraction(sessionId, requestId, revision = 0) {
  const generation = sessionGeneration;
  if (!isCurrent(sessionId, generation)) return Promise.resolve();
  const key = sessionId + '/' + requestId;
  const task = (loads.get(key) || Promise.resolve()).then(async () => {
    if (!isCurrent(sessionId, generation)) return;
    const existing = interactionCard(requestId);
    // A replay must not replace a draft or revive a resolved form. Revision 0
    // forces reconciliation after POST, including when its SSE event is lost.
    if (revision && existing && Number(existing.dataset.revision) >= revision) return;
    const restoreFocus = existing?.contains(document.activeElement) || focusAfterResponse.has(key);
    const controller = new AbortController();
    controllers.add(controller);
    try {
      await htmx.ajax('GET', requestPath(sessionId, requestId), {
        source: messages,
        interactionRequest: { sessionId, generation },
        target: existing || messages,
        swap: existing ? 'outerHTML' : 'beforeend',
        async fetch(path, options) {
          const response = await apiFetch(path, { ...options, signal: controller.signal });
          if (!isCurrent(sessionId, generation)) throw new Error('Conversation ended');
          if (!response.ok) throw new Error('Interaction unavailable');
          return response;
        }
      });
      if (!isCurrent(sessionId, generation)) return;
      failedLoads.delete(key);
      if (loadError) loadError.hidden = failedLoads.size === 0;
      const updated = interactionCard(requestId);
      if (!existing) updated?.scrollIntoView({ block: 'start' });
      if (updated?.dataset.pending !== 'true') {
        if (restoreFocus && (!document.activeElement || document.activeElement === document.body || existing?.contains(document.activeElement))) {
          updated?.focus({ preventScroll: true });
        }
        focusAfterResponse.delete(key);
      }
    } finally {
      controllers.delete(controller);
    }
  }).catch(() => {
    if (isCurrent(sessionId, generation)) {
      failedLoads.set(key, { sessionId, requestId });
      if (loadError) loadError.hidden = false;
    }
  }).finally(() => { if (loads.get(key) === task) loads.delete(key); });
  loads.set(key, task);
  return task;
}

document.addEventListener('exhibit:agent-event', event => {
  const { event: agentEvent, sessionId } = event.detail;
  if (activeSessionId === undefined && sessionId) changeSession(sessionId);
  if (sessionId !== activeSessionId) return;
  if (agentEvent.type === 'exhibit_interaction' && sessionId) {
    return refreshInteraction(sessionId, agentEvent.requestId, agentEvent.revision);
  }
  if (agentEvent.type === 'exhibit_session_closed') changeSession(null);
});

document.getElementById('interaction-retry')?.addEventListener('click', () => {
  return Promise.all([...failedLoads.values()].map(item => refreshInteraction(item.sessionId, item.requestId)));
});

// Every type uses the same owner-authenticated action path. Only a type's
// controller decides which typed action to submit; free text is not approval.
export async function respondToInteraction(sessionId, requestId, responseBody) {
  const generation = sessionGeneration;
  if (!isCurrent(sessionId, generation)) return false;
  const key = sessionId + '/' + requestId;
  const card = interactionCard(requestId);
  if (!card || card.dataset.pending !== 'true' || submissions.has(key)) return false;
  submissions.add(key);
  const focusedField = document.activeElement;
  if (card.contains(focusedField)) focusAfterResponse.add(key);
  const status = card.querySelector('.interaction-status');
  const fields = [...card.querySelectorAll('input, textarea, button')];
  const enabledFields = fields.filter(field => !field.disabled);
  fields.forEach(field => { field.disabled = true; });
  card.setAttribute('aria-busy', 'true');
  status.textContent = 'Sending…';
  let failureMessage = 'Could not send. Please try again.';
  try {
    const response = await apiFetch(requestPath(sessionId, requestId), {
      method: 'POST', body: JSON.stringify({ ...responseBody, revision: Number(card.dataset.revision) })
    });
    if (!isCurrent(sessionId, generation)) return false;
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      if (!isCurrent(sessionId, generation)) return false;
      if (response.status === 409 || response.status === 404) {
        card.dataset.pending = 'false';
        status.textContent = 'This request is no longer waiting for a response.';
        resolved(sessionId, requestId);
        if (response.status === 409) await refreshInteraction(sessionId, requestId);
        return false;
      }
      failureMessage = data.error || failureMessage;
      throw new Error(failureMessage);
    }
    card.dataset.pending = 'false';
    status.textContent = 'Response sent.';
    resolved(sessionId, requestId);
    await refreshInteraction(sessionId, requestId);
    return true;
  } catch (error) {
    if (!isCurrent(sessionId, generation)) return false;
    status.textContent = failureMessage;
    focusAfterResponse.delete(key);
    if (card.dataset.pending === 'true') {
      enabledFields.forEach(field => { field.disabled = false; });
      if (focusedField?.isConnected && card.contains(focusedField) &&
          (!document.activeElement || document.activeElement === document.body || card.contains(document.activeElement))) {
        focusedField.focus();
      }
    }
    return false;
  } finally {
    submissions.delete(key);
    card.removeAttribute('aria-busy');
  }
}

function resolved(sessionId, requestId) {
  document.dispatchEvent(new CustomEvent('exhibit:interaction-resolved', { detail: { sessionId, requestId } }));
}

