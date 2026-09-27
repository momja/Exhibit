/* The edit page's Tags panel. Served from the app origin at
 * /assets/gallery/tags.js, loaded by edit.tmpl.
 *
 * This is the one place an artifact's tags change: the library grid renders
 * them as static pills. Attaching, detaching and creating act on this
 * artifact; the edit-tag modal renames, recolors or deletes a tag across the
 * whole library.
 *
 * Globals from the page's inline bootstrap:
 *   ID                - the artifact id
 *   TOKEN / READ_ONLY - this visitor's API credential (av-5imk), spent by
 *                       api.js's apiFetch
 *
 * Every change goes through the tag API and then fires `exhibit:tags-changed`,
 * which htmx turns into a re-fetch of /partials/tag-panel — the server owns the
 * rows, in one definition shared with the full page render, so nothing here
 * builds markup over user-authored tag names. No reload either: it would drop
 * the unsaved buffers in the editors elsewhere on this page.
 *
 * Listeners are delegated from #tags-panel, which is stable — htmx replaces the
 * contents of #tags-body inside it, so anything bound to an element in there
 * would be bound to a node that is gone after the first change.
 */
(function() {
  const panel = document.getElementById('tags-panel');
  if (!panel) return;

  const byId = (id) => document.getElementById(id);

  function changed() {
    document.body.dispatchEvent(new CustomEvent('exhibit:tags-changed'));
  }

  // One error line per surface: the panel's lives outside the swapped region
  // so the re-render that follows a failed attempt does not wipe it.
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

  // Colors: a swatch, the native picker and the hex field all describe one
  // value, so setting any of them sets all three. `scope` is the element the
  // swatches live under — the panel's create fields or the edit modal.
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
    const r = await apiFetch('/api/tags/' + encodeURIComponent(tagID) + '/artifacts/' + encodeURIComponent(ID), {
      method: 'DELETE'
    }).catch(function() { return null; });
    if (!r || !r.ok) setError('tag-add-error', await errorOf(r, 'Could not remove the tag.'));
    changed();
  }

  // Attach an existing tag, or create one first when "create new" is chosen.
  // Attaching a tag the artifact already carries is a no-op on the server.
  async function add() {
    setError('tag-add-error', '');
    const select = byId('tag-add-select');
    const choice = select ? select.value : '';
    if (!choice) { setError('tag-add-error', 'Choose a tag or create a new one.'); return; }

    let tagID = choice;
    if (choice === '__new__') {
      const name = byId('tag-add-name').value.trim();
      if (!name) { setError('tag-add-error', 'Name is required.'); return; }
      const created = await apiFetch('/api/tags', {
        method: 'POST',
        body: JSON.stringify({ name: name, color: byId('tag-add-color-hex').value.trim() })
      }).catch(function() { return null; });
      if (!created || !created.ok) { setError('tag-add-error', await errorOf(created, 'Could not create the tag.')); return; }
      const data = await created.json().catch(function() { return {}; });
      tagID = data.id;
    }

    const attached = await apiFetch('/api/tags/' + encodeURIComponent(tagID) + '/artifacts/' + encodeURIComponent(ID), {
      method: 'POST'
    }).catch(function() { return null; });
    if (!attached || !attached.ok) setError('tag-add-error', await errorOf(attached, 'Could not add the tag.'));
    // A tag created just now exists even when the attach failed, so the
    // dropdown is re-rendered either way.
    changed();
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
    if (target.closest && target.closest('#tag-add-confirm')) return add();
    return undefined;
  });

  panel.addEventListener('change', function(e) {
    if (e.target.id !== 'tag-add-select') return;
    const fields = byId('tag-add-create-fields');
    const creating = e.target.value === '__new__';
    if (fields) fields.hidden = !creating;
    if (creating) {
      setColor('tag-add', fields, byId('tag-add-color-hex').value);
      byId('tag-add-name').focus();
    }
  });

  panel.addEventListener('input', function(e) {
    onColorInput('tag-add', byId('tag-add-create-fields'), e.target);
  });

  panel.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && e.target && e.target.id === 'tag-add-name') {
      e.preventDefault();
      add();
    }
  });

  // --- the edit-tag modal: library-wide rename, recolor, delete -----------

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
      // Backdrop only: a click on the dialog inside it does not close it.
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
