import { apiFetch } from './api-module.js';

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
window.pendingIngestArtifactId = '';
let ingestPending = false;
let createdArtifactId = '';

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
  if (ingestPending || createdArtifactId) return;
  clearFeedback();
  currentMode = mode;
  document.querySelectorAll('.route[data-mode]').forEach(function(tile) {
    const on = tile.dataset.mode === mode;
    tile.classList.toggle('is-selected', on);
    tile.setAttribute('aria-pressed', on ? 'true' : 'false');
    if (on) document.getElementById('route-description').textContent = tile.querySelector('.route-note').textContent;
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
  clearFeedback();
  if (!description.value.trim()) { showFieldError(description, 'Describe what you want the tool to do.'); return; }
  const brief = {
    title: document.getElementById('agent-title').value.trim(),
    description: description.value.trim()
  };
  try {
    sessionStorage.setItem(BRIEF_KEY, JSON.stringify(brief));
  } catch (e) {
    // Storage refused (private mode, blocked site data, quota). The agent page
    // still opens; it opens with an empty composer instead of the brief. Clear
    // the key on the way out: a brief an earlier hand-off left behind is the
    // one agent.js would otherwise consume, so a failed write would start the
    // session on somebody's previous description rather than on none.
    try { sessionStorage.removeItem(BRIEF_KEY); } catch (e2) { /* nothing left to do */ }
  }
  location.href = '/agent';
}

// errorMessage extracts something readable from a failed response, whatever
// shape it came in. JSON {error: …} is preferred; the API's http.Error path
// sends plain text; and a body that is empty or unreadable falls back to the
// status line, which at least names what happened.
async function errorMessage(resp) {
  let text = '';
  try {
    text = (await resp.text()).trim();
  } catch (err) {
    return resp.statusText || ('HTTP ' + resp.status);
  }
  if (text.startsWith('{')) {
    try {
      const parsed = JSON.parse(text);
      if (parsed.error) return parsed.error;
    } catch (err) { /* not JSON after all; fall through to the text */ }
  }
  return text || resp.statusText || ('HTTP ' + resp.status);
}

function showFieldError(field, message) {
  const error = document.getElementById(field.id + '-error');
  error.textContent = message;
  error.hidden = false;
  field.setAttribute('aria-invalid', 'true');
  const editable = field.nextElementSibling?.querySelector('.cm-content') || field;
  editable.setAttribute('aria-invalid', 'true');
  editable.focus();
}

function clearFeedback() {
  for (const field of document.querySelectorAll('[aria-invalid]')) field.removeAttribute('aria-invalid');
  for (const error of document.querySelectorAll('.field-error')) error.hidden = true;
  const status = document.getElementById('status');
  status.textContent = '';
  status.classList.remove('is-error');
}

function showIngestError(message) {
  const status = document.getElementById('status');
  status.textContent = message;
  status.classList.add('is-error');
}

function lockIngest(pending) {
  ingestPending = pending;
  const panel = document.getElementById('ingest-panel');
  panel.setAttribute('aria-busy', String(pending));
  for (const field of panel.querySelectorAll('.ingest-field input,.ingest-field textarea,#snapshot-toggle')) field.disabled = pending || !!createdArtifactId;
  const source = panel.querySelector('.cm-content');
  if (source) source.contentEditable = String(!pending && !createdArtifactId);
  for (const control of panel.querySelectorAll('#network-approval button,#network-approval input')) control.disabled = pending;
  for (const control of document.querySelectorAll('.route, #ingest-panel .ingest-foot button')) control.disabled = pending || !!createdArtifactId;
}

async function ingest() {
  if (ingestPending || createdArtifactId) return;
  clearFeedback();
  const title = document.getElementById('title').value.trim();
  const status = document.getElementById('status');
  const scanDiv = document.getElementById('scan-result');
  scanDiv.style.display = 'none';

  let payload;
  if (currentMode === 'url') {
    const url = document.getElementById('url-input').value.trim();
    if (!url) { showFieldError(document.getElementById('url-input'), 'Enter a URL first.'); return; }
    try {
      if (!['http:', 'https:'].includes(new URL(url).protocol)) throw new Error('protocol');
    } catch { showFieldError(document.getElementById('url-input'), 'Enter a full http:// or https:// page URL.'); return; }
    const snapshot = document.getElementById('snapshot-toggle').checked;
    payload = {title: title || '', url, snapshot, network_allowlist: []};
  } else {
    const body = document.getElementById('body').value.trim();
    if (!body) { showFieldError(document.getElementById('body'), 'Paste HTML first.'); return; }
    payload = {title: title || 'Untitled', body, network_allowlist: []};
  }

  status.textContent = currentMode === 'url' && payload.snapshot
    ? 'Fetching the page and its files…' : 'Adding artifact…';
  lockIngest(true);
  try {
    let resp;
    try {
      resp = await apiFetch('/api/artifacts', {
        method: 'POST',
        body: JSON.stringify(payload)
      });
    } catch (err) {
      // The request never completed — offline, DNS, a dropped connection. Say so
      // rather than leaving the last progress message on screen, which reads as a
      // hang and is indistinguishable from a slow snapshot.
      showIngestError('Couldn’t reach the server. Check your connection and try again.');
      return;
    }

    // The failure branch has to come before parsing, and parsing has to tolerate
    // a non-JSON body. The API's error path is http.Error, which writes plain
    // text: calling resp.json() first threw on every failed ingest, and with
    // nothing catching it the status stayed on 'Fetching page and snapshotting
    // assets…' forever. A dead URL looked identical to a slow one.
    if (!resp.ok) {
      showIngestError('Couldn’t add the artifact: ' + (await errorMessage(resp)));
      return;
    }

    let data;
    try {
      data = await resp.json();
    } catch (err) {
      showIngestError('Couldn’t read the server’s response. Try again.');
      return;
    }

    const id = data.artifact.id;
    createdArtifactId = id;
    const footprint = data.network_footprint || [];
    snapshotReportHTML = renderSnapshotReport(data.snapshot);
    snapshotNeedsAttention = snapshotHasProblems(data.snapshot);
    if (footprint.length > 0 || (data.network_warnings || []).length > 0) {
      // The artifact is saved but network-blocked (CSP connect-src 'none').
      // Pause here for explicit approval — nothing is added to the allowlist,
      // and no origin gains network access, until the user decides.
      status.textContent = 'Saved. Review network access before continuing.';
      showApproval(id);
      return;
    }
    finishIngest(id);
  } finally {
    lockIngest(false);
  }
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
    return '<div class="snapshot-report"><strong>Couldn’t save the page’s files</strong>. The original page was saved, but its files still need network access.' +
      (rep.error ? '<div class="snapshot-bad">' + esc(rep.error) + '</div>' : '') +
      '</div>';
  }
  const urls = rep.vendored_urls || [];
  let html = '<div class="snapshot-report"><strong>Saved files</strong>' +
    '<div class="snapshot-line">' + urls.length + ' asset' + (urls.length === 1 ? '' : 's') +
    ' saved with the artifact (' + fmtBytes(rep.vendored_bytes || 0) + ').</div>';
  if (urls.length > 0) {
    html += '<details class="snapshot-line"><summary>Saved assets</summary>' +
      '<ul class="snapshot-list">' +
      urls.map(u => '<li><code>' + esc(u) + '</code></li>').join('') +
      '</ul></details>';
  }
  const fails = rep.failures || [];
  if (fails.length > 0) {
    html += '<div class="snapshot-bad">' + fails.length + ' asset' + (fails.length === 1 ? '' : 's') +
      ' could not be saved. These files still need network access:</div>' +
      '<ul class="snapshot-list">' +
      fails.map(f => '<li><code>' + esc(f.ref) + '</code> — ' + esc(f.kind) +
        (f.detail ? ' (' + esc(f.detail) + ')' : '') + '</li>').join('') +
      '</ul>';
  }
  if ((rep.residual_origins || []).length === 0) {
    html += '<div class="snapshot-good">The scan found no remaining external references.</div>';
  }
  return html + '</div>';
}

// showApproval presents the scanned origins for explicit approval. Origins are
// blocked until the user approves them; nothing is written to the allowlist here.
// The snapshot report, when one exists, is shown above the approval controls.
function showApproval(id) {
  const scanDiv = document.getElementById('scan-result');
  window.pendingIngestArtifactId = id;
  scanDiv.style.display = 'block';
  document.getElementById('snapshot-report').innerHTML = snapshotReportHTML;
  document.body.dispatchEvent(new CustomEvent('exhibit:network-approval-changed'));
}

// approveOrigins writes the user-selected origins to the artifact's allowlist.
async function approveOrigins(id) {
  if (ingestPending) return;
  const selected = Array.from(document.querySelectorAll('.al-origin:checked')).map(c => c.value);
  const status = document.getElementById('status');
  status.classList.remove('is-error');
  status.textContent = 'Applying…';
  lockIngest(true);
  try {
    const response = await apiFetch('/api/artifacts/' + id, {
      method: 'PATCH',
      body: JSON.stringify({network_allowlist: selected})
    });
    if (!response.ok) throw new Error(await errorMessage(response));
    finishIngest(id);
  } catch (error) {
    showIngestError('Couldn’t update allowed sites: ' + error.message);
  } finally {
    lockIngest(false);
  }
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
    document.getElementById('snapshot-report').innerHTML = snapshotReportHTML;
    document.getElementById('network-approval').replaceChildren();
    return;
  }
  status.textContent = '✓ Saved — opening artifact…';
  location.href = href;
}

// Feature handlers stay local to this module. The network approval partial is
// also used by the classic editor and retains its explicit callback bridge.
window.approveOrigins = approveOrigins;
window.finishIngest = finishIngest;
for (const route of document.querySelectorAll('.route[data-mode]')) {
  route.addEventListener('click', () => setMode(route.dataset.mode));
}
document.getElementById('agent-panel').addEventListener('submit', startAgent);
document.getElementById('ingest-panel').addEventListener('submit', event => {
  event.preventDefault();
  ingest();
});
for (const field of document.querySelectorAll('#agent-description,#body,#url-input')) {
  field.addEventListener('input', () => { if (!ingestPending && !createdArtifactId) clearFeedback(); });
}
