/* Gallery index (library) page script. Served from the app origin at
 * /assets/gallery/index.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads before it loads:
 *   TOKEN             - API bearer token
 *   DEFAULT_TAG_COLOR - store.DefaultTagColor, the add-tag modal's preset
 */

// Eager search: filter the gallery as the user types instead of waiting for a
// submit. A debounced fetch re-asks the server-rendered gallery page with the
// current query and swaps only the .grid contents, so search stays authoritative
// (it runs the same FTS query as the form did) while the upload box, tag modals,
// and delegated card/tag handlers stay untouched. The empty query lists all.
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

let currentMode = 'paste';

// Mount the CodeMirror island over the body textarea. The editor keeps
// textarea.value in sync, so ingest() below still reads the field — and if the
// bundle failed to load, the plain textarea still works. mount() hides the
// textarea and reveals its own .cm-editor element, so setMode toggles that
// element (bodyView.dom) rather than the now-hidden textarea.
let bodyView = null;
if (window.ArtifactEditor) {
  bodyView = ArtifactEditor.mount(document.getElementById('body'));
}

function setMode(mode) {
  currentMode = mode;
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  event.target.classList.add('active');
  // CodeMirror's base theme sets .cm-editor{display:flex!important}, so a plain
  // inline display:none is overridden — hide the mounted editor (or the textarea
  // fallback) with an !important inline rule, and clear it to reveal again.
  const bodyEl = bodyView ? bodyView.dom : document.getElementById('body');
  if (mode === 'paste') bodyEl.style.removeProperty('display');
  else bodyEl.style.setProperty('display', 'none', 'important');
  document.getElementById('url-input').style.display = mode === 'url' ? 'block' : 'none';
  document.getElementById('snapshot-row').style.display = mode === 'url' ? 'flex' : 'none';
}

async function ingest() {
  const title = document.getElementById('title').value.trim();
  const status = document.getElementById('status');
  const scanDiv = document.getElementById('scan-result');
  status.textContent = 'Uploading…';
  scanDiv.style.display = 'none';

  let payload;
  if (currentMode === 'url') {
    const url = document.getElementById('url-input').value.trim();
    if (!url) { status.textContent = 'Enter a URL first.'; return; }
    const snapshot = document.getElementById('snapshot-toggle').checked;
    if (snapshot) status.textContent = 'Fetching page and snapshotting assets…';
    payload = {title: title || '', url, snapshot, network_allowlist: []};
  } else {
    const body = document.getElementById('body').value.trim();
    if (!body) { status.textContent = 'Paste an artifact first.'; return; }
    payload = {title: title || 'Untitled', body, network_allowlist: []};
  }

  const resp = await fetch('/api/artifacts', {
    method: 'POST',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify(payload)
  });
  const data = await resp.json();
  if (!resp.ok) { status.textContent = 'Error: ' + (data.error || resp.statusText); return; }

  const id = data.artifact.id;
  const footprint = data.network_footprint || [];
  snapshotReportHTML = renderSnapshotReport(data.snapshot);
  if (footprint.length > 0) {
    // The artifact is saved but network-blocked (CSP connect-src 'none').
    // Pause here for explicit approval — nothing is added to the allowlist,
    // and no origin gains network access, until the user decides.
    status.textContent = '✓ Saved — review network access below.';
    showApproval(id, footprint);
    return;
  }
  finishIngest(id);
}

// The current ingest's snapshot report, kept so it stays on screen through
// the approval step and after saving (finishIngest skips the auto-reload
// that would wipe it).
let snapshotReportHTML = '';

function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function fmtBytes(n) {
  if (n < 1024) return n + ' B';
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1048576).toFixed(1) + ' MB';
}

// renderSnapshotReport turns the ingest response's snapshot report into HTML
// for the scan-result panel: vendored summary, per-asset failures, and whether
// the artifact came out fully self-contained. Returns '' when the ingest ran
// without a snapshot.
function renderSnapshotReport(rep) {
  if (!rep) return '';
  if (!rep.applied) {
    return '<div class="snapshot-report"><strong>Snapshot failed</strong> — stored the original page with a ' +
      '<code>&lt;base href&gt;</code> fallback so relative references still resolve.' +
      (rep.error ? '<div style="color:#c00;margin-top:4px">' + esc(rep.error) + '</div>' : '') +
      '</div>';
  }
  const urls = rep.vendored_urls || [];
  let html = '<div class="snapshot-report"><strong>Snapshot report</strong>' +
    '<div style="margin-top:4px">' + urls.length + ' asset' + (urls.length === 1 ? '' : 's') +
    ' vendored into the file (' + fmtBytes(rep.vendored_bytes || 0) + ').</div>';
  if (urls.length > 0) {
    html += '<details style="margin-top:4px"><summary style="cursor:pointer">Vendored assets</summary>' +
      '<ul style="margin:4px 0 0 18px">' +
      urls.map(u => '<li><code>' + esc(u) + '</code></li>').join('') +
      '</ul></details>';
  }
  const fails = rep.failures || [];
  if (fails.length > 0) {
    html += '<div style="color:#c00;margin-top:6px">' + fails.length + ' asset' + (fails.length === 1 ? '' : 's') +
      ' could not be inlined (reference kept, see origins below):</div>' +
      '<ul style="margin:4px 0 0 18px">' +
      fails.map(f => '<li><code>' + esc(f.ref) + '</code> — ' + esc(f.kind) +
        (f.detail ? ' (' + esc(f.detail) + ')' : '') + '</li>').join('') +
      '</ul>';
  }
  if ((rep.residual_origins || []).length === 0) {
    html += '<div style="color:#2a7d2a;margin-top:6px">No residual network references — the artifact is fully self-contained.</div>';
  }
  return html + '</div>';
}

// showApproval presents the scanned origins for explicit approval. Origins are
// blocked until the user approves them; nothing is written to the allowlist here.
// The snapshot report, when one exists, is shown above the approval controls.
function showApproval(id, footprint) {
  const scanDiv = document.getElementById('scan-result');
  const rows = footprint.map(o =>
    '<label style="display:block;margin:4px 0">' +
    '<input type="checkbox" class="al-origin" value="' + esc(o) + '" checked> ' +
    '<code style="background:#eee;padding:1px 5px;border-radius:3px">' + esc(o) + '</code></label>'
  ).join('');
  scanDiv.style.display = 'block';
  scanDiv.innerHTML = snapshotReportHTML +
    '<strong>This artifact wants to contact these origins.</strong>' +
    '<div style="color:#888;margin:4px 0 8px">The most secure option will <em>always</em> be to disable all external origins. Use your own discretion when allowing access to the listed networks below. This is a static scan and may not include every origin the application needs to work.</div>' +
    rows +
    '<div class="upload-row">' +
    '<button class="btn btn-sm" onclick="approveOrigins(\'' + id + '\')">Approve selected &amp; enable</button>' +
    '<button class="btn btn-sm" style="background:#888" onclick="finishIngest(\'' + id + '\')">Keep all blocked</button>' +
    '</div>';
}

// approveOrigins writes the user-selected origins to the artifact's allowlist.
async function approveOrigins(id) {
  const selected = Array.from(document.querySelectorAll('.al-origin:checked')).map(c => c.value);
  const status = document.getElementById('status');
  status.textContent = 'Applying…';
  const r = await fetch('/api/artifacts/' + id, {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({network_allowlist: selected})
  });
  if (!r.ok) { status.textContent = '✗ Failed to update allowlist'; return; }
  finishIngest(id);
}

function finishIngest(id) {
  const status = document.getElementById('status');
  status.textContent = '✓ Saved — ';
  const link = document.createElement('a');
  link.href = '/artifacts/' + id;
  link.textContent = 'View artifact';
  status.appendChild(link);
  if (snapshotReportHTML) {
    // Keep the snapshot report readable (dropping any approval controls)
    // instead of auto-reloading it away.
    const scanDiv = document.getElementById('scan-result');
    scanDiv.style.display = 'block';
    scanDiv.innerHTML = snapshotReportHTML;
    return;
  }
  setTimeout(() => location.reload(), 1200);
}

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
    const r = await fetch('/api/tags/' + encodeURIComponent(btn.dataset.tagId) + '/artifacts/' + encodeURIComponent(btn.dataset.artifactId), {
      method: 'DELETE',
      headers: {'Authorization':'Bearer '+TOKEN}
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
  const r = await fetch('/api/tags/' + encodeURIComponent(editingTagId), {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
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
  const r = await fetch('/api/tags/' + encodeURIComponent(editingTagId), {
    method: 'DELETE',
    headers: {'Authorization':'Bearer '+TOKEN}
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
    const created = await fetch('/api/tags', {
      method: 'POST',
      headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
      body: JSON.stringify({name: name, color: color})
    });
    const data = await created.json().catch(function() { return {}; });
    if (!created.ok) { setModalError('tag-add', data.error || 'Failed to create tag.'); return; }
    tagId = data.id;
  }

  const attached = await fetch('/api/tags/' + encodeURIComponent(tagId) + '/artifacts/' + encodeURIComponent(addingArtifactId), {
    method: 'POST',
    headers: {'Authorization':'Bearer '+TOKEN}
  });
  if (!attached.ok) {
    const data = await attached.json().catch(function() { return {}; });
    setModalError('tag-add', data.error || 'Failed to attach tag.');
    return;
  }
  location.reload();
});
