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

// --- CodeMirror islands ----------------------------------------------------
// Both source fields get the same editor: the artifact's own body and its
// gallery widget are both single-file HTML documents, and there is no reason
// one of them should be a bare textarea. The editor keeps textarea.value in
// sync, so the save paths below are oblivious to it — and if the bundle failed
// to load, both plain textareas still work. Line wrapping stays off so
// long/deeply-nested lines scroll horizontally instead of reflowing.
//
// Mounting is deferred until a panel is actually open. CodeMirror measures the
// DOM when it is constructed, and a closed <details> is display:none, so
// mounting into one yields an editor with zero-width gutters and a misplaced
// cursor when it is later revealed. Waiting for the first open sidesteps the
// whole problem — and a panel the user never opens costs nothing.
const editors = {};

function mountEditorWhenOpen(panelID, textareaID) {
  const panel = document.getElementById(panelID);
  const textarea = document.getElementById(textareaID);
  if (!panel || !textarea || !window.ArtifactEditor) return;
  function mount() {
    if (editors[textareaID] || !panel.open) return;
    editors[textareaID] = ArtifactEditor.mount(textarea);
  }
  panel.addEventListener('toggle', mount);
  mount(); // a panel rendered open (artifact source) mounts right away
}

// Replaces a field's contents from code. The textarea alone is not enough once
// an editor is mounted over it: the sync only runs editor -> textarea, so a
// bare textarea.value assignment would leave the visible document stale.
function setSource(textareaID, text) {
  const textarea = document.getElementById(textareaID);
  if (textarea) textarea.value = text;
  const view = editors[textareaID];
  if (view) {
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
  }
}

mountEditorWhenOpen('source-panel', 'body');
mountEditorWhenOpen('widget-panel', 'widget-src');

// The capability popover links here as /edit#security-panel, and a fragment
// pointing AT a <details> does not open it — the visitor would land on a
// collapsed panel having just clicked "Manage in allowlist settings". Open
// whichever panel the hash names.
(function() {
  const target = location.hash ? document.querySelector(location.hash) : null;
  if (target && target.tagName === 'DETAILS') {
    target.open = true;
    target.scrollIntoView({ block: 'nearest' });
  }
})();

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

// --- gallery widget panel (av-fafu) ----------------------------------------
// The widget is a separate document with its own endpoint, so it saves on its
// own buttons rather than riding the artifact's Save. On success the panel
// fires exhibit:widget-saved and htmx re-fetches the cardWidget fragment into
// the preview slot — no page reload (which would drop the editor buffer) and
// no markup rebuilt here that the template already owns.
(function() {
  const src = document.getElementById('widget-src');
  const status = document.getElementById('widget-status');
  const saveBtn = document.getElementById('widget-save');
  const removeBtn = document.getElementById('widget-remove');
  const generateBtn = document.getElementById('widget-generate');
  if (!src) return;

  function refreshPreview() {
    document.body.dispatchEvent(new CustomEvent('exhibit:widget-saved'));
  }

  // The summary line is the only thing outside this panel that reports whether
  // the artifact has a widget, so keep it honest as the panel changes it.
  function setHasWidget(has) {
    const summary = document.getElementById('widget-summary-text');
    if (summary) summary.textContent = has ? 'custom tile' : 'default tile';
    const label = document.getElementById('widget-generate-label');
    if (label) label.textContent = has ? 'Regenerate' : 'Generate widget';
  }

  saveBtn.addEventListener('click', async function() {
    const body = src.value.trim();
    if (!body) { status.textContent = 'Nothing to save — the source is empty.'; return; }
    status.textContent = 'Saving…';
    try {
      const r = await fetch('/api/artifacts/' + ID + '/widget', {
        method: 'PUT',
        headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
        body: JSON.stringify({body: body})
      });
      if (!r.ok) {
        status.textContent = '✗ ' + ((await r.text().catch(() => '')).trim() || r.statusText);
        return;
      }
      const data = await r.json();
      // A widget shares the artifact's allowlist, so an origin missing from it
      // is already blocked at render — a fact to report, not a pending
      // approval. The allowlist panel above is where it would be granted.
      status.textContent = (data.unapproved_origins || []).length
        ? '✓ Saved — but ' + data.unapproved_origins.join(', ') + ' is not on the allowlist and will be blocked.'
        : '✓ Saved';
      setHasWidget(true);
      refreshPreview();
    } catch (e) {
      status.textContent = '✗ ' + e.message;
    }
  });

  removeBtn.addEventListener('click', async function() {
    if (!confirm('Remove this artifact’s widget? Its card falls back to the default tile.')) return;
    status.textContent = 'Removing…';
    try {
      const r = await fetch('/api/artifacts/' + ID + '/widget', {
        method: 'DELETE',
        headers: {'Authorization':'Bearer '+TOKEN}
      });
      if (!r.ok) {
        status.textContent = '✗ ' + ((await r.text().catch(() => '')).trim() || r.statusText);
        return;
      }
      setSource('widget-src', '');
      status.textContent = '✓ Removed';
      setHasWidget(false);
      refreshPreview();
    } catch (e) {
      status.textContent = '✗ ' + e.message;
    }
  });

  // --- Generate with the agent ---------------------------------------------
  // The button carries no prompt: POST returns a session id, and the whole
  // instruction lives server-side. Progress comes from the session's ordinary
  // SSE stream — the same route and the same exhibit_widget_saved event the
  // chat surface uses — so this adds no streaming machinery of its own and the
  // request never hangs waiting on a model.
  if (generateBtn && !generateBtn.disabled) {
    // An agent turn is slow but not unbounded; give up rather than spin forever.
    const GENERATE_TIMEOUT_MS = 180000;

    generateBtn.addEventListener('click', async function() {
      generateBtn.disabled = true;
      saveBtn.disabled = true;
      status.textContent = 'Generating… the agent is reading the artifact and writing its tile.';

      let events = null, timer = null;
      function finish(message) {
        if (timer) clearTimeout(timer);
        if (events) events.close();
        generateBtn.disabled = false;
        saveBtn.disabled = false;
        status.textContent = message;
      }

      try {
        const r = await fetch('/api/artifacts/' + ID + '/widget/generate', {
          method: 'POST',
          headers: {'Authorization':'Bearer '+TOKEN}
        });
        if (!r.ok) {
          const data = await r.json().catch(() => ({}));
          finish('✗ ' + (data.error || r.statusText));
          return;
        }
        const sessionId = (await r.json()).session_id;

        // EventSource cannot set headers, so this route takes the same bearer
        // token as ?token= — the existing contract, not a new one.
        events = new EventSource('/api/agent/sessions/' + encodeURIComponent(sessionId) +
          '/events?token=' + encodeURIComponent(TOKEN));
        timer = setTimeout(function() {
          finish('✗ Timed out waiting for the agent. Try again, or write the widget by hand.');
        }, GENERATE_TIMEOUT_MS);

        events.onmessage = async function(e) {
          let ev;
          try { ev = JSON.parse(e.data); } catch (err) { return; }

          if (ev.type === 'exhibit_widget_saved') {
            // Pull the saved source back into the editor so the user can see
            // and edit what the agent wrote, not just its rendered tile.
            try {
              const got = await fetch('/api/artifacts/' + ID + '/widget', {
                headers: {'Authorization':'Bearer '+TOKEN}
              });
              if (got.ok) setSource('widget-src', (await got.json()).body || '');
            } catch (err) { /* the tile still rendered; the source can be re-read */ }
            setHasWidget(true);
            refreshPreview();
            finish((ev.unapproved || []).length
              ? '✓ Generated — but ' + ev.unapproved.join(', ') + ' is not on the allowlist and will be blocked.'
              : '✓ Generated');
            // One-shot session: the work is done, so don't leave a subprocess
            // alive until the idle reaper gets to it.
            fetch('/api/agent/sessions/' + encodeURIComponent(sessionId), {
              method: 'DELETE', headers: {'Authorization':'Bearer '+TOKEN}
            }).catch(function(){});
            return;
          }
          // The turn ended without a widget: the model declined, errored, or
          // ran out of room. Say so rather than leaving the button spinning.
          if (ev.type === 'exhibit_session_closed') {
            finish('✗ The agent finished without saving a widget.');
          }
        };
        events.onerror = function() {
          finish('✗ Lost the connection to the agent.');
        };
      } catch (e) {
        finish('✗ ' + e.message);
      }
    });
  }
})();
