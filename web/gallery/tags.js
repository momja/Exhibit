/* Tag APIs persist independently of page Save. Templates own all markup;
 * htmx refreshes the list/forms, while these handlers retain local feedback.
 */
import { apiFetch } from './api-module.js';
import { modalFor } from './modal.js';

const panel = document.getElementById('tags-panel');
if (panel) connectTags(panel);

function connectTags(panel) {
  const artifactID = panel.dataset.artifactId;
  const byId = id => panel.querySelector('#' + id);
  const modal = byId('tag-edit-modal');
  const editor = modalFor(modal);
  const fields = byId('tags-body');
  const refreshNotice = byId('tag-refresh-notice');
  const retainedErrors = new Map();
  let panelPending = false;
  let needsRefresh = false;
  let editingTagID = null;
  let originalName = '';
  let originalColor = '';
  let focusTagID = null;

  function status(message = 'Adding or removing a tag saves immediately.') {
    byId('tag-status').textContent = message;
  }

  function setError(id, message) {
    if (id === 'tag-add-error' || id === 'tag-list-error') {
      if (message) retainedErrors.set(id, message);
      else retainedErrors.delete(id);
    }
    const element = byId(id);
    if (!element) return;
    element.textContent = message;
    element.hidden = !message;
  }

  function clearPanelErrors() {
    for (const id of ['tag-add-error', 'tag-list-error', 'tag-create-error']) setError(id, '');
  }

  function refresh(tagID) {
    focusTagID = tagID;
    needsRefresh = true;
    setPanelPending(true);
    document.body.dispatchEvent(new CustomEvent('exhibit:tags-changed'));
  }

  function setPanelPending(pending) {
    panelPending = pending;
    fields.disabled = pending || needsRefresh;
    byId('tag-refresh').disabled = pending;
    if (pending) panel.setAttribute('aria-busy', 'true');
    else panel.removeAttribute('aria-busy');
    syncAdd();
    syncCreate();
  }

  panel.addEventListener('htmx:before:request', event => {
    if (event.target !== fields) return;
    needsRefresh = true;
    setPanelPending(true);
  });
  function refreshFailed() {
    needsRefresh = true;
    setPanelPending(false);
    refreshNotice.hidden = false;
    byId('tag-refresh').focus();
  }
  panel.addEventListener('htmx:after:request', event => {
    if (event.target === fields && event.detail.ctx.response.status >= 400) {
      event.preventDefault();
      refreshFailed();
    }
  });
  panel.addEventListener('htmx:error', event => {
    if (event.target === fields) refreshFailed();
  });
  panel.addEventListener('htmx:after:swap', event => {
    if (event.target !== fields) return;
    needsRefresh = false;
    setPanelPending(false);
    if (!refreshNotice.hidden) status('Tags refreshed.');
    refreshNotice.hidden = true;
    for (const [id, message] of retainedErrors) setError(id, message);
    if (!focusTagID || modal.open) return;
    const pencil = [...panel.querySelectorAll('[data-action="edit-tag"]')]
      .find(button => button.dataset.tagId === focusTagID);
    const select = byId('tag-add-select');
    if (!pencil && [...(select?.options || [])].some(option => option.value === focusTagID)) select.value = focusTagID;
    syncAdd();
    (pencil || select || byId('tag-add-name'))?.focus();
    focusTagID = null;
  });

  async function errorOf(response, fallback) {
    const data = await response?.json().catch(() => ({}));
    return data?.error || fallback;
  }

  async function request(path, method, body) {
    return apiFetch(path, {
      method, ...(body ? { body: JSON.stringify(body) } : {})
    }).catch(() => null);
  }

  function validColor(value) { return /^#[0-9a-f]{6}$/i.test(value.trim()); }

  function nameError(inputID, errorID, message) {
    byId(inputID).setAttribute('aria-invalid', String(!!message));
    setError(errorID, message);
    const hint = byId(errorID.replace(/-error$/, '-hint'));
    if (hint) hint.hidden = !!message;
  }

  function readDetails(kind) {
    const editing = kind === 'edit';
    const nameID = editing ? 'tag-edit-name' : 'tag-add-name';
    const errorID = editing ? 'tag-edit-name-error' : 'tag-create-name-error';
    const prefix = editing ? 'tag-edit' : 'tag-add';
    const name = byId(nameID).value.trim();
    const color = byId(prefix + '-color-hex').value.trim();
    nameError(nameID, errorID, name ? '' : 'Name is required.');
    if (!name) { byId(nameID).focus(); return null; }
    if (!validColor(color)) {
      byId(prefix + '-color-controls').dispatchEvent(new CustomEvent('color-picker:refresh'));
      byId(prefix + '-color-hex').focus();
      return null;
    }
    return { name, color: color.toLowerCase() };
  }

  function syncAdd() {
    const button = byId('tag-add-confirm');
    if (button) button.disabled = panelPending || !byId('tag-add-select')?.value;
  }

  function syncCreate() {
    const name = byId('tag-add-name');
    byId('tag-create-confirm').disabled = panelPending || !name.value.trim() || !validColor(byId('tag-add-color-hex').value) || name.getAttribute('aria-invalid') === 'true';
  }

  async function runPanel(message, operation) {
    if (panelPending || needsRefresh || editor.pending) return;
    clearPanelErrors();
    setPanelPending(true);
    status(message);
    try { await operation(); }
    finally { if (!needsRefresh) setPanelPending(false); }
  }

  async function detach(tagID) {
    await runPanel('Removing tag…', async () => {
      const response = await request('/api/tags/' + encodeURIComponent(tagID) + '/artifacts/' + encodeURIComponent(artifactID), 'DELETE');
      if (!response?.ok) {
        setError('tag-list-error', await errorOf(response, 'Could not remove the tag. Try again.'));
        status();
        return;
      }
      status('Tag removed from this artifact.');
      refresh(tagID);
    });
  }

  async function attach(tagID) {
    return request('/api/tags/' + encodeURIComponent(tagID) + '/artifacts/' + encodeURIComponent(artifactID), 'POST');
  }

  async function addExisting() {
    if (panelPending) return;
    const tagID = byId('tag-add-select')?.value;
    if (!tagID) {
      setError('tag-add-error', 'Choose an existing tag to add.');
      byId('tag-add-select')?.focus();
      return;
    }
    await runPanel('Adding tag…', async () => {
      const response = await attach(tagID);
      if (!response?.ok) {
        setError('tag-add-error', await errorOf(response, 'Could not add the tag. Try again.'));
        status();
        return;
      }
      status('Tag added.');
      refresh(tagID);
    });
  }

  async function createAndAdd() {
    if (panelPending) return;
    const name = byId('tag-add-name');
    if (name.value.trim() && name.getAttribute('aria-invalid') === 'true') { name.focus(); return; }
    setError('tag-create-error', '');
    const details = readDetails('create');
    if (!details) return;
    let focusName = false;
    await runPanel('Creating tag…', async () => {
      byId('tag-create-label').textContent = 'Creating…';
      try {
        const response = await request('/api/tags', 'POST', details);
        if (!response?.ok) {
          if (response?.status === 409) {
            nameError('tag-add-name', 'tag-create-name-error', await errorOf(response, 'A tag with this name already exists.'));
            focusName = true;
          } else setError('tag-create-error', await errorOf(response, 'Could not create the tag. Try again.'));
          status();
          return;
        }
        const created = await response.json();
        byId('tag-add-name').value = '';
        const attached = await attach(created.id);
        if (!attached?.ok) {
          setError('tag-add-error', 'Tag created, but not added. ' + await errorOf(attached, 'Choose it above to try again.'));
          status('Tag created. Choose it above to add it.');
          // The new tag must become available, and the nearby error must survive.
          refresh(created.id);
          return;
        }
        status('Tag created and added.');
        refresh(created.id);
      } finally { byId('tag-create-label').textContent = 'Create and add tag'; }
    });
    if (focusName) byId('tag-add-name').focus();
  }

  function syncSave() {
    const name = byId('tag-edit-name').value.trim();
    const color = byId('tag-edit-color-hex').value.trim().toLowerCase();
    const dirty = name !== originalName || color !== originalColor;
    byId('tag-edit-save').disabled = editor.pending || !dirty || !name || !validColor(color) || byId('tag-edit-name').getAttribute('aria-invalid') === 'true';
  }

  function deleteView(deleting, focus = true) {
    if (deleting) setError('tag-delete-error', '');
    byId('tag-edit-fields').hidden = deleting;
    byId('tag-delete-fields').hidden = !deleting;
    byId('tag-edit-cancel').hidden = deleting;
    byId('tag-edit-save').hidden = deleting;
    byId('tag-delete-cancel').hidden = !deleting;
    byId('tag-edit-confirm-delete').hidden = !deleting;
    byId('tag-edit-title').textContent = deleting ? 'Delete tag?' : 'Edit tag';
    modal.setAttribute('aria-describedby', deleting ? 'tag-delete-help' : 'tag-edit-scope');
    if (focus) byId(deleting ? 'tag-delete-cancel' : 'tag-edit-delete').focus();
  }

  function openEditModal(tagID, name, color, trigger) {
    if (modal.open || panelPending || needsRefresh) return;
    editingTagID = tagID;
    originalName = name;
    originalColor = color.toLowerCase();
    byId('tag-edit-name').value = name;
    byId('tag-edit-color-hex').value = color;
    byId('tag-edit-color-picker').value = color;
    byId('tag-delete-name').textContent = name;
    deleteView(false, false);
    nameError('tag-edit-name', 'tag-edit-name-error', '');
    setError('tag-edit-error', '');
    setError('tag-delete-error', '');
    byId('tag-edit-status').textContent = '';
    modal.querySelector('[data-controller~=color-picker]').dispatchEvent(new CustomEvent('color-picker:refresh'));
    syncSave();
    editor.show(trigger);
  }

  async function mutateTag(method, details) {
    if (!modal.open || editor.pending) return;
    const deleting = method === 'DELETE';
    const labelID = deleting ? 'tag-delete-label' : 'tag-edit-save-label';
    const errorID = deleting ? 'tag-delete-error' : 'tag-edit-error';
    setError(errorID, '');
    byId(labelID).textContent = deleting ? 'Deleting…' : 'Saving…';
    byId('tag-edit-status').textContent = deleting ? 'Deleting tag…' : 'Saving tag…';
    modal.setAttribute('aria-busy', 'true');
    let focusName = false;
    try {
      const saved = await editor.run(async () => {
        const response = await request('/api/tags/' + encodeURIComponent(editingTagID), method, details);
        if (!response?.ok) {
          const message = await errorOf(response, deleting ? 'Could not delete the tag. Try again.' : 'Could not save the tag. Try again.');
          if (!deleting && response?.status === 409) {
            nameError('tag-edit-name', 'tag-edit-name-error', message);
            focusName = true;
          } else setError(errorID, message);
          byId('tag-edit-status').textContent = '';
          return false;
        }
        return true;
      });
      if (saved) {
        status(deleting ? 'Tag deleted from your library.' : 'Tag saved across your library.');
        editor.close();
        refresh(editingTagID);
      } else if (focusName) byId('tag-edit-name').focus();
    } finally {
      modal.removeAttribute('aria-busy');
      byId(labelID).textContent = deleting ? 'Delete tag' : 'Save tag';
      syncSave();
    }
  }

  function saveTag() {
    if (editor.pending) return;
    const name = byId('tag-edit-name');
    if (name.value.trim() && name.getAttribute('aria-invalid') === 'true') { name.focus(); return; }
    const details = readDetails('edit');
    if (!details || details.name === originalName && details.color === originalColor) return;
    return mutateTag('PATCH', details);
  }

  panel.addEventListener('click', event => {
    const action = event.target.closest?.('[data-action]');
    if (action?.dataset.action === 'detach-tag') return detach(action.dataset.tagId);
    if (action?.dataset.action === 'edit-tag') return openEditModal(action.dataset.tagId, action.dataset.tagName, action.dataset.tagColor, action);
    if (event.target.closest?.('#tag-add-confirm')) return addExisting();
    if (event.target.closest?.('#tag-create-confirm')) return createAndAdd();
  });

  panel.addEventListener('change', event => {
    if (event.target.id === 'tag-add-select') { setError('tag-add-error', ''); syncAdd(); }
  });
  panel.addEventListener('input', event => {
    if (event.target.id === 'tag-add-name') {
      nameError('tag-add-name', 'tag-create-name-error', event.target.value.trim() ? '' : 'Name is required.');
      setError('tag-create-error', '');
      syncCreate();
    }
    if (event.target.id === 'tag-edit-name') {
      nameError('tag-edit-name', 'tag-edit-name-error', event.target.value.trim() ? '' : 'Name is required.');
      setError('tag-edit-error', '');
      syncSave();
    }
  });
  panel.addEventListener('color-picker:change', event => {
    if (modal.contains(event.target)) { setError('tag-edit-error', ''); syncSave(); }
    else syncCreate();
  });
  panel.addEventListener('keydown', event => {
    if (event.key === 'Enter' && ['tag-add-name', 'tag-add-color-hex'].includes(event.target.id)) {
      event.preventDefault();
      return createAndAdd();
    }
    if (event.key === 'Enter' && ['tag-edit-name', 'tag-edit-color-hex'].includes(event.target.id)) {
      event.preventDefault();
      return saveTag();
    }
  });
  modal.addEventListener('click', event => {
    if (event.target.closest?.('#tag-edit-save')) return saveTag();
    if (event.target.closest?.('#tag-edit-delete') && !editor.pending) return deleteView(true);
    if (event.target.closest?.('#tag-delete-cancel') && !editor.pending) return deleteView(false);
    if (event.target.closest?.('#tag-edit-confirm-delete')) return mutateTag('DELETE');
  });
  syncAdd();
  syncCreate();
}
