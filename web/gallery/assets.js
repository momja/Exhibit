import { apiFetch } from './api-module.js';

// Delegation keeps delete controls working after imports swap in fresh rows
// from the server-rendered asset panel partial.
document.getElementById('assets-panel')?.addEventListener('click', async event => {
  const button = event.target.closest('[data-delete-asset]');
  if (!button || button.disabled) return;
  if (!window.confirm(
    'Delete ' + button.dataset.sourceUrl + '?\n\n' +
    'This cannot be undone. If the artifact still loads that file, it will stop working.'
  )) return;
  const status = document.getElementById('assets-status');
  button.disabled = true;
  status.textContent = 'Deleting…';
  status.classList.remove('error');
  try {
    const response = await apiFetch('/api/artifacts/' + encodeURIComponent(ID) + '/assets/' + encodeURIComponent(button.dataset.deleteAsset), { method: 'DELETE' });
    if (!response.ok) throw new Error('HTTP ' + response.status);
    status.textContent = 'Deleted.';
    document.body.dispatchEvent(new Event('exhibit:assets-changed'));
  } catch (error) {
    button.disabled = false;
    status.textContent = 'Could not delete: ' + error.message;
    status.classList.add('error');
  }
});
