/* Edit page Tags panel (edit.tmpl). Independent existing-tag and new-tag actions.
 * Every change goes through the tag API, then fires exhibit:tags-changed so
 * htmx re-renders #tags-body. Listeners are delegated from #tags-panel, which
 * survives the swap.
 */
import { apiFetch } from './api-module.js';

(function() {
  const panel = document.getElementById('tags-panel');
  if (!panel) return;

  const artifactID = panel.dataset.artifactId;
  const byId = (id) => document.getElementById(id);

  function changed() {
    document.body.dispatchEvent(new CustomEvent('exhibit:tags-changed'));
  }

  function setError(id, message) {
    const el = byId(id);
    if (!el) return;
    el.textContent = message;
    el.hidden = !message;
  }

  async function errorOf(r, fallback) {
    if (!r) return fallback;
    const data = await r.json().catch(function() { return {}; });
    return data.error || fallback;
  }

  // Keeps swatches, picker and hex field in sync. `scope` holds the swatches.
  function setColor(prefix, scope, hex) {
    const hexField = byId(prefix + '-color-hex');
    const picker = byId(prefix + '-color-picker');
    if (hexField) hexField.value = hex;
    if (picker) picker.value = hex;
    if (!scope) return;
    scope.querySelectorAll('.color-swatch').forEach(function(sw) {
      sw.classList.toggle('selected', sw.dataset.color.toLowerCase() === hex.toLowerCase());
    });
  }

  function onColorInput(prefix, scope, target) {
    if (target.id === prefix + '-color-picker') setColor(prefix, scope, target.value);
    else if (target.id === prefix + '-color-hex' && /^#[0-9a-fA-F]{6}$/.test(target.value)) setColor(prefix, scope, target.value);
  }

  // --- this artifact's tags ----------------------------------------------

  async function detach(tagID) {
    setError('tag-add-error', '');
    const r = await apiFetch('/api/tags/' + encodeURIComponent(tagID) + '/artifacts/' + encodeURIComponent(artifactID), {
      method: 'DELETE'
    }).catch(function() { return null; });
    if (!r || !r.ok) setError('tag-add-error', await errorOf(r, 'Could not remove the tag.'));
    changed();
  }

  async function attach(tagID, errorID) {
    const attached = await apiFetch('/api/tags/' + encodeURIComponent(tagID) + '/artifacts/' + encodeURIComponent(artifactID), {
      method: 'POST'
    }).catch(function() { return null; });
    if (!attached || !attached.ok) setError(errorID, await errorOf(attached, 'Could not add the tag.'));
    // Re-render even on failure: a just-created tag belongs in the dropdown.
    changed();
  }

  async function addExisting() {
    setError('tag-add-error', '');
    const tagID = byId('tag-add-select').value;
    if (!tagID) { setError('tag-add-error', 'Choose an existing tag to add.'); return; }
    await attach(tagID, 'tag-add-error');
  }

  async function createAndAdd() {
    const button = byId('tag-create-confirm');
    if (button.disabled) return;
    setError('tag-create-error', '');
    const name = byId('tag-add-name').value.trim();
    if (!name) { setError('tag-create-error', 'Name is required.'); byId('tag-add-name').focus(); return; }
    button.disabled = true;
    try {
      const created = await apiFetch('/api/tags', {
        method: 'POST',
        body: JSON.stringify({ name: name, color: byId('tag-add-color-hex').value.trim() })
      }).catch(function() { return null; });
      if (!created || !created.ok) { setError('tag-create-error', await errorOf(created, 'Could not create the tag.')); return; }
      const data = await created.json();
      // Attachment errors must survive the htmx refresh of the creation fields.
      setError('tag-add-error', '');
      await attach(data.id, 'tag-add-error');
    } finally {
      button.disabled = false;
    }
  }

  panel.addEventListener('click', function(e) {
    const target = e.target;
    const action = target.closest ? target.closest('[data-action]') : null;
    if (action && action.dataset.action === 'detach-tag') return detach(action.dataset.tagId);
    if (action && action.dataset.action === 'edit-tag') {
      return openEditModal(action.dataset.tagId, action.dataset.tagName, action.dataset.tagColor);
    }
    const swatch = target.closest ? target.closest('.color-swatch') : null;
    if (swatch) return setColor('tag-add', byId('tag-add-create-fields'), swatch.dataset.color);
    if (target.closest && target.closest('#tag-add-confirm')) return addExisting();
    if (target.closest && target.closest('#tag-create-confirm')) return createAndAdd();
    return undefined;
  });

  panel.addEventListener('input', function(e) {
    onColorInput('tag-add', byId('tag-add-create-fields'), e.target);
  });

  panel.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && e.target && (e.target.id === 'tag-add-name' || e.target.id === 'tag-add-color-hex')) {
      e.preventDefault();
      return createAndAdd();
    }
  });

  // --- edit-tag modal: library-wide rename, recolor, delete ---

  const modal = byId('tag-edit-modal');
  let editingTagID = null;

  function openEditModal(tagID, name, hex) {
    editingTagID = tagID;
    byId('tag-edit-name').value = name;
    setColor('tag-edit', modal, hex);
    setError('tag-edit-error', '');
    modal.hidden = false;
    byId('tag-edit-name').focus();
  }

  function closeEditModal() {
    modal.hidden = true;
    editingTagID = null;
  }

  async function saveTag() {
    const name = byId('tag-edit-name').value.trim();
    if (!name) { setError('tag-edit-error', 'Name is required.'); return; }
    const r = await apiFetch('/api/tags/' + encodeURIComponent(editingTagID), {
      method: 'PATCH',
      body: JSON.stringify({ name: name, color: byId('tag-edit-color-hex').value.trim() })
    }).catch(function() { return null; });
    if (!r || !r.ok) { setError('tag-edit-error', await errorOf(r, 'Failed to save tag.')); return; }
    closeEditModal();
    changed();
  }

  async function deleteTag() {
    const name = byId('tag-edit-name').value;
    if (!confirm('Delete tag "' + name + '"? It will be removed from every artifact. This cannot be undone.')) return;
    const r = await apiFetch('/api/tags/' + encodeURIComponent(editingTagID), {
      method: 'DELETE'
    }).catch(function() { return null; });
    if (!r || !r.ok) { setError('tag-edit-error', await errorOf(r, 'Failed to delete tag.')); return; }
    closeEditModal();
    changed();
  }

  if (modal) {
    modal.addEventListener('click', function(e) {
      const target = e.target;
      if (target === modal) return closeEditModal();
      const swatch = target.closest ? target.closest('.color-swatch') : null;
      if (swatch) return setColor('tag-edit', modal, swatch.dataset.color);
      if (target.closest && target.closest('#tag-edit-cancel')) return closeEditModal();
      if (target.closest && target.closest('#tag-edit-save')) return saveTag();
      if (target.closest && target.closest('#tag-edit-delete')) return deleteTag();
      return undefined;
    });
    modal.addEventListener('input', function(e) { onColorInput('tag-edit', modal, e.target); });
    document.addEventListener('keydown', function(e) {
      if (e.key === 'Escape' && !modal.hidden) closeEditModal();
    });
  }
})();
