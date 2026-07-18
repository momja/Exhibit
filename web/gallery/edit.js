/* Artifact edit page script. Served from the app origin at
 * /assets/gallery/edit.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads before it loads:
 *   TOKEN - API bearer token
 *   ID    - the artifact id
 */

// Mount the CodeMirror island over the body textarea. The editor keeps
// textarea.value in sync, so save() below is oblivious to it — and if the
// bundle failed to load, the plain textarea still works.
if (window.ArtifactEditor) {
  ArtifactEditor.mount(document.getElementById('body'));
}

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
    body: JSON.stringify({title: title || 'Untitled', body})
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
  const r = await fetch('/api/artifacts/' + ID, {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({network_allowlist: selected})
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
