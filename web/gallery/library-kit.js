// The library's component kit picker on the profile page (Exh-61ix). A choice
// saves as soon as it is made: it is one setting, not part of a draft, and
// every artifact without a kit of its own picks it up on its next render.
import { apiFetch } from './api-module.js';

const select = document.getElementById('library-kit');
const status = document.getElementById('library-kit-status');

if (select && status) {
  let saved = select.value;
  select.addEventListener('change', async () => {
    const chosen = select.value;
    select.disabled = true;
    status.textContent = 'Saving…';
    try {
      const resp = await apiFetch('/api/settings/kit', {
        method: 'PUT',
        body: JSON.stringify({ kit: chosen }),
      });
      if (!resp.ok) {
        const data = await resp.json().catch(() => ({}));
        throw new Error(data.error || resp.statusText);
      }
      saved = chosen;
      status.textContent = '✓ Saved. Artifacts restyle the next time they load.';
    } catch (err) {
      select.value = saved;
      status.textContent = '✗ Not saved: ' + err.message;
    } finally {
      select.disabled = false;
    }
  });
}
