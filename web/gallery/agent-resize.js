// The divider stays outside the htmx preview swap, so resizing never reloads
// the artifact or interrupts the conversation.
const STORAGE_KEY = 'exhibit.agentChatWidth';
const DEFAULT_WIDTH = 460;
const MIN_PANE_WIDTH = 280;
const layout = document.getElementById('agent-layout');
const chat = document.getElementById('pane-chat');
const divider = document.getElementById('agent-pane-resizer');
const mobile = window.matchMedia('(max-width: 640px)');
let preferredWidth = DEFAULT_WIDTH;
let drag = null;

try {
  const saved = Number(localStorage.getItem(STORAGE_KEY));
  if (Number.isFinite(saved) && saved >= MIN_PANE_WIDTH) preferredWidth = saved;
} catch { /* Storage can be unavailable; resizing still works. */ }

function maximumWidth() {
  return Math.max(MIN_PANE_WIDTH, layout.clientWidth - MIN_PANE_WIDTH);
}

function applyWidth() {
  if (mobile.matches) return;
  const width = Math.round(Math.max(MIN_PANE_WIDTH, Math.min(preferredWidth, maximumWidth())));
  layout.style.setProperty('--agent-chat-width', width + 'px');
  divider.setAttribute('aria-valuemax', String(maximumWidth()));
  divider.setAttribute('aria-valuenow', String(width));
  divider.setAttribute('aria-valuetext', width + ' pixels');
}

function rememberWidth() {
  try { localStorage.setItem(STORAGE_KEY, String(preferredWidth)); } catch { /* Optional preference. */ }
}

function resizeTo(width) {
  preferredWidth = Math.max(MIN_PANE_WIDTH, Math.min(width, maximumWidth()));
  applyWidth();
}

function finishDrag(event) {
  if (!drag || (event && event.pointerId !== drag.pointerId)) return;
  const pointerId = drag.pointerId;
  drag = null;
  layout.classList.remove('is-resizing');
  if (divider.hasPointerCapture(pointerId)) divider.releasePointerCapture(pointerId);
  rememberWidth();
}

divider.addEventListener('pointerdown', (event) => {
  if (mobile.matches || drag || event.button !== 0 || !event.isPrimary) return;
  event.preventDefault();
  divider.focus();
  divider.setPointerCapture(event.pointerId);
  drag = { pointerId: event.pointerId, x: event.clientX, width: chat.getBoundingClientRect().width };
  layout.classList.add('is-resizing');
});

divider.addEventListener('pointermove', (event) => {
  if (!drag || event.pointerId !== drag.pointerId) return;
  resizeTo(drag.width + event.clientX - drag.x);
});

for (const type of ['pointerup', 'pointercancel', 'lostpointercapture']) {
  divider.addEventListener(type, finishDrag);
}
window.addEventListener('blur', () => finishDrag());

divider.addEventListener('keydown', (event) => {
  if (mobile.matches) return;
  const step = event.shiftKey ? 64 : 16;
  let width = chat.getBoundingClientRect().width;
  switch (event.key) {
    case 'ArrowLeft': width -= step; break;
    case 'ArrowRight': width += step; break;
    case 'Home': width = MIN_PANE_WIDTH; break;
    case 'End': width = maximumWidth(); break;
    default: return;
  }
  event.preventDefault();
  resizeTo(width);
  rememberWidth();
});

divider.addEventListener('dblclick', () => {
  if (mobile.matches) return;
  preferredWidth = DEFAULT_WIDTH;
  applyWidth();
  rememberWidth();
});

// Clamp the displayed width when the window shrinks without losing the user's
// preferred width when they make it larger again.
new ResizeObserver(applyWidth).observe(layout);
mobile.addEventListener('change', () => {
  finishDrag();
  applyWidth();
});
applyWidth();
