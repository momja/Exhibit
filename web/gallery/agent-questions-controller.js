import { Controller } from '../stimulus/stimulus.js';
import { respondToInteraction } from './agent-interactions.js';

export function collectAnswers(form) {
  return [...form.querySelectorAll('[data-question-id]')].map(fieldset => ({
    id: fieldset.dataset.questionId,
    selected: [...fieldset.querySelectorAll('[data-question-choice][aria-pressed="true"]')].map(button => button.value),
    text: fieldset.querySelector('textarea').value.trim()
  }));
}

export default class extends Controller {
  static targets = ['progress', 'status', 'skipConfirmation', 'skipTrigger'];

  connect() { this.updateProgress(); }

  choose(event) {
    const choice = event.currentTarget;
    const question = choice.closest('[data-question-id]');
    const multi = question.dataset.multiSelect === 'true';
    const selected = choice.getAttribute('aria-pressed') !== 'true';
    if (!multi) {
      question.querySelectorAll('[data-question-choice]').forEach(button => button.setAttribute('aria-pressed', 'false'));
      question.querySelector('textarea').value = '';
      question.querySelector('details').open = false;
    }
    choice.setAttribute('aria-pressed', String(selected));
    this.edited();
  }

  edit(event) {
    const question = event.target.closest('[data-question-id]');
    if (question.dataset.multiSelect !== 'true') {
      question.querySelectorAll('[data-question-choice]').forEach(button => button.setAttribute('aria-pressed', 'false'));
    }
    this.edited();
  }

  edited() {
    this.statusTarget.textContent = '';
    this.element.querySelectorAll('[aria-invalid]').forEach(field => field.removeAttribute('aria-invalid'));
    this.keepAnswering(false);
    this.updateProgress();
  }

  updateProgress() {
    const answers = collectAnswers(this.element);
    const answered = answers.filter(answer => answer.selected.length || answer.text).length;
    this.progressTarget.textContent = `${answered} of ${answers.length} answered`;
  }

  send(event) {
    event.preventDefault();
    const answers = collectAnswers(this.element);
    const index = answers.findIndex(answer => !answer.selected.length && !answer.text);
    if (index !== -1) {
      this.statusTarget.textContent = 'Answer each question before sending, or skip this set.';
      const field = this.element.querySelectorAll('[data-question-id]')[index];
      field.setAttribute('aria-invalid', 'true');
      field.querySelector('button, textarea').focus();
      return;
    }
    return this.respond({ action: 'answer', answers });
  }

  keyDown(event) {
    // Plain Enter writes a newline. The explicit shortcut sends the whole set.
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.isComposing) return this.send(event);
  }

  skip() {
    if (collectAnswers(this.element).some(answer => answer.selected.length || answer.text)) {
      this.skipConfirmationTarget.hidden = false;
      this.skipTriggerTarget.hidden = true;
      this.skipConfirmationTarget.querySelector('button').focus();
      return;
    }
    return this.confirmSkip();
  }

  confirmSkip() { return this.respond({ action: 'skip' }); }

  keepAnswering(focus = true) {
    this.skipConfirmationTarget.hidden = true;
    this.skipTriggerTarget.hidden = false;
    if (focus) this.skipTriggerTarget.focus();
  }

  focusOther(event) {
    if (event.target.open) event.target.querySelector('textarea').focus();
  }

  respond(body) {
    const card = this.element.closest('[data-interaction-id]');
    return respondToInteraction(card.dataset.sessionId, card.dataset.interactionId, body);
  }
}

// Chat redirection is a question-specific action, never a generic approval.
document.addEventListener('exhibit:question-message', event => {
  const { sessionId, requestId, message, images, snippets } = event.detail;
  const card = document.getElementById('agent-interaction-' + requestId);
  if (card?.dataset.interactionKind !== 'questions') return;
  event.detail.response = respondToInteraction(sessionId, requestId, { action: 'message', message, images, snippets });
});
