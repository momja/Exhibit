import { apiFetch, viewerFetch } from './api-module.js';

/* Settings → AI in artifacts (Exh-zue3). Stop withdraws a tool's permission
 * to use AI at this person's expense: for their own tool it clears the
 * tool's AI approval; for a tool shared with them it withdraws their own
 * consent, leaving the owner's decision alone. The list is server-rendered;
 * this script only makes the change and asks for the region again. */
const status = () => document.getElementById('ai-permissions-status');

document.addEventListener('click', async event => {
  const button = event.target.closest('[data-ai-revoke]');
  if (!button) return;
  const id = encodeURIComponent(button.dataset.aiRevoke);
  const headers = { 'X-Exhibit-Inference-Channel': button.dataset.artifactChannel };
  button.disabled = true;
  const res = await (button.dataset.aiOwned === 'true'
    ? apiFetch('/api/artifacts/' + id, { method: 'PATCH', headers, body: JSON.stringify({ inference_approved: false }) })
    : viewerFetch('/api/artifacts/' + id + '/inference/consent', { method: 'DELETE', headers })).catch(() => null);
  const el = status();
  if (!res || !res.ok) {
    button.disabled = false;
    if (el) { el.hidden = false; el.textContent = '✗ Could not stop that tool using AI. Try again.'; }
    return;
  }
  if (el) { el.hidden = false; el.textContent = 'Stopped. The tool will ask again if it needs AI.'; }
  document.body.dispatchEvent(new CustomEvent('exhibit:ai-permissions-changed', { bubbles: true }));
});
