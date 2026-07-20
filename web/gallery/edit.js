/* Artifact edit page script. Served from the app origin at
 * /assets/gallery/edit.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads (and reassigns) before it loads:
 *   TOKEN             - API bearer token
 *   ID                - the artifact id
 *   allowlist         - approved network origins (mutable working copy)
 *   unapproved        - origins the current body references but the
 *                        artifact hasn't approved (mutable working copy)
 *   downloadsApproved - persisted first-use download approval (mutable)
 *   clipboardApproved - persisted first-use clipboard approval (mutable)
 */

// Mount the CodeMirror island over the body textarea. The editor keeps
// textarea.value in sync, so save() below is oblivious to it — and if the
// bundle failed to load, the plain textarea still works. Line wrapping stays
// off so long/deeply-nested lines scroll horizontally instead of reflowing.
if (window.ArtifactEditor) {
  ArtifactEditor.mount(document.getElementById('body'));
}

// --- security panel: allowlist + capabilities (working copy, applied on Save) ---
// All edits here mutate the in-memory allowlist/unapproved/downloadsApproved/
// clipboardApproved copies and re-render; nothing hits the API until the one
// Save button fires the single PATCH below. This mirrors the panel's own
// posture summary, which is also derived from these working copies.

document.getElementById('dl-select').value = String(downloadsApproved);
document.getElementById('clip-select').value = String(clipboardApproved);
document.getElementById('dl-select').addEventListener('change', function(e) {
  downloadsApproved = e.target.value === 'true';
  renderSecurityPanel();
});
document.getElementById('clip-select').addEventListener('change', function(e) {
  clipboardApproved = e.target.value === 'true';
  renderSecurityPanel();
});

document.getElementById('allowlist-rows').addEventListener('click', function(e) {
  const btn = e.target.closest('[data-action="remove"]');
  if (!btn) return;
  const origin = btn.closest('.allowlist-row').dataset.origin;
  allowlist = allowlist.filter(o => o !== origin);
  renderSecurityPanel();
});

document.getElementById('unapproved-rows') && document.getElementById('unapproved-rows').addEventListener('click', function(e) {
  const btn = e.target.closest('[data-action="allow"]');
  if (!btn) return;
  const origin = btn.closest('.allowlist-row').dataset.origin;
  unapproved = unapproved.filter(o => o !== origin);
  if (!allowlist.includes(origin)) allowlist.push(origin);
  renderSecurityPanel();
});

document.getElementById('al-add-btn').addEventListener('click', function() {
  const inp = document.getElementById('al-add-input');
  const val = inp.value.trim();
  if (!val) return;
  if (!allowlist.includes(val)) allowlist.push(val);
  inp.value = '';
  renderSecurityPanel();
});
document.getElementById('al-add-input').addEventListener('keydown', function(e) {
  if (e.key === 'Enter') { e.preventDefault(); document.getElementById('al-add-btn').click(); }
});

// Builds one allowlist/unapproved row via createElement + textContent rather
// than interpolated markup — origins are user/scanner-controlled and can
// contain HTML metacharacters (av-tux9), same reasoning as detail.js's
// renderBadges().
function buildOriginRow(origin, actionLabel, action) {
  const row = document.createElement('div');
  row.className = 'allowlist-row';
  row.dataset.origin = origin;
  const code = document.createElement('code');
  code.textContent = origin;
  code.title = origin;
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = action === 'remove' ? 'btn btn-sm btn-sec' : 'btn btn-sm';
  btn.dataset.action = action;
  btn.textContent = actionLabel;
  row.append(code, btn);
  return row;
}

function renderSecurityPanel() {
  const alRows = document.getElementById('allowlist-rows');
  alRows.innerHTML = '';
  allowlist.forEach(o => alRows.appendChild(buildOriginRow(o, 'Remove', 'remove')));

  const unRows = document.getElementById('unapproved-rows');
  const unHeading = unRows && unRows.previousElementSibling;
  if (unRows) {
    unRows.innerHTML = '';
    unapproved.forEach(o => unRows.appendChild(buildOriginRow(o, 'Allow', 'allow')));
    const show = unapproved.length > 0;
    unRows.style.display = show ? '' : 'none';
    if (unHeading) unHeading.style.display = show ? '' : 'none';
  }

  document.getElementById('security-summary-text').textContent =
    allowlist.length + (allowlist.length === 1 ? ' origin' : ' origins') +
    ' · downloads: ' + (downloadsApproved ? 'always allow' : 'ask first') +
    ' · clipboard: ' + (clipboardApproved ? 'always allow' : 'ask first');
}
renderSecurityPanel();

async function save() {
  const title = document.getElementById('title').value.trim();
  const body  = document.getElementById('body').value;
  const status = document.getElementById('status');
  if (!body.trim()) { status.textContent = 'Body cannot be empty.'; return; }
  status.textContent = 'Saving…';
  document.getElementById('scan-result').style.display = 'none';
  const resp = await fetch('/api/artifacts/' + ID, {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({
      title: title || 'Untitled',
      body,
      network_allowlist: allowlist,
      downloads_approved: downloadsApproved,
      clipboard_approved: clipboardApproved
    })
  });
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    status.textContent = '✗ Error: ' + (data.error || resp.statusText);
    return;
  }
  status.textContent = '✓ Saved';
  // If the edited body changed the network footprint, the server re-ran the
  // scan and returned it. Re-run the explicit-approval flow so the user can
  // review/enable new origins — the same gate ingest uses. The allowlist is
  // never seeded from the scan; only the origins the user selects are written.
  const footprint = data.network_footprint || [];
  if (data.footprint_changed && footprint.length > 0) {
    showApproval(footprint);
    return;
  }
  setTimeout(() => { window.location.href = '/artifacts/' + ID; }, 500);
}

function showApproval(footprint) {
  const scanDiv = document.getElementById('scan-result');
  const rows = footprint.map(o =>
    '<label style="display:block;margin:4px 0">' +
    '<input type="checkbox" class="al-origin" value="' + esc(o) + '" checked> ' +
    '<code>' + esc(o) + '</code></label>'
  ).join('');
  scanDiv.style.display = 'block';
  scanDiv.innerHTML = '<strong>Edited artifact wants to contact these origins.</strong>' +
    '<div style="color:#888;margin:4px 0 8px">The most secure option will <em>always</em> be to disable all external origins. Origin approval is never automatic.</div>' +
    rows +
    '<div class="btn-row" style="margin-top:10px">' +
    '<button class="btn btn-sm" onclick="approveOrigins()">Approve selected &amp; enable</button>' +
    '<button class="btn btn-sm" style="background:#888" onclick="finishEdit()">Keep all blocked</button>' +
    '</div>';
}

async function approveOrigins() {
  const selected = Array.from(document.querySelectorAll('.al-origin:checked')).map(c => c.value);
  const status = document.getElementById('status');
  status.textContent = 'Applying…';
  // Union with the allowlist already written by save() above — a bare
  // overwrite would drop origins the security panel approved that the new
  // body's footprint doesn't happen to reference.
  const merged = Array.from(new Set([...allowlist, ...selected]));
  const r = await fetch('/api/artifacts/' + ID, {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({network_allowlist: merged})
  });
  if (!r.ok) { status.textContent = '✗ Failed to update allowlist'; return; }
  finishEdit();
}

function finishEdit() {
  document.getElementById('status').textContent = '✓ Saved — reloading…';
  setTimeout(() => { window.location.href = '/artifacts/' + ID; }, 400);
}

function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

async function deleteArtifact() {
  if (!confirm('Are you sure you want to delete this artifact? The action cannot be reversed and all data will be lost.')) return;
  const status = document.getElementById('status');
  status.textContent = 'Deleting…';
  try {
    const resp = await fetch('/api/artifacts/' + ID, {
      method: 'DELETE',
      headers: {'Authorization':'Bearer '+TOKEN}
    });
    if (!resp.ok) {
      const txt = await resp.text().catch(() => '');
      status.textContent = '✗ Error: ' + (txt.trim() || resp.statusText);
      return;
    }
    window.location.href = '/';
  } catch (e) {
    status.textContent = '✗ Error: ' + e.message;
  }
}
