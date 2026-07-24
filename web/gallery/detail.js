/* Artifact detail (viewer) page script. Served from the app origin at
 * /assets/gallery/detail.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads (and reassigns) before it loads:
 *   TOKEN              - API bearer token
 *   ID                 - the artifact id
 *   SOURCE_URL         - source URL for URL-ingested artifacts ('' otherwise;
 *                        the Update-from-source button only renders when set)
 *   downloadsApproved  - persisted first-use download approval (mutable)
 *   clipboardApproved  - persisted first-use clipboard approval (mutable)
 */

// Mobile actions sheet (av-g7n7): below 640px the toolbar is styled as a
// bottom sheet that this kebab slides up over a scrim. One body class drives
// both, and above the breakpoint the kebab and scrim are display:none — so
// nothing here can fire, and the class means nothing to the desktop layout.
const sheet = document.getElementById('actions-sheet');
const sheetToggle = document.getElementById('sheet-toggle');

function setSheetOpen(open) {
  document.body.classList.toggle('sheet-open', open);
  sheetToggle.setAttribute('aria-expanded', String(open));
  // Move focus with the sheet: into its first action on open, back to the
  // kebab on close, so the sheet is never dismissed out from under focus.
  if (open) {
    const first = sheet.querySelector('a,button');
    if (first) first.focus();
  } else if (sheet.contains(document.activeElement)) {
    sheetToggle.focus();
  }
}

sheetToggle.addEventListener('click', function() {
  setSheetOpen(!document.body.classList.contains('sheet-open'));
});
document.getElementById('sheet-scrim').addEventListener('click', function() {
  setSheetOpen(false);
});
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape' && document.body.classList.contains('sheet-open')) setSheetOpen(false);
});

// State bridge: the artifact runs in a sandboxed (opaque-origin) iframe and
// cannot call the API itself. Its storage shim posts state writes here; we
// forward them same-origin with the auth token. Validate the message shape and
// that it truly came from our artifact frame (e.origin is 'null' when sandboxed,
// so identity is established by the source window, not the origin string).
window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avState !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  fetch('/api/artifacts/' + encodeURIComponent(ID) + '/state', {
    method: 'PUT',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({ key: d.key, value: d.value })
  }).catch(function(){});
});

// Module-worker warning (av-yvtb): the sandboxed frame's opaque origin makes
// Chrome refuse to fetch a module worker's script, so a Worker({type:'module'})
// (e.g. ffmpeg.wasm 0.12) silently hangs forever on "Loading…" in the embedded
// preview — while it runs fine opened top-level, which has a real origin. The
// shim posts a diagnostic when it sees that case; reveal a non-blocking banner
// explaining it and offering the "Open in new tab" render URL. Debounced in the
// shim to the first occurrence, and we only reveal the banner once.
window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avModuleWorker !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  const banner = document.getElementById('module-worker-banner');
  if (banner) banner.hidden = false;
});

// The module-worker diagnostic usually fires at iframe load — possibly before
// this listener is attached, so the shim buffers it and replays on request.
// Announce readiness on every iframe load (targetOrigin '*' — the frame is
// opaque; the shim validates the ping came from our app origin) so any buffered
// diagnostic is delivered even when the worker was constructed before we listened.
(function() {
  const frame = document.querySelector('iframe');
  if (!frame) return;
  frame.addEventListener('load', function() {
    frame.contentWindow.postMessage({ __avHostReady: true }, '*');
  });
})();

// Download bridge: the sandboxed frame cannot download anything itself (the
// sandbox omits allow-downloads). The shim posts intercepted download
// attempts here — filename + transferred bytes, validated the same way as
// state messages. On the artifact's first attempt we prompt; the user's
// approval is persisted server-side (downloads_approved, via PATCH — the
// single write path) so it survives reloads and devices, and is revocable
// from the toolbar. Denial just drops the bytes; the artifact keeps running.
let pendingDownload = null;

window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avDownload !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  if (!(d.bytes instanceof ArrayBuffer)) return;
  const dl = {
    filename: String(d.filename || 'download'),
    mime: String(d.mime || 'application/octet-stream'),
    bytes: d.bytes
  };
  if (downloadsApproved) { triggerDownload(dl); return; }
  pendingDownload = dl;
  document.getElementById('dl-filename').textContent = dl.filename;
  document.getElementById('dl-modal').hidden = false;
});

// Reconstructs the transferred bytes as a Blob and downloads it via an
// app-origin anchor. The revoke is deferred so the browser has started the
// download before the object URL disappears.
function triggerDownload(dl) {
  const url = URL.createObjectURL(new Blob([dl.bytes], {type: dl.mime}));
  const a = document.createElement('a');
  a.href = url;
  a.download = dl.filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(function() { URL.revokeObjectURL(url); }, 10000);
}

// Shared capability-bridge approval: persists a first-use grant server-side
// via PATCH (the single write path). Downloads and clipboard both ride this.
// The viewer never surfaces a revoke control (that's now Edit-page-only,
// av-hwx2) — this only grants on the artifact's first attempt.
async function setCapabilityApproved(field, approved, label) {
  const st = document.getElementById('al-status');
  const r = await fetch('/api/artifacts/' + ID, {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({[field]: approved})
  }).catch(function() { return null; });
  if (!r || !r.ok) { st.textContent = '✗ Failed to update ' + label + ' permission'; return false; }
  return true;
}

async function setDownloadsApproved(approved) {
  if (!(await setCapabilityApproved('downloads_approved', approved, 'download'))) return false;
  downloadsApproved = approved;
  return true;
}

function closeDownloadModal() {
  document.getElementById('dl-modal').hidden = true;
  pendingDownload = null;
}

document.getElementById('dl-block').addEventListener('click', closeDownloadModal);
document.getElementById('dl-modal').addEventListener('click', function(e) {
  if (e.target.id === 'dl-modal') closeDownloadModal();
});
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape' && !document.getElementById('dl-modal').hidden) closeDownloadModal();
});
document.getElementById('dl-allow').addEventListener('click', async function() {
  const dl = pendingDownload;
  if (!(await setDownloadsApproved(true))) return;
  closeDownloadModal();
  if (dl) triggerDownload(dl);
});

// Clipboard bridge: the sandboxed frame's navigator.clipboard is denied by
// permissions policy, so the shim proxies readText/writeText here. Same
// first-use approval model as downloads (clipboard_approved, via PATCH). On
// approval the host performs the op on the app origin — which has clipboard
// access and, from the Allow click, transient user activation — and posts the
// result back into the frame, correlated by id. Denial rejects the shim's
// promise so the artifact sees a normal DOMException.
let pendingClip = null;

window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avClipboard !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  const op = d.op === 'read' ? 'read' : 'write';
  const req = { id: String(d.id), op: op, text: op === 'write' ? String(d.text == null ? '' : d.text) : null };
  if (clipboardApproved) { performClipboard(req); return; }
  pendingClip = req;
  document.getElementById('clip-direction').textContent = op === 'read' ? 'read' : 'write to';
  document.getElementById('clip-modal').hidden = false;
});

// Posts a clipboard result back into the sandbox frame. targetOrigin is '*'
// because the frame's origin is opaque; the payload is only what the artifact
// itself asked to read or write.
function replyClip(id, ok, text, error) {
  const frame = document.querySelector('iframe');
  if (!frame) return;
  frame.contentWindow.postMessage(
    { __avClipboardResult: true, id: id, ok: ok, text: text, error: error }, '*'
  );
}

async function performClipboard(req) {
  try {
    if (req.op === 'read') {
      const text = await navigator.clipboard.readText();
      replyClip(req.id, true, text);
    } else {
      await navigator.clipboard.writeText(req.text);
      replyClip(req.id, true);
    }
  } catch (err) {
    replyClip(req.id, false, undefined, (err && err.message) || 'Clipboard operation failed');
  }
}

async function setClipboardApproved(approved) {
  if (!(await setCapabilityApproved('clipboard_approved', approved, 'clipboard'))) return false;
  clipboardApproved = approved;
  return true;
}

// deny=true rejects the pending request so the artifact's clipboard call
// settles (with a DOMException) instead of hanging forever.
function closeClipModal(deny) {
  document.getElementById('clip-modal').hidden = true;
  if (deny && pendingClip) replyClip(pendingClip.id, false, undefined, 'Clipboard access denied');
  pendingClip = null;
}

document.getElementById('clip-block').addEventListener('click', function() { closeClipModal(true); });
document.getElementById('clip-modal').addEventListener('click', function(e) {
  if (e.target.id === 'clip-modal') closeClipModal(true);
});
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape' && !document.getElementById('clip-modal').hidden) closeClipModal(true);
});
document.getElementById('clip-allow').addEventListener('click', async function() {
  const req = pendingClip;
  if (!(await setClipboardApproved(true))) return;
  document.getElementById('clip-modal').hidden = true;
  pendingClip = null;
  if (req) performClipboard(req);
});

// "Update from source" — only reachable from the toolbar button, which the
// server renders only for URL-ingested artifacts (SOURCE_URL is '' otherwise).
async function refetchSource() {
  const warning = 'Re-fetch a fresh snapshot from the source URL?\n\n' +
    SOURCE_URL + '\n\n' +
    'This overwrites the stored content with whatever the URL returns now and ' +
    're-scans its network allowlist. It is NOT versioned and cannot be undone. ' +
    "The artifact's saved state/data may break if the new content changed.";
  if (!confirm(warning)) return;
  const st = document.getElementById('al-status');
  st.textContent = 'Fetching…';
  try {
    const r = await fetch('/api/artifacts/' + ID + '/refetch', {
      method: 'POST',
      headers: {'Authorization':'Bearer '+TOKEN}
    });
    if (!r.ok) {
      const txt = await r.text().catch(() => '');
      st.textContent = '✗ Error: ' + (txt.trim() || r.statusText);
      return;
    }
    st.textContent = '✓ Updated — reloading…';
    window.location.reload();
  } catch (e) {
    st.textContent = '✗ Error: ' + e.message;
  }
}
