import { apiFetch } from './api-module.js';

// getRandomValues also works on self-hosted HTTP installations where browsers
// withhold randomUUID. Keep one identity for every retry of a modal submission.
function requestID() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 15) | 64;
  bytes[8] = (bytes[8] & 63) | 128;
  const hex = [...bytes].map(value => value.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export function initializeDuplication(root) {
  const dialog = root.querySelector('[data-duplicate-dialog]');
  if (!dialog) return;
  const form = dialog.querySelector('form');
  const title = form.elements.title;
  const share = form.elements.copy_share_settings;
  const data = form.elements.copy_data;
  const submit = form.querySelector('[type="submit"]');
  const cancel = form.querySelector('[data-duplicate-cancel]');
  const status = form.querySelector('[data-duplicate-status]');
  let artifactID = '';
  let trigger;
  let busy = false;
  let pending;
  let newArtifactID = '';

  function controls() {
    for (const input of [title, share, data]) input.disabled = busy || !!pending;
    submit.disabled = busy;
    cancel.disabled = busy;
  }
  function close() {
    if (busy) return;
    dialog.close();
    trigger?.focus();
    // An unconfirmed request may already have succeeded. Reconcile when the
    // person closes the modal too, without issuing another mutation.
    if (pending) root.dispatchEvent(new CustomEvent('exhibit:artifacts-changed', { bubbles: true }));
  }
  root.addEventListener('exhibit:duplicate', event => {
    if (busy || dialog.open) return;
    artifactID = event.detail.artifactID;
    trigger = event.detail.trigger;
    title.value = event.detail.title + ' (copy)';
    share.checked = false;
    data.checked = false;
    pending = undefined;
    status.textContent = '';
    submit.textContent = 'Duplicate artifact';
    controls();
    dialog.showModal();
    title.focus();
    title.select();
  });
  cancel.addEventListener('click', close);
  dialog.addEventListener('cancel', event => {
    event.preventDefault();
    close();
  });
  dialog.addEventListener('click', event => {
    if (event.target !== dialog) return;
    const bounds = dialog.getBoundingClientRect();
    if (event.clientX < bounds.left || event.clientX > bounds.right ||
        event.clientY < bounds.top || event.clientY > bounds.bottom) close();
  });
  form.addEventListener('submit', async event => {
    event.preventDefault();
    if (busy) return;
    if (!pending && !title.value.trim()) {
      status.textContent = 'Enter a title for the copy.';
      title.focus();
      return;
    }
    pending ||= { title: title.value.trim(), copy_share_settings: share.checked, copy_data: data.checked, request_id: requestID() };
    busy = true;
    controls();
    status.textContent = 'Duplicating artifact…';
    try {
      const response = await apiFetch('/api/artifacts/' + encodeURIComponent(artifactID) + '/duplicate', {
        method: 'POST', body: JSON.stringify(pending)
      });
      if (!response?.ok) {
        // A client refusal published nothing. A server/network failure may
        // have committed: preserve the payload and identity for a safe retry.
        if (response && response.status < 500) {
          pending = undefined;
          const content = await response.json().catch(() => ({}));
          status.textContent = content.error || 'Could not duplicate this artifact. Try again.';
        } else {
          status.textContent = 'Could not confirm duplication. Try again to check the saved result.';
        }
        submit.textContent = 'Try again';
        return;
      }
      const artifact = await response.json();
      if (!artifact?.id) throw Error('missing duplicate result');
      newArtifactID = artifact.id;
      pending = undefined;
      busy = false;
      close();
      root.dispatchEvent(new CustomEvent('exhibit:artifacts-changed', { bubbles: true }));
    } catch {
      status.textContent = 'Could not confirm duplication. Try again to check the saved result.';
      submit.textContent = 'Try again';
    } finally {
      busy = false;
      controls();
    }
  });
  root.addEventListener('htmx:after:swap', () => {
    if (!newArtifactID) return;
    const link = root.querySelector('.card-title[href="/artifacts/' + newArtifactID + '"]');
    if (link) {
      link.focus();
      newArtifactID = '';
    }
  });
}
