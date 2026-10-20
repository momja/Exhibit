import { apiFetch } from './api-module.js';

// Validate and check for existing data before prompting. Cancellation has
// changed nothing; a concurrent first write is checked in the transaction.
for (const kind of ['state', 'assets']) {
  const button = document.getElementById(kind + '-import');
  const input = document.getElementById(kind + '-import-file');
  const status = document.getElementById(kind + '-import-status');
  if (!button || !input) continue;
  button.addEventListener('click', () => { if (!button.disabled) input.click(); });
  input.addEventListener('change', async () => {
    const file = input.files?.[0];
    if (!file || button.disabled) return;
    button.disabled = true;
    input.disabled = true;
    status.textContent = 'Importing…';
    status.classList.remove('error');
    try {
      const url = '/api/artifacts/' + encodeURIComponent(ID) + '/' + kind + '/import';
      const options = { method: 'POST', body: file, headers: { 'Content-Type': kind === 'state' ? 'application/json' : 'application/zip' } };
      let response = await apiFetch(url, options);
      let failure = response.ok ? null : await response.json().catch(() => null);
      if (response.status === 409 && failure?.confirmation_required) {
        const description = kind === 'state' ? 'stored state' : 'stored assets';
        if (!window.confirm(
          'Import ' + description + '?\n\n' +
          'This will erase all existing ' + description + ' for this artifact and replace it with the file’s data.' +
          (kind === 'state' ? ' Unsaved edits in the state inspector will also be discarded.' : '') +
          '\n\nContinue only if you understand that the existing data will be erased.'
        )) {
          status.textContent = 'Import cancelled.';
          return;
        }
        response = await apiFetch(url + '?replace=true', options);
        failure = response.ok ? null : await response.json().catch(() => null);
      }
      if (!response.ok) throw new Error(failure?.error || 'HTTP ' + response.status);
      status.textContent = 'Imported.';
      document.body.dispatchEvent(new Event('exhibit:' + kind + '-imported'));
      if (kind === 'assets') document.body.dispatchEvent(new Event('exhibit:assets-changed'));
    } catch (error) {
      status.textContent = 'Could not import ' + kind + ': ' + error.message;
      status.classList.add('error');
    } finally {
      input.value = '';
      input.disabled = false;
      button.disabled = false;
    }
  });
}
