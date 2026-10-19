import { apiFetch } from './api-module.js';
import { downloadBlob } from './download.js';

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
    downloadBlob(new Blob(
      [JSON.stringify(state, null, 2) + '\n'], { type: 'application/json' }
    ), ID + '-localStorage.json');
    status.textContent = '';
  } catch (error) {
    status.textContent = 'Could not export state: ' + error.message;
    status.classList.add('error');
  } finally {
    exportButton.disabled = false;
  }
});
