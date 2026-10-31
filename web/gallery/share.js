/* Owner-only sharing controls. Every action persists immediately; htmx refreshes
 * the server-rendered panel after a mutation. Keep the request locked through
 * that refresh so a second action cannot use an obsolete share id or mode.
 */
import { apiFetch } from './api-module.js';
import { modalFor } from './modal.js';

function initializeSharePanel() {
  const panel = document.getElementById('share-panel');
  if (!panel) return;
  const dialog = modalFor(panel);
  const artifactID = panel.querySelector('.share-cols').dataset.artifactId;
  const byId = (id) => panel.querySelector('#' + id);
  const readSaved = () => ({
    mode: byId('share-state-mode').dataset.savedMode,
    linkID: byId('share-link-toggle').dataset.shareId || ''
  });
  let saved = readSaved();
  let finishRefresh;
  let needsRefresh = false;
  let recipients = '';
  let composing = false;
  let focusAfterRefresh = '';
  let mutationStatus = '';
  const disclosureState = new Map();
  let peopleScrollTop = 0;

  function status(message) {
    byId('share-status').textContent = message;
  }

  function restoreFields() {
    byId('share-state-own').checked = saved.mode === 'own';
    byId('share-state-shared').checked = saved.mode === 'shared';
    byId('share-link-toggle').checked = !!saved.linkID;
    byId('share-add-input').value = recipients;
  }

  function rememberDisclosures() {
    const people = panel.querySelector('.share-people-list');
    if (people) peopleScrollTop = people.scrollTop;
    ['state-how-it-works', 'share-widget-details'].forEach((id) => {
      const disclosure = byId(id);
      if (disclosure) disclosureState.set(id, disclosure.open);
    });
  }

  async function refresh(operation) {
    const completed = await dialog.run(async () => {
      if (operation) await operation();
      rememberDisclosures();
      needsRefresh = true;
      await new Promise(resolve => {
        finishRefresh = resolve;
        document.body.dispatchEvent(new CustomEvent('exhibit:shares-changed'));
      });
      return true;
    });
    if (!completed) return;
    // A failed refresh leaves stale settings locked, but Done becomes usable.
    byId('share-body').disabled = needsRefresh;
    if (!needsRefresh && panel.open && focusAfterRefresh) byId(focusAfterRefresh)?.focus();
    focusAfterRefresh = '';
  }

  panel.addEventListener('modal:closed', () => {
    recipients = '';
    composing = false;
    const input = byId('share-add-input');
    if (input) input.value = '';
  });
  document.getElementById('share-open').addEventListener('click', event => {
    dialog.show(event.currentTarget);
    if (needsRefresh) refresh();
    else status('');
  });

  async function request(path, options) {
    const response = await apiFetch(path, options);
    // A concurrent revoke already achieved this DELETE's intended result.
    if (response && response.status === 404 && options.method === 'DELETE') return response;
    if (!response || !response.ok) throw new Error('request failed');
    return response;
  }
  const post = (body) => request('/api/shares', { method: 'POST', body: JSON.stringify(body) });

  function mutate(operation, failureMessage, focus = '') {
    if (dialog.pending || needsRefresh) return;
    focusAfterRefresh = focus;
    return refresh(async () => {
      status('Saving changes…');
      try {
        const message = await operation();
        mutationStatus = message || '✓ Changes saved';
      } catch (error) {
        mutationStatus = failureMessage;
        // Even a failed bulk grant may have saved some names before its error.
        // Reconcile with the server and retain the input for a safe retry.
        restoreFields();
      }
      status(mutationStatus);
    });
  }

  // The single text field uses native implicit form submission: desktop Enter
  // and the mobile keyboard action follow the same path without a hidden Add
  // control. Composition Enter belongs to the IME and must not submit a grant.
  panel.addEventListener('compositionstart', function(e) {
    if (e.target.id === 'share-add-input') composing = true;
  });
  panel.addEventListener('compositionend', function(e) {
    if (e.target.id === 'share-add-input') composing = false;
  });
  panel.addEventListener('keydown', function(e) {
    if (e.target.id === 'share-add-input' && e.key === 'Enter' &&
        (composing || e.isComposing || e.keyCode === 229)) e.preventDefault();
  });
  panel.addEventListener('input', function(e) {
    if (e.target.id !== 'share-add-input' || dialog.pending || needsRefresh) return;
    recipients = e.target.value;
    status('');
  });
  panel.addEventListener('submit', function(e) {
    if (e.target.id !== 'share-add-form') return;
    e.preventDefault();
    if (composing || dialog.pending || needsRefresh) return;
    recipients = byId('share-add-input').value;
    const names = [...new Map(recipients.split(/[,\n]+/).map((name) => name.trim())
      .filter(Boolean).map((name) => [name.toLowerCase(), name])).values()];
    if (names.length === 0) return;
    return mutate(async () => {
      const response = await post({ artifact_id: artifactID, recipients: names });
      const data = await response.json();
      if (!Array.isArray(data.results) || data.results.length !== names.length) throw new Error('missing grant results');
      const errors = [];
      const unresolved = [];
      for (const result of data.results) {
        if (['granted', 'already_shared', 'self'].includes(result.status)) continue;
        unresolved.push(result.name);
        errors.push(result.name + ': ' + (result.status === 'unknown'
          ? 'no account with that name or address'
          : result.status === 'ambiguous' ? 'that address matches more than one account' : 'could not grant access'));
      }
      recipients = unresolved.join(', ');
      return errors.length ? 'Check these names: ' + errors.join('; ') + '.' : '';
    }, 'Could not grant access. Your names are still in the field; try again.', 'share-add-input');
  });

  panel.addEventListener('change', function(e) {
    if (dialog.pending || needsRefresh) {
      restoreFields();
      return;
    }
    if (e.target.id === 'share-link-toggle') {
      const enabled = !!e.target.checked;
      if (enabled === !!saved.linkID) return;
      return mutate(async () => {
        if (enabled) {
          const response = await post({ artifact_id: artifactID });
          const data = await response.json();
          if (!data.share || !data.share.id) throw new Error('missing link id');
          saved.linkID = data.share.id;
        } else {
          await request('/api/shares/' + encodeURIComponent(saved.linkID), { method: 'DELETE' });
          saved.linkID = '';
        }
      }, 'Could not change link access. Your previous setting is restored; try again.');
    }
    if (e.target.name === 'share-state-mode') {
      const mode = e.target.value;
      if (!['own', 'shared'].includes(mode) || mode === saved.mode) return;
      return mutate(async () => {
        await request('/api/artifacts/' + encodeURIComponent(artifactID), { method: 'PATCH', body: JSON.stringify({ share_state_mode: mode }) });
        saved.mode = mode;
      }, 'Could not change the data mode. Your previous setting is restored; try again.');
    }
  });

  function copyField(id, what) {
    const field = byId(id);
    if (!field) return;
    field.select();
    if (!navigator.clipboard) {
      status('Select the ' + what.toLowerCase() + ' and copy it.');
      return;
    }
    return navigator.clipboard.writeText(field.value)
      .then(() => status('✓ ' + what + ' copied'))
      .catch(() => status('Select the ' + what.toLowerCase() + ' and copy it.'));
  }

  panel.addEventListener('click', function(e) {
    if (dialog.pending || needsRefresh) return;
    const target = e.target;
    const revoke = target.closest ? target.closest('[data-action="revoke"]') : null;
    if (revoke) {
      return mutate(() => request('/api/artifacts/' + encodeURIComponent(artifactID) + '/grants?recipient=' + encodeURIComponent(revoke.dataset.recipientId), { method: 'DELETE' })
        .then(() => ''), 'Could not revoke access. This person still appears in the list; try again.');
    }
    const button = target.closest ? target.closest('button') : target;
    if (!button) return;
    if (button.id === 'share-link-replace') {
      return mutate(async () => {
        const response = await post({ artifact_id: artifactID, replace: true });
        const data = await response.json();
        if (!data.share || !data.share.id) throw new Error('missing link id');
        saved.linkID = data.share.id;
      }, 'Could not replace the link. Try again.');
    }
    if (button.id === 'share-link-copy') return copyField('share-link-url', 'Link');
    if (button.id === 'share-widget-copy') return copyField('share-widget-embed', 'Embed code');
  });

  // htmx 4 exposes response metadata under detail.ctx and uses colon-separated
  // event names. Preserve the panel on server/network errors so reopening can
  // retry the GET without replaying the mutation. A 404 replaces stale controls
  // with the server's missing-artifact notice.
  function refreshFailed() {
    needsRefresh = true;
    finishRefresh?.();
    finishRefresh = null;
    status('The sharing panel could not refresh. Close and reopen it to retry.');
  }
  panel.addEventListener('htmx:after:request', function(e) {
    if (e.target.id !== 'share-body') return;
    const response = e.detail.ctx.response;
    if (response.status >= 500) {
      e.preventDefault();
      refreshFailed();
    }
  });
  panel.addEventListener('htmx:error', function(e) {
    if (e.target.id === 'share-body') refreshFailed();
  });
  panel.addEventListener('htmx:before:swap', function(e) {
    if (e.target.id === 'share-body') rememberDisclosures();
  });
  panel.addEventListener('htmx:after:swap', function(e) {
    if (e.target.id !== 'share-body') return;
    needsRefresh = false;
    if (!byId('share-state-mode')) {
      // An error fragment has no settings to restore or recipient field.
      recipients = '';
      composing = false;
      focusAfterRefresh = 'share-close';
      mutationStatus = '';
      status('');
      finishRefresh?.();
      finishRefresh = null;
      return;
    }
    saved = readSaved();
    status(mutationStatus);
    restoreFields();
    for (const [id, open] of disclosureState) {
      const disclosure = byId(id);
      if (disclosure) disclosure.open = open;
    }
    const people = panel.querySelector('.share-people-list');
    if (people) people.scrollTop = peopleScrollTop;
    finishRefresh?.();
    finishRefresh = null;
  });
}

initializeSharePanel();
