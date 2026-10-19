import { apiFetch } from './api-module.js';

const messages = document.getElementById('messages');
const loadError = document.getElementById('question-load-error');
const loads = new Map();
const failedLoads = new Map();
const submissions = new Set();

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
      form.closest('[data-request-id]').dataset.pending = 'false';
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
    selected: [...fieldset.querySelectorAll('[data-question-choice][aria-pressed="true"]')].map(button => button.value),
    text: fieldset.querySelector('textarea').value.trim()
  }));
}

async function submitResponse(sessionId, requestId, responseBody) {
  const key = sessionId + '/' + requestId;
  if (submissions.has(key)) return false;
  submissions.add(key);
  const card = document.getElementById('agent-question-' + requestId);
  const status = card?.querySelector('.question-status');
  const showStatus = text => { if (status) status.textContent = text; };
  const fields = [...(card?.querySelectorAll('input, textarea, button') || [])];
  fields.forEach(field => { field.disabled = true; });
  showStatus('Sending…');
  const retry = card?.querySelector('[data-question-resend]');
  if (retry) retry.hidden = true;
  let failureMessage = 'Could not send your answers. Please try again.';
  try {
    const response = await apiFetch(requestPath(sessionId, requestId), {
      method: 'POST', body: JSON.stringify(responseBody)
    });
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      if (response.status === 409 || response.status === 404) {
        showStatus('These questions are no longer waiting for an answer.');
        document.dispatchEvent(new CustomEvent('exhibit:question-resolved', { detail: { sessionId, requestId } }));
        if (response.status === 409) await refreshQuestions(sessionId, requestId, false);
        return false;
      }
      failureMessage = data.error || failureMessage;
      throw new Error(failureMessage);
    }
    // Also refresh after the POST: the form still settles if its SSE response
    // was lost. The server returns the canonical answers to both tabs.
    showStatus(responseBody.cancelled ? 'Questions skipped.' : 'Answers sent.');
    document.dispatchEvent(new CustomEvent('exhibit:question-resolved', { detail: { sessionId, requestId } }));
    await refreshQuestions(sessionId, requestId, false);
    return true;
  } catch {
    showStatus(failureMessage);
    if (card?.dataset.pending === 'true') {
      fields.forEach(field => { field.disabled = false; });
      if (retry && !responseBody.message && !responseBody.cancelled) retry.hidden = false;
    }
    return false;
  } finally {
    submissions.delete(key);
  }
}

function sendForm(form, cancelled = false) {
  const card = form.closest('[data-request-id]');
  return submitResponse(card.dataset.sessionId, card.dataset.requestId, {
    answers: cancelled ? [] : collectAnswers(form), cancelled
  });
}

function sendWhenReady(form) {
  const unanswered = [...form.querySelectorAll('[data-question-id]')].find(question => question.dataset.answerReady !== 'true');
  if (unanswered) {
    unanswered.querySelector('button, textarea')?.focus();
    return;
  }
  return sendForm(form);
}

function finishQuestion(question) {
  const form = question.closest('[data-question-form]');
  const answer = collectAnswers(form).find(answer => answer.id === question.dataset.questionId);
  if (!answer.selected.length && !answer.text) {
    form.querySelector('.question-status').textContent = 'Choose an option or write an answer.';
    question.querySelector('textarea').focus();
    return;
  }
  question.dataset.answerReady = 'true';
  return sendWhenReady(form);
}

// The ordinary chat composer can answer or redirect the entire request. Keep
// its draft until the same owner-scoped answer endpoint accepts the message.
document.addEventListener('exhibit:question-message', event => {
  const { sessionId, requestId, message } = event.detail;
  event.detail.response = submitResponse(sessionId, requestId, { message });
});

messages.addEventListener('submit', event => {
  const form = event.target.closest('[data-question-form]');
  if (!form) return;
  event.preventDefault();
  return sendWhenReady(form);
});

messages.addEventListener('input', event => {
  const form = event.target.closest('[data-question-form]');
  if (form) form.querySelector('.question-status').textContent = '';
  const question = event.target.closest('[data-question-id]');
  if (event.target.tagName === 'TEXTAREA' && question) {
    question.dataset.answerReady = 'false';
    if (question.dataset.multiSelect === 'false') {
      question.querySelectorAll('[data-question-choice]').forEach(button => button.setAttribute('aria-pressed', 'false'));
    }
  }
});

messages.addEventListener('keydown', event => {
  if (event.target.tagName !== 'TEXTAREA' || event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
  const question = event.target.closest('[data-question-id]');
  if (!question || event.target.disabled) return;
  event.preventDefault();
  return finishQuestion(question);
});

messages.addEventListener('toggle', event => {
  if (event.target.matches('.question-other') && event.target.open) event.target.querySelector('textarea').focus();
}, true);

messages.addEventListener('click', event => {
  const choice = event.target.closest('[data-question-choice]');
  if (!choice || choice.disabled) return;
  const question = choice.closest('[data-question-id]');
  const form = question.closest('[data-question-form]');
  form.querySelector('.question-status').textContent = '';
  if (question.dataset.multiSelect === 'true') {
    choice.setAttribute('aria-pressed', String(choice.getAttribute('aria-pressed') !== 'true'));
    question.dataset.answerReady = 'false';
    return;
  }
  question.querySelectorAll('[data-question-choice]').forEach(button => button.setAttribute('aria-pressed', String(button === choice)));
  question.querySelector('textarea').value = '';
  question.querySelector('details').open = false;
  question.dataset.answerReady = 'true';
  return sendWhenReady(form);
});

messages.addEventListener('click', event => {
  const send = event.target.closest('[data-question-answer]');
  if (send && !send.disabled) return finishQuestion(send.closest('[data-question-id]'));
  const retry = event.target.closest('[data-question-resend]');
  if (retry && !retry.disabled) return sendWhenReady(retry.closest('[data-question-form]'));
  const button = event.target.closest('[data-question-skip]');
  if (!button || button.disabled) return;
  return sendForm(button.closest('[data-question-form]'), true);
});
