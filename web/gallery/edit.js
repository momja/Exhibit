/* Artifact edit page script. Served from the app origin at
 * /assets/gallery/edit.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads (and reassigns) before it loads:
 *   TOKEN             - API bearer token
 *   ID                - the artifact id
 *   allowlist         - approved network origins (mutable working copy)
 *   unapproved        - origins the current body references that carry no
 *                        decision at all (mutable working copy)
 *   blocked           - origins with an explicit block decision, i.e. a
 *                        "don't ask again" answer (mutable working copy).
 *                        They never reach the CSP; allowing one here moves it
 *                        into the allowlist and Save's PATCH upserts it as an
 *                        allow decision. Block decisions this page doesn't
 *                        touch are never cleared by Save (exhibit-x87).
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

// "Allow" in either the undecided or the blocked section moves the origin into
// the working allowlist; Save upserts it as an allow decision, which is also
// what overrides a previous block.
function bindAllowSection(containerId, take) {
  const el = document.getElementById(containerId);
  if (!el) return;
  el.addEventListener('click', function(e) {
    const btn = e.target.closest('[data-action="allow"]');
    if (!btn) return;
    const origin = btn.closest('.allowlist-row').dataset.origin;
    take(origin);
    if (!allowlist.includes(origin)) allowlist.push(origin);
    renderSecurityPanel();
  });
}
bindAllowSection('unapproved-rows', o => { unapproved = unapproved.filter(x => x !== o); });
bindAllowSection('blocked-rows', o => { blocked = blocked.filter(x => x !== o); });

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
function buildOriginRow(origin, actionLabel, action, note) {
  const row = document.createElement('div');
  row.className = 'allowlist-row';
  row.dataset.origin = origin;
  const code = document.createElement('code');
  code.textContent = origin;
  code.title = origin;
  row.appendChild(code);
  // A note labels the row's state (e.g. "blocked") so an explicit block
  // decision never renders identically to a merely undecided origin.
  if (note) {
    const tag = document.createElement('span');
    tag.className = 'text-sm muted';
    tag.textContent = note;
    row.appendChild(tag);
  }
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = action === 'remove' ? 'btn btn-sm btn-sec' : 'btn btn-sm';
  btn.dataset.action = action;
  btn.textContent = actionLabel;
  row.appendChild(btn);
  return row;
}

function renderOriginSection(containerId, origins, note) {
  const rows = document.getElementById(containerId);
  if (!rows) return; // section absent because it rendered empty server-side
  const heading = rows.previousElementSibling;
  rows.innerHTML = '';
  origins.forEach(o => rows.appendChild(buildOriginRow(o, 'Allow', 'allow', note)));
  const show = origins.length > 0;
  rows.style.display = show ? '' : 'none';
  if (heading) heading.style.display = show ? '' : 'none';
}

function renderSecurityPanel() {
  const alRows = document.getElementById('allowlist-rows');
  alRows.innerHTML = '';
  allowlist.forEach(o => alRows.appendChild(buildOriginRow(o, 'Remove', 'remove')));

  // Both sections offer "Allow"; the blocked one labels each row so an
  // explicit "don't ask again" reads differently from an undecided origin.
  // Each section (and its heading) hides once emptied.
  renderOriginSection('unapproved-rows', unapproved, null);
  renderOriginSection('blocked-rows', blocked, 'blocked');

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
  // An origin the user previously blocked ("don't ask again") is listed but
  // starts unchecked and labelled — re-approving it must be a deliberate act,
  // not the default the other origins get.
  const rows = footprint.map(o => {
    const isBlocked = blocked.includes(o);
    return '<label style="display:block;margin:4px 0">' +
      '<input type="checkbox" class="al-origin" value="' + esc(o) + '"' + (isBlocked ? '' : ' checked') + '> ' +
      '<code>' + esc(o) + '</code>' + (isBlocked ? ' <span class="text-sm muted">blocked</span>' : '') +
      '</label>';
  }).join('');
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
