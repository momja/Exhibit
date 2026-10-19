import { apiFetch } from './api-module.js';

const messages = document.getElementById('messages');
const loadError = document.getElementById('question-load-error');
const loads = new Map();
const failedLoads = new Map();

function requestPath(sessionId, requestId) {
  return '/api/agent/sessions/' + encodeURIComponent(sessionId) + '/questions/' + encodeURIComponent(requestId);
}

// htmx owns the markup and the swap. Its fetch hook uses the same credential
// client as other API calls, including single-user instances with a token.
function refreshQuestions(sessionId, requestId, pending) {
  const key = sessionId + '/' + requestId;
  const task = (loads.get(key) || Promise.resolve()).then(async () => {
    const existing = document.getElementById('agent-question-' + requestId);
    // SSE reconnect replays the backlog. Keep an unfinished draft intact.
    if (pending && existing) return;
    let failed = false;
    await htmx.ajax('GET', requestPath(sessionId, requestId), {
      target: existing || messages,
      swap: existing ? 'outerHTML' : 'beforeend',
      async fetch(path, options) {
        try {
          const response = await apiFetch(path, options);
          if (!response.ok) throw new Error('Questions unavailable');
          failedLoads.delete(key);
          return response;
        } catch (error) {
          failed = true;
          failedLoads.set(key, { sessionId, requestId, pending });
          loadError.hidden = false;
          throw error;
        }
      }
    });
    if (!failed) {
      loadError.hidden = failedLoads.size === 0;
      messages.scrollTop = messages.scrollHeight;
    }
  }).catch(() => {
    failedLoads.set(key, { sessionId, requestId, pending });
    loadError.hidden = false;
  }).finally(() => { if (loads.get(key) === task) loads.delete(key); });
  loads.set(key, task);
  return task;
}

document.addEventListener('exhibit:agent-event', event => {
  const { event: agentEvent, sessionId } = event.detail;
  if (agentEvent.type === 'exhibit_questions' && sessionId) {
    return refreshQuestions(sessionId, agentEvent.requestId, agentEvent.pending);
  }
  if (agentEvent.type === 'exhibit_session_closed') {
    messages.querySelectorAll('[data-question-form]').forEach(form => {
      form.querySelectorAll('input, textarea, button').forEach(field => { field.disabled = true; });
      form.querySelector('.question-status').textContent = 'This conversation has ended.';
    });
    failedLoads.clear();
    loadError.hidden = true;
  }
});

document.getElementById('question-retry').addEventListener('click', () => {
  return Promise.all([...failedLoads.values()].map(item => refreshQuestions(item.sessionId, item.requestId, item.pending)));
});

export function collectAnswers(form) {
  return [...form.querySelectorAll('[data-question-id]')].map(fieldset => ({
    id: fieldset.dataset.questionId,
    selected: [...fieldset.querySelectorAll('input:checked')].map(input => input.value),
    text: fieldset.querySelector('textarea').value.trim()
  }));
}

async function submitAnswers(form, cancelled) {
  if (form.dataset.submitting === 'true') return;
  const card = form.closest('[data-request-id]');
  const status = form.querySelector('.question-status');
  const answers = cancelled ? [] : collectAnswers(form);
  if (!cancelled && answers.some(answer => answer.selected.length === 0 && !answer.text)) {
    status.textContent = 'Answer each question or skip.';
    return;
  }
  form.dataset.submitting = 'true';
  const fields = [...form.querySelectorAll('input, textarea, button')];
  fields.forEach(field => { field.disabled = true; });
  status.textContent = 'Sending…';
  let failureMessage = 'Could not send your answers. Please try again.';
  try {
    const response = await apiFetch(requestPath(card.dataset.sessionId, card.dataset.requestId), {
      method: 'POST', body: JSON.stringify({ answers, cancelled })
    });
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      if (response.status === 409 || response.status === 404) {
        status.textContent = 'These questions are no longer waiting for an answer.';
        if (response.status === 409) await refreshQuestions(card.dataset.sessionId, card.dataset.requestId, false);
        return;
      }
      failureMessage = data.error || failureMessage;
      throw new Error(failureMessage);
    }
    // Also refresh after the POST: the form still settles if its SSE response
    // was lost. The server returns the canonical answers to both tabs.
    status.textContent = cancelled ? 'Questions skipped.' : 'Answers sent.';
    await refreshQuestions(card.dataset.sessionId, card.dataset.requestId, false);
  } catch {
    status.textContent = failureMessage;
    fields.forEach(field => { field.disabled = false; });
  } finally {
    form.dataset.submitting = 'false';
  }
}

messages.addEventListener('submit', event => {
  const form = event.target.closest('[data-question-form]');
  if (!form) return;
  event.preventDefault();
  return submitAnswers(form, false);
});

messages.addEventListener('input', event => {
  const form = event.target.closest('[data-question-form]');
  if (form && form.dataset.submitting !== 'true') form.querySelector('.question-status').textContent = '';
  const question = event.target.closest('[data-question-id]');
  if (event.target.tagName === 'TEXTAREA' && event.target.value.trim() && question?.dataset.multiSelect === 'false') {
    question.querySelectorAll('input[type="radio"]').forEach(input => { input.checked = false; });
  }
});

messages.addEventListener('change', event => {
  if (event.target.type !== 'radio') return;
  const question = event.target.closest('[data-question-id]');
  if (question) {
    question.querySelector('textarea').value = '';
    question.querySelector('details').open = false;
  }
});

messages.addEventListener('click', event => {
  const button = event.target.closest('[data-question-skip]');
  if (!button) return;
  return submitAnswers(button.closest('[data-question-form]'), true);
});
