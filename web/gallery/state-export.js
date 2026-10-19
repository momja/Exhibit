import { apiFetch } from './api-module.js';

const exportButton = document.getElementById('state-export');
const status = document.getElementById('state-export-status');

exportButton?.addEventListener('click', async () => {
  if (exportButton.disabled) return;
  exportButton.disabled = true;
  status.textContent = 'Exporting…';
  status.classList.remove('error');
  try {
    // Fetch saved strings afresh: the inspector may contain unsaved edits,
    // and another tab or device may have changed state since it was opened.
    const response = await apiFetch(window.ExhibitState.url(ID));
    if (!response.ok) throw new Error('HTTP ' + response.status);
    const state = await response.json();
    const url = URL.createObjectURL(new Blob(
      [JSON.stringify(state, null, 2) + '\n'], { type: 'application/json' }
    ));
    const link = document.createElement('a');
    link.href = url;
    link.download = ID + '-localStorage.json';
    document.body.appendChild(link);
    link.click();
    link.remove();
    // Give the browser time to start the download before releasing its bytes.
    setTimeout(() => URL.revokeObjectURL(url), 10000);
    status.textContent = '';
  } catch (error) {
    status.textContent = 'Could not export state: ' + error.message;
    status.classList.add('error');
  } finally {
    exportButton.disabled = false;
  }
});
