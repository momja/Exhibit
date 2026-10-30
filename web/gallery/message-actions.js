// Message actions share one delegated listener, including history swapped by
// htmx and messages moved into the live chat when a conversation is continued.
// Add a handler here and a data-message-action button in agentMessage to extend.
const actions = {
  copy: {
    failureMessage: 'Could not copy. Select the text and copy manually.',
    async run({ content }) {
      const text = content.querySelector('[data-message-source]')?.dataset.messageSource ?? content.textContent;
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
      } else {
        // Self-hosted HTTP origins may lack the secure-context Clipboard API.
        // Keep the fallback synchronous so it retains the user's click gesture.
        const field = document.createElement('textarea');
        field.value = text;
        field.setAttribute('readonly', '');
        field.style.position = 'fixed';
        field.style.opacity = '0';
        const focused = document.activeElement;
        const selection = window.getSelection();
        const ranges = Array.from({ length: selection.rangeCount }, (_, i) => selection.getRangeAt(i));
        document.body.appendChild(field);
        try {
          field.select();
          if (!document.execCommand('copy')) throw new Error('Copy unavailable');
        } finally {
          field.remove();
          focused?.focus({ preventScroll: true });
          selection.removeAllRanges();
          ranges.forEach(range => selection.addRange(range));
        }
      }
      return 'Copied';
    }
  }
};

const statusTimers = new WeakMap();
document.getElementById('pane-chat').addEventListener('click', async event => {
  const button = event.target.closest('[data-message-action]');
  if (!button || button.disabled) return;
  if (!Object.hasOwn(actions, button.dataset.messageAction)) return;
  const action = actions[button.dataset.messageAction];
  const message = button.closest('[data-agent-message]');
  if (!message) return;
  const content = message.querySelector('[data-message-content]');
  const status = message.querySelector('.message-action-status');
  clearTimeout(statusTimers.get(status));
  status.textContent = '';
  status.removeAttribute('data-error');
  button.disabled = true;
  try {
    status.textContent = await action.run({ content, message, button });
    statusTimers.set(status, setTimeout(() => { status.textContent = ''; }, 2500));
  } catch {
    status.textContent = action.failureMessage;
    status.setAttribute('data-error', '');
  } finally {
    button.disabled = false;
  }
});
