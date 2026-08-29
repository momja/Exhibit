/* Add-artifact (ingest) page script. Served from the app origin at
 * /assets/gallery/new.js. The page's inline bootstrap <script> defines the
 * per-request global this file reads before it loads:
 *   TOKEN / READ_ONLY - this visitor's API credential, decided server-side
 *                       per request (av-5imk); spent via api.js's apiFetch
 *
 * This is the ingest half of what used to be the gallery index script
 * (av-qo0j); the library page kept search, tags and modals. Ingest behavior is
 * unchanged: persist first (network-inert), surface the scanned footprint for
 * explicit approval, then PATCH the approved allowlist.
 *
 * nw-d1dd added a third mode, agent, and made it the one the page opens on.
 * It shares the tile switch and nothing else: it submits a brief to the agent
 * surface rather than an artifact to the API, so it has its own panel, its own
 * footer and its own submit.
 */

let currentMode = 'agent';

// The CodeMirror island mounts the first time Paste HTML is selected, not at
// load. The page no longer opens on paste, and an editor mounted inside a
// display:none wrapper measures itself as zero-height and stays that way until
// something forces a remeasure. The editor keeps textarea.value in sync, so
// ingest() below still reads the field — and if the bundle failed to load, the
// plain textarea still works.
let editorMounted = false;
function mountEditor() {
  if (editorMounted || !window.ArtifactEditor) return;
  ArtifactEditor.mount(document.getElementById('body'));
  editorMounted = true;
}

// The route tiles are the page's mode switch: which panel is on screen and,
// for the two ingest modes, what that panel asks for.
function setMode(mode) {
  currentMode = mode;
  document.querySelectorAll('.route[data-mode]').forEach(function(tile) {
    const on = tile.dataset.mode === mode;
    tile.classList.toggle('is-selected', on);
    tile.setAttribute('aria-pressed', on ? 'true' : 'false');
  });
  const ingesting = mode !== 'agent';
  document.getElementById('agent-panel').hidden = ingesting;
  document.getElementById('ingest-panel').hidden = !ingesting;
  // Hide the field *wrappers*, not the controls: the mounted CodeMirror sets
  // .cm-editor{display:flex!important} on itself and would win against an
  // inline display:none of its own.
  document.getElementById('source-field').hidden = mode !== 'paste';
  document.getElementById('url-field').hidden = mode !== 'url';
  // Snapshot is URL-only and must stay that way: the API rejects `snapshot`
  // without a `url` (400 "snapshot requires a source url") because the
  // vendorer needs an absolute http(s) base to resolve references against.
  // Hiding the control here is the visible half of that rule; ingest() reads
  // the checkbox only on the url branch, so paste mode cannot send it at all.
  document.getElementById('snapshot-row').hidden = mode !== 'url';
  if (mode === 'paste') mountEditor();
}

// --- Agent brief -----------------------------------------------------------
// The brief crosses to /agent in sessionStorage rather than in a query string.
// These are the user's own words about what they are building, and a URL is
// copied into this server's request log, the operator's proxy access log and
// the browser's history — the same reasoning that keeps the SSE credential out
// of a URL (av-rgp1), applied to content instead of a secret. Same origin,
// same tab, and agent.js consumes it exactly once on boot.
const BRIEF_KEY = 'exhibit:agent-brief';

// startAgent collects the brief and hands off. It is the form's submit
// handler, so the browser has already enforced `required` on the description
// by the time it runs; the check below is what a submit triggered any other
// way falls back on.
function startAgent(event) {
  if (event) event.preventDefault();
  const description = document.getElementById('agent-description');
  if (!description.value.trim()) { description.focus(); return; }
  const brief = {
    title: document.getElementById('agent-title').value.trim(),
    description: description.value.trim()
  };
  try {
    sessionStorage.setItem(BRIEF_KEY, JSON.stringify(brief));
  } catch (e) {
    // Storage refused (private mode, blocked site data). The agent page still
    // opens; it opens with an empty composer instead of the brief.
  }
  location.href = '/agent';
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

  const resp = await apiFetch('/api/artifacts', {
    method: 'POST',
    body: JSON.stringify(payload)
  });
  const data = await resp.json();
  if (!resp.ok) { status.textContent = 'Error: ' + (data.error || resp.statusText); return; }

  const id = data.artifact.id;
  const footprint = data.network_footprint || [];
  snapshotReportHTML = renderSnapshotReport(data.snapshot);
  snapshotNeedsAttention = snapshotHasProblems(data.snapshot);
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

// The current ingest's snapshot report, kept so it stays on screen through the
// approval step; snapshotNeedsAttention records whether it says anything the
// user must read before we navigate away from it (see finishIngest).
let snapshotReportHTML = '';
let snapshotNeedsAttention = false;

function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function fmtBytes(n) {
  if (n < 1024) return n + ' B';
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1048576).toFixed(1) + ' MB';
}

// A snapshot report is worth stopping for only when something did not vendor:
// the whole snapshot failed, or individual assets were left as live
// references. A clean report is a summary the detail page makes redundant.
function snapshotHasProblems(rep) {
  if (!rep) return false;
  return !rep.applied || (rep.failures || []).length > 0;
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
      (rep.error ? '<div class="snapshot-bad">' + esc(rep.error) + '</div>' : '') +
      '</div>';
  }
  const urls = rep.vendored_urls || [];
  let html = '<div class="snapshot-report"><strong>Snapshot report</strong>' +
    '<div class="snapshot-line">' + urls.length + ' asset' + (urls.length === 1 ? '' : 's') +
    ' vendored into the file (' + fmtBytes(rep.vendored_bytes || 0) + ').</div>';
  if (urls.length > 0) {
    html += '<details class="snapshot-line"><summary>Vendored assets</summary>' +
      '<ul class="snapshot-list">' +
      urls.map(u => '<li><code>' + esc(u) + '</code></li>').join('') +
      '</ul></details>';
  }
  const fails = rep.failures || [];
  if (fails.length > 0) {
    html += '<div class="snapshot-bad">' + fails.length + ' asset' + (fails.length === 1 ? '' : 's') +
      ' could not be inlined (reference kept, see origins below):</div>' +
      '<ul class="snapshot-list">' +
      fails.map(f => '<li><code>' + esc(f.ref) + '</code> — ' + esc(f.kind) +
        (f.detail ? ' (' + esc(f.detail) + ')' : '') + '</li>').join('') +
      '</ul>';
  }
  if ((rep.residual_origins || []).length === 0) {
    html += '<div class="snapshot-good">No residual network references — the artifact is fully self-contained.</div>';
  }
  return html + '</div>';
}

// showApproval presents the scanned origins for explicit approval. Origins are
// blocked until the user approves them; nothing is written to the allowlist here.
// The snapshot report, when one exists, is shown above the approval controls.
function showApproval(id, footprint) {
  const scanDiv = document.getElementById('scan-result');
  const rows = footprint.map(o =>
    '<label class="origin-row">' +
    '<input type="checkbox" class="al-origin" value="' + esc(o) + '" checked> ' +
    '<code>' + esc(o) + '</code></label>'
  ).join('');
  scanDiv.style.display = 'block';
  scanDiv.innerHTML = snapshotReportHTML +
    '<strong>This artifact wants to contact these origins.</strong>' +
    '<div class="scan-note">The most secure option will <em>always</em> be to disable all external origins. Use your own discretion when allowing access to the listed networks below. This is a static scan and may not include every origin the application needs to work.</div>' +
    rows +
    '<div class="scan-actions">' +
    '<button class="btn btn-sm" onclick="approveOrigins(\'' + id + '\')">Approve selected &amp; enable</button>' +
    '<button class="btn btn-sm btn-sec" onclick="finishIngest(\'' + id + '\')">Keep all blocked</button>' +
    '</div>';
}

// approveOrigins writes the user-selected origins to the artifact's allowlist.
async function approveOrigins(id) {
  const selected = Array.from(document.querySelectorAll('.al-origin:checked')).map(c => c.value);
  const status = document.getElementById('status');
  status.textContent = 'Applying…';
  const r = await apiFetch('/api/artifacts/' + id, {
    method: 'PATCH',
    body: JSON.stringify({network_allowlist: selected})
  });
  if (!r.ok) { status.textContent = '✗ Failed to update allowlist'; return; }
  finishIngest(id);
}

// The page's job ends when the artifact exists, so it hands off to the
// artifact's own page rather than sitting on a form the user is done with.
// The one exception is a snapshot report with something to read: navigating
// away would throw away the only account of what didn't vendor, so that stays
// on screen behind an explicit link.
function finishIngest(id) {
  const status = document.getElementById('status');
  const href = '/artifacts/' + id;
  if (snapshotNeedsAttention) {
    status.textContent = '✓ Saved — ';
    const link = document.createElement('a');
    link.href = href;
    link.textContent = 'View artifact';
    status.appendChild(link);
    const scanDiv = document.getElementById('scan-result');
    scanDiv.style.display = 'block';
    scanDiv.innerHTML = snapshotReportHTML;
    return;
  }
  status.textContent = '✓ Saved — opening artifact…';
  location.href = href;
}
