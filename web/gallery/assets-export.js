import { apiFetch } from './api-module.js';
import { downloadBlob } from './download.js';

const exportButton = document.getElementById('assets-export');
const status = document.getElementById('assets-export-status');

exportButton?.addEventListener('click', async () => {
  if (exportButton.disabled) return;
  exportButton.disabled = true;
  status.textContent = 'Exporting…';
  status.classList.remove('error');
  try {
    const response = await apiFetch('/api/artifacts/' + encodeURIComponent(ID) + '/assets/export');
    if (!response.ok) throw new Error('HTTP ' + response.status);
    downloadBlob(await response.blob(), ID + '-assets.zip');
    status.textContent = '';
  } catch (error) {
    status.textContent = 'Could not export assets: ' + error.message;
    status.classList.add('error');
  } finally {
    exportButton.disabled = false;
  }
});
