/* Gallery index (library) page script. Served from the app origin at
 * /assets/gallery/index.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads before it loads:
 *   TOKEN / READ_ONLY - this visitor's API credential, decided server-side
 *                       per request (av-5imk); spent via api.js's apiFetch
 *   DEFAULT_TAG_COLOR - store.DefaultTagColor, the add-tag modal's preset
 */

// Eager search: filter the gallery as the user types instead of waiting for a
// submit. A debounced fetch re-asks the server-rendered gallery page with the
// current query and swaps only the .grid contents, so search stays authoritative
// (it runs the same FTS query as the form did) while the tag modals and the
// delegated card/tag handlers stay untouched. The empty query lists all.
(function() {
  const input = document.getElementById('search-input');
  const clear = document.getElementById('search-clear');
  if (!input) return;
  let timer = null;
  let lastQ = input.value.trim();
  syncClear();
  input.addEventListener('input', function() {
    syncClear();
    clearTimeout(timer);
    timer = setTimeout(runSearch, 220);
  });
  input.addEventListener('keydown', function(e) { if (e.key === 'Enter') { e.preventDefault(); clearTimeout(timer); runSearch(); } });
  if (clear) clear.addEventListener('click', function() { input.value = ''; syncClear(); input.focus(); clearTimeout(timer); runSearch(); });
  function syncClear() { if (clear) clear.hidden = !input.value; }
  function runSearch() {
    const q = input.value.trim();
    if (q === lastQ) return;
    lastQ = q;
    const grid = document.querySelector('.grid');
    if (!grid) return;
    grid.classList.add('grid-loading');
    const url = q ? '/?q=' + encodeURIComponent(q) : '/';
    fetch(url, { headers: { 'X-Requested-With': 'gallery-search' }, credentials: 'same-origin' })
      .then(function(r) { return r.ok ? r.text() : Promise.reject(r.statusText); })
      .then(function(html) {
        const doc = new DOMParser().parseFromString(html, 'text/html');
        const fresh = doc.querySelector('.grid');
        grid.innerHTML = fresh ? fresh.innerHTML : '';
        grid.classList.remove('grid-loading');
        if (history.replaceState) history.replaceState(null, '', url);
      })
      .catch(function() { grid.classList.remove('grid-loading'); });
  }
})();

// Tag pill hover controls: detach (x) removes this tag from this artifact
// only; edit (pencil) opens the edit-tag modal. The trailing '+' opens the
// add-tag modal for that card.
document.addEventListener('click', function(e) {
  const detachBtn = e.target.closest('.tag-pill-detach');
  if (detachBtn) {
    e.preventDefault();
    detachTag(detachBtn);
    return;
  }
  const editBtn = e.target.closest('.tag-pill-edit');
  if (editBtn) {
    e.preventDefault();
    openEditTagModal(editBtn.dataset.tagId, editBtn.dataset.tagName, editBtn.dataset.tagColor);
    return;
  }
  const addBtn = e.target.closest('.tag-add-btn');
  if (addBtn) {
    e.preventDefault();
    openAddTagModal(addBtn.dataset.artifactId);
  }
});

// Clicking anywhere on a card opens that artifact's detail/viewer page — the
// card itself is the way in. Clicks that land on an interactive child (the
// title or Details link, anything in the tag row — pills, edit/detach, the
// '+' button — or the capability cluster, whose own click toggles its
// popover in components.js) are left alone so those keep their own behavior.
// The 'Open' card action was removed; this is the single open affordance per
// card.
document.addEventListener('click', function(e) {
  if (e.target.closest('a, button, .tag-row, .card-actions, [data-capability-trigger], .capability-popover')) return;
  const card = e.target.closest('.card');
  if (!card || !card.dataset.href) return;
  window.location.href = card.dataset.href;
});

async function detachTag(btn) {
  const pill = btn.closest('.tag-pill');
  btn.disabled = true;
  try {
    const r = await apiFetch('/api/tags/' + encodeURIComponent(btn.dataset.tagId) + '/artifacts/' + encodeURIComponent(btn.dataset.artifactId), {
      method: 'DELETE'
    });
    if (r.ok) {
      pill.remove();
      return;
    }
  } catch (e) {}
  btn.disabled = false;
}

// Edit-tag modal: rename + recolor (PATCH) or delete (DELETE) a tag. Both
// mutations are global, so on success we reload the gallery rather than
// patching just the one card — every pill of that tag updates/disappears
// everywhere at once.
let editingTagId = null;

function openEditTagModal(tagId, tagName, tagColor) {
  editingTagId = tagId;
  document.getElementById('tag-edit-name').value = tagName;
  setModalColor('tag-edit', tagColor);
  setModalError('tag-edit', '');
  document.getElementById('tag-edit-modal').hidden = false;
  document.getElementById('tag-edit-name').focus();
}

function closeTagEditModal() {
  document.getElementById('tag-edit-modal').hidden = true;
  editingTagId = null;
}

// setModalColor/setModalError are shared by the edit-tag modal (tww.2.4)
// and the add-tag modal (tww.2.5), which reuse the same field ids under a
// different prefix (e.g. 'tag-edit' / 'tag-add').
function setModalColor(prefix, hex) {
  document.getElementById(prefix + '-color-hex').value = hex;
  document.getElementById(prefix + '-color-picker').value = hex;
  document.querySelectorAll('#' + prefix + '-modal .color-swatch').forEach(function(sw) {
    sw.classList.toggle('selected', sw.dataset.color.toLowerCase() === hex.toLowerCase());
  });
}

function setModalError(prefix, message) {
  const el = document.getElementById(prefix + '-error');
  el.textContent = message;
  el.hidden = !message;
}

function wireColorControls(prefix) {
  document.querySelectorAll('#' + prefix + '-modal .color-swatch').forEach(function(sw) {
    sw.addEventListener('click', function() { setModalColor(prefix, sw.dataset.color); });
  });
  document.getElementById(prefix + '-color-picker').addEventListener('input', function(e) {
    setModalColor(prefix, e.target.value);
  });
  document.getElementById(prefix + '-color-hex').addEventListener('input', function(e) {
    if (/^#[0-9a-fA-F]{6}$/.test(e.target.value)) setModalColor(prefix, e.target.value);
  });
}
wireColorControls('tag-edit');

document.getElementById('tag-edit-cancel').addEventListener('click', closeTagEditModal);
document.getElementById('tag-edit-modal').addEventListener('click', function(e) {
  if (e.target.id === 'tag-edit-modal') closeTagEditModal();
});
document.addEventListener('keydown', function(e) {
  if (e.key !== 'Escape') return;
  if (!document.getElementById('tag-edit-modal').hidden) closeTagEditModal();
  if (!document.getElementById('tag-add-modal').hidden) closeTagAddModal();
});

document.getElementById('tag-edit-save').addEventListener('click', async function() {
  const name = document.getElementById('tag-edit-name').value.trim();
  const color = document.getElementById('tag-edit-color-hex').value.trim();
  if (!name) { setModalError('tag-edit', 'Name is required.'); return; }
  const r = await apiFetch('/api/tags/' + encodeURIComponent(editingTagId), {
    method: 'PATCH',
    body: JSON.stringify({name: name, color: color})
  });
  if (!r.ok) {
    const data = await r.json().catch(function() { return {}; });
    setModalError('tag-edit', data.error || 'Failed to save tag.');
    return;
  }
  location.reload();
});

document.getElementById('tag-edit-delete').addEventListener('click', async function() {
  const name = document.getElementById('tag-edit-name').value;
  if (!confirm('Delete tag "' + name + '"? It will be removed from every artifact. This cannot be undone.')) return;
  const r = await apiFetch('/api/tags/' + encodeURIComponent(editingTagId), {
    method: 'DELETE'
  });
  if (!r.ok) {
    const data = await r.json().catch(function() { return {}; });
    setModalError('tag-edit', data.error || 'Failed to delete tag.');
    return;
  }
  location.reload();
});

// Add-tag modal: pick an existing tag from the dropdown, or "create new" to
// reveal the same name+color fields as the edit-tag modal. Confirm creates
// the tag first (if new) and always attaches it; attaching a tag the
// artifact already has is a no-op on the server, so no special-casing is
// needed here.
let addingArtifactId = null;

function openAddTagModal(artifactId) {
  addingArtifactId = artifactId;
  document.getElementById('tag-add-select').value = '';
  document.getElementById('tag-add-create-fields').hidden = true;
  document.getElementById('tag-add-name').value = '';
  setModalColor('tag-add', DEFAULT_TAG_COLOR);
  setModalError('tag-add', '');
  document.getElementById('tag-add-modal').hidden = false;
  document.getElementById('tag-add-select').focus();
}

function closeTagAddModal() {
  document.getElementById('tag-add-modal').hidden = true;
  addingArtifactId = null;
}

document.getElementById('tag-add-select').addEventListener('change', function(e) {
  document.getElementById('tag-add-create-fields').hidden = e.target.value !== '__new__';
});
wireColorControls('tag-add');

document.getElementById('tag-add-cancel').addEventListener('click', closeTagAddModal);
document.getElementById('tag-add-modal').addEventListener('click', function(e) {
  if (e.target.id === 'tag-add-modal') closeTagAddModal();
});

document.getElementById('tag-add-confirm').addEventListener('click', async function() {
  const choice = document.getElementById('tag-add-select').value;
  if (!choice) { setModalError('tag-add', 'Choose a tag or create a new one.'); return; }

  let tagId = choice;
  if (choice === '__new__') {
    const name = document.getElementById('tag-add-name').value.trim();
    if (!name) { setModalError('tag-add', 'Name is required.'); return; }
    const color = document.getElementById('tag-add-color-hex').value.trim();
    const created = await apiFetch('/api/tags', {
      method: 'POST',
      body: JSON.stringify({name: name, color: color})
    });
    const data = await created.json().catch(function() { return {}; });
    if (!created.ok) { setModalError('tag-add', data.error || 'Failed to create tag.'); return; }
    tagId = data.id;
  }

  const attached = await apiFetch('/api/tags/' + encodeURIComponent(tagId) + '/artifacts/' + encodeURIComponent(addingArtifactId), {
    method: 'POST'
  });
  if (!attached.ok) {
    const data = await attached.json().catch(function() { return {}; });
    setModalError('tag-add', data.error || 'Failed to attach tag.');
    return;
  }
  location.reload();
});
