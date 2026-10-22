import { modalFor } from './modal.js';
import { apiFetch, apiStateFetch } from './api-module.js';
import { networkPrompt } from './network-prompt.js';
/* Artifact detail (viewer) page script. Served from the app origin at
 * /assets/gallery/detail.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads (and reassigns) before it loads:
 *   TOKEN / READ_ONLY  - this visitor's API credential, decided server-side
 *                        per request (av-5imk); spent via api.js's apiFetch
 *   STATE_WRITABLE     - whether this visitor may write the artifact's saved
 *                        data. Not READ_ONLY inverted: a granted recipient is
 *                        both read-only and state-writable (av-v991)
 *   SHARED_STATE       - the artifact's share_state_mode is 'shared', so every
 *                        viewer is on the owner's rows and somebody else may be
 *                        writing them while this page is open
 *   ID                 - the artifact id
 *   SOURCE_URL         - source URL for URL-ingested artifacts ('' otherwise;
 *                        the Update-from-source button only renders when set)
 *   OPEN_URL           - the app-origin route that mints a fresh render token
 *                        and redirects to it: the "Open in new tab" destination,
 *                        where capture devices and location actually work
 *                        (av-mv3k, av-f446), and what the network prompt reloads
 *                        the frame through (av-kmwj)
 *   downloadsApproved  - persisted first-use download approval (mutable)
 *   clipboardApproved  - persisted first-use clipboard approval (mutable)
 *   linksApproved      - persisted first-use external-link approval (mutable)
 *   cameraApproved     - persisted first-use camera approval (mutable)
 *   microphoneApproved - persisted first-use microphone approval (mutable)
 *   geolocationApproved - persisted first-use location approval (mutable)
 */

// A frame navigation invalidates requests even though WindowProxy identity is
// retained by the browser. Replies must belong to the document that asked.
let previewGeneration = 0;
document.querySelector('iframe')?.addEventListener('load', () => {
  previewGeneration++;
  document.dispatchEvent(new CustomEvent('exhibit:preview-reloaded'));
});
function currentPreviewRequest(request) {
  return request?.frame === document.querySelector('iframe') && request.generation === previewGeneration;
}

// Whether this visitor may grant this artifact anything (av-awr4).
//
// Five first-use prompts on this page write per-artifact authority — downloads,
// clipboard, external links, the camera/microphone gate and the location gate,
// all through PATCH /api/artifacts/:id — and a sixth, the network prompt,
// through POST …/origins. Every one of them is the owner's decision to make, enforced by
// owner-scoped queries that have always refused anybody else. So a recipient
// who was offered them would click Allow and watch nothing happen, and would
// reasonably conclude the tool is broken rather than that it is not theirs.
//
// The answer is not to prompt: the artifact's request is settled the way a
// denial has always settled it, so the artifact sees the failure it already
// knows how to handle and nothing hangs. What the visitor is told about is the
// one case where silence would leave a tool visibly doing nothing with no
// reason anywhere — a blocked origin, which explainBlockedOrigin below turns
// into a sentence.
//
// READ_ONLY is the server's word for it, narrowed per artifact by
// pageCredentials.forArtifactOwnedBy. Reading it here rather than a flag of
// this script's own is what keeps "what the page offers" and "what the page may
// send" one decision: api.js refuses the same writes locally off the same
// value, so a prompt that slipped through this gate would still send nothing.
function mayApprove() {
  return !(typeof READ_ONLY === 'boolean' && READ_ONLY);
}

function needsHostReload() {
  return (typeof RELAY_ENABLED === 'boolean' && RELAY_ENABLED) ||
    (typeof CONNECTION_PROTECTED === 'boolean' && CONNECTION_PROTECTED);
}

// State bridge: the artifact runs in a sandboxed (opaque-origin) iframe and
// cannot call the API itself. Its storage shim posts state writes here; we
// forward them same-origin with the auth token. Validate the message shape and
// that it truly came from our artifact frame (e.origin is 'null' when sandboxed,
// so identity is established by the source window, not the origin string).
//
// These go through apiStateFetch rather than apiFetch, and that is the one
// difference av-v991 makes here. A recipient's page is READ_ONLY — the artifact
// is not theirs to change — but the data it saves while they use it is theirs,
// so the state write is the single exception carved out of that flag, gated on
// STATE_WRITABLE and refused again by the server. Before it, a granted tool
// booted with the state it had and lost everything typed into it afterwards.
//
// pendingStateWrites is read by the resync below: a refetch that lands between
// a setItem and its PUT would push the pre-write server copy back into the
// frame and fire a storage event undoing what the user just did. Last-write-
// wins settles it a moment later either way, but the flicker is visible and
// avoidable.
let pendingStateWrites = 0;

function sendStateWrite(path, opts, version) {
  pendingStateWrites++;
  return window.ExhibitState.write(apiStateFetch, path, opts, version)
    .then(function (res) {
      if (res && res.status === 409) {
        const banner = document.getElementById('state-changed-banner');
        if (banner) banner.hidden = false;
      }
    })
    .catch(function () {})
    .then(function () { pendingStateWrites--; });
}

window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avState !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  // URL construction lives in state-api.js so this and agent.js share one
  // definition — the ".." path-traversal bug (av-hh1o) had to be fixed in
  // three copies of it.
  if (d.op === 'clear') {
    sendStateWrite(window.ExhibitState.deleteURL(ID), { method: 'DELETE' }, d.version);
  } else if (d.op === 'delete') {
    sendStateWrite(window.ExhibitState.deleteURL(ID, d.key), { method: 'DELETE' }, d.version);
  } else if (d.op === 'set' || d.op === undefined) {
    // Only a recognized write reaches the API. An unknown op used to fall
    // through to this branch, so a future typo would silently become a write.
    sendStateWrite(window.ExhibitState.url(ID), {
      method: 'PUT',
      body: JSON.stringify({ key: d.key, value: d.value })
    }, d.version);
  }
});

// State resync (av-v991): the other direction of the same channel.
//
// The frame's cache was inlined when the render document was served, so it is a
// snapshot. On a SHARED_STATE artifact somebody else is writing the same rows;
// on any artifact the same person may be writing them from another device. The
// host refetches on the app origin with its own credential and posts the map
// into the frame, which applies it in place and fires real 'storage' events.
// There is no channel on the render origin — no SSE, no connect-src source, no
// write credential in that document — because there does not need to be one.
//
// What this buys, stated as narrowly as it deserves: the window in which two
// people's writes collide shrinks from "until somebody reloads" to seconds. It
// does not merge. Two people appending to one JSON blob under one key still
// lose an item, and no polling interval fixes that.
const STATE_RESYNC_INTERVAL_MS = 5000;
let stateResyncTimer = null;

function stateSyncEnabled() {
  // A visitor with no principal has no rows to keep in step, and their frame
  // was rendered with none inlined — polling would be a 401 every few seconds.
  return typeof STATE_WRITABLE === 'boolean' && STATE_WRITABLE;
}

async function resyncState() {
  if (!stateSyncEnabled() || pendingStateWrites > 0) return;
  const frame = document.querySelector('iframe');
  if (!frame || !frame.contentWindow) return;
  let resp;
  try {
    const options = typeof NAVIGATION_POLICY === 'string'
      ? { headers: { 'X-Exhibit-Relay-Policy': NAVIGATION_POLICY } } : undefined;
    resp = await apiFetch(window.ExhibitState.url(ID), options);
  } catch (err) {
    return;
  }
  if (!resp || !resp.ok) return;
  const state = await resp.json().catch(function () { return null; });
  if (!state || typeof state !== 'object') return;
  // targetOrigin '*' because the frame's origin is opaque and matches nothing
  // else. The payload is the artifact's own state, which the frame already
  // holds a snapshot of, so there is nothing here to leak to a frame that is
  // somehow not ours — and the frame checks e.origin against the app origin
  // before believing it.
  frame.contentWindow.postMessage({ __avStateSync: true, artifactId: ID, state: state }, '*');
}

// Two triggers, and they answer different questions.
//
// visibilitychange is the important one and applies to every artifact: a tab
// left open for an hour has no idea whether anything changed, and asking once
// when the person comes back is both cheap and exactly when they care. It is
// also the cross-device case — the phone wrote, the laptop was asleep.
//
// The interval only runs for a SHARED_STATE artifact, because only there is
// somebody else writing your rows while you watch. Polling every artifact view
// would spend a request every few seconds to discover nothing, forever.
document.addEventListener('visibilitychange', function () {
  if (document.visibilityState === 'visible') resyncState();
});

function startStateResync() {
  if (!stateSyncEnabled() || !SHARED_STATE || stateResyncTimer) return;
  stateResyncTimer = setInterval(function () {
    // A hidden tab is nobody watching; the visibilitychange handler catches it
    // up the moment it is looked at again.
    if (document.visibilityState !== 'hidden') resyncState();
  }, STATE_RESYNC_INTERVAL_MS);
}
startStateResync();

// The frame's answer to a resync, and the reason the reload control exists.
//
// The shim can guarantee getItem returns fresh data. It cannot re-render an
// artifact that read storage once at startup and never listened again, and most
// artifacts are written that way. So when a resync changed something and
// nothing in the frame was listening, the visitor is offered a reload — in the
// page's own chrome, never in the artifact's DOM, which the artifact could
// forge into a convincing button of its own.
//
// Never a silent reload. The render document is no-store, so a reload is a full
// re-fetch; sessionStorage is in-memory by design and would die with the frame;
// and so would everything the artifact holds in a variable rather than in
// storage, including a half-typed input.
window.addEventListener('message', function (e) {
  const d = e.data;
  if (!d || d.__avStateSynced !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  const banner = document.getElementById('state-changed-banner');
  if (!banner) return;
  // An artifact that listens updated itself, so saying "this tool's data
  // changed, reload it" would be noise reporting work already done.
  if (d.changed > 0 && !d.live) banner.hidden = false;
});

// Reloading goes through /open rather than reusing the frame's current src, for
// the reason the network prompt already does: that src carries a render token
// minted when this page loaded, and this banner is most likely to be pressed on
// a page that has been open a while.
const stateReloadBtn = document.getElementById('state-changed-reload');
if (stateReloadBtn) {
  stateReloadBtn.addEventListener('click', function () {
    if (needsHostReload()) { window.location.reload(); return; }
    const frame = document.querySelector('iframe');
    if (frame) frame.src = OPEN_URL + '?r=' + Date.now();
    document.getElementById('state-changed-banner').hidden = true;
  });
}

// Unsupported-capability warning (av-yvtb): some browser capabilities can't work
// inside the render frame's opaque-origin sandbox and fail silently rather than
// throwing — e.g. a module worker (Worker({type:'module'}), as ffmpeg.wasm 0.12
// uses) hangs forever on "Loading…" here while running fine opened top-level,
// which has a real origin. The shim posts a generic __avCapabilityWarning naming
// the capability; we reveal a non-blocking banner whose headline is a generic,
// reusable line and whose collapsed <details> describes the specific failure for
// support. The channel is capability-agnostic on purpose: new detections need
// only a CAPABILITY_COPY entry, not a new message type. Debounced in the shim to
// the first occurrence, and we only reveal the banner once.
//
// CAPABILITY_COPY maps a capability slug (set by the shim) to its support text:
// the sentence shown in <details> and the label for the optional resource string
// the shim attaches (e.g. a worker's script URL). Unknown slugs fall back to a
// generic description so an added detection is never left with empty details.
const CAPABILITY_COPY = {
  camera: {
    detail: 'This artifact asked for your camera. No browser can give a camera to the ' +
      "embedded preview — its sandboxed frame has no stable origin, and a device " +
      'permission is granted to an origin. Opening the artifact directly gives it a real ' +
      'origin, where it reaches the camera under the approval you already granted.',
    resourceLabel: 'Device'
  },
  microphone: {
    detail: 'This artifact asked for your microphone. No browser can give a microphone to ' +
      "the embedded preview — its sandboxed frame has no stable origin, and a device " +
      'permission is granted to an origin. Opening the artifact directly gives it a real ' +
      'origin, where it reaches the microphone under the approval you already granted.',
    resourceLabel: 'Device'
  },
  'camera-microphone': {
    detail: 'This artifact asked for your camera and microphone. No browser can give a ' +
      "capture device to the embedded preview — its sandboxed frame has no stable origin, " +
      'and a device permission is granted to an origin. Opening the artifact directly gives ' +
      'it a real origin, where it reaches them under the approval you already granted.',
    resourceLabel: 'Device'
  },
  // av-f446. Unlike the devices above, a browser *could* hand this frame a
  // location (Chromium does, given an allow= delegation), so the copy does not
  // say it can't. What it says is the actual reason: the grant would belong to
  // the library rather than to this artifact.
  geolocation: {
    detail: 'This artifact asked for your location. The embedded preview does not pass ' +
      "your location to artifacts: its sandboxed frame has no stable origin of its own, so " +
      "your browser's location permission could only be granted to the library, not to " +
      'this artifact. Opening the artifact directly gives it a real origin, where it reads ' +
      'your location under the approval you already granted.',
    resourceLabel: 'Request'
  },
  // av-kmwj. The artifact asked for an origin its allowlist already permits and
  // the browser blocked it anyway, which means the request did not end where it
  // started. CSP re-checks every redirect hop, and it deliberately reports the
  // URL the artifact asked for rather than the one it was sent to — a policy
  // must not become a way to probe where a cross-origin redirect leads. So
  // nobody on this page can name the origin that actually needs approving, and
  // the network prompt stays out of the way rather than offering to grant a
  // permission that is already granted.
  //
  // It carries its own headline because the shared one is wrong here: opening
  // the artifact directly does not fix a redirect, the policy is identical
  // there. What it does buy is a console that names the blocked URL.
  'redirected-origin': {
    headline: 'A request was blocked after being redirected to an unapproved origin.',
    detail: 'This artifact contacted a host you have already allowed, but that host ' +
      'forwarded the request somewhere else, and the destination is not on the ' +
      "allowlist. Browsers hide a redirect's destination from the page, so Exhibit " +
      'cannot offer it for approval — if you know the host it forwards to, add it in ' +
      "allowlist settings. Opening the artifact directly runs it under the same " +
      "policy, where your browser's Network panel shows the redirect and names " +
      'the destination.',
    resourceLabel: 'Blocked request'
  },
  'module-worker': {
    detail: "This artifact spawns a module worker (new Worker(url, { type: 'module' })), " +
      'which browsers refuse to run in the embedded preview because its sandboxed ' +
      'frame has no stable origin. Opening the artifact directly gives it a real ' +
      'origin, where it runs normally.',
    resourceLabel: 'Worker script'
  }
};
const CAPABILITY_COPY_FALLBACK = {
  detail: "This artifact uses a browser capability the embedded preview's sandboxed " +
    'frame cannot provide. Opening the artifact directly runs it in a normal ' +
    'browsing context, where the capability is available.',
  resourceLabel: 'Resource'
};
window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avCapabilityWarning !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  const banner = document.getElementById('capability-warning-banner');
  if (!banner) return;
  // Fill the collapsed details from the capability's copy. textContent, never
  // innerHTML: d.resource is artifact-controlled and must not be interpreted as
  // markup on the app origin.
  const detail = document.getElementById('capability-warning-detail');
  if (detail && !detail.textContent) {
    const copy = CAPABILITY_COPY[d.capability] || CAPABILITY_COPY_FALLBACK;
    detail.textContent = copy.detail;
    // Most capabilities share the banner's headline ("open it directly to run
    // it"), which is true of every failure the sandbox causes. A capability
    // that opening directly does NOT fix supplies its own rather than leaving
    // the shared line to say something false.
    const headline = document.getElementById('capability-warning-headline');
    if (headline && copy.headline) headline.textContent = copy.headline;
    if (d.resource) {
      const label = document.createElement('div');
      label.className = 'banner-detail-url';
      label.textContent = copy.resourceLabel + ': ';
      const code = document.createElement('code');
      code.textContent = d.resource;
      label.appendChild(code);
      detail.appendChild(label);
    }
  }
  banner.hidden = false;
});

// The preamble buffers what it cannot deliver yet — the module-worker
// diagnostic (av-yvtb) and CSP-violation reports (av-kmwj) both fire at frame
// load, before this page has attached its listeners — and flushes when the host
// announces itself. announceTo owns both halves of that handshake.
networkPrompt.announceTo(document.querySelector('iframe'));

// Download bridge: the sandboxed frame cannot download anything itself (the
// sandbox omits allow-downloads). The shim posts intercepted download
// attempts here — filename + transferred bytes, validated the same way as
// state messages. On the artifact's first attempt we prompt; the user's
// approval is persisted server-side (downloads_approved, via PATCH — the
// single write path) so it survives reloads and devices, and is revocable
// from its security settings. Denial just drops the bytes; the artifact keeps running.
let pendingDownload = null;
const downloadDialog = document.getElementById('dl-modal');
const downloadModal = modalFor(downloadDialog);
for (const event of ['modal:closed', 'modal:disconnected']) downloadDialog.addEventListener(event, () => { pendingDownload = null; });
document.addEventListener('exhibit:preview-reloaded', closeDownloadModal);

window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avDownload !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  if (!(d.bytes instanceof ArrayBuffer)) return;
  const dl = {
    filename: String(d.filename || 'download'),
    mime: String(d.mime || 'application/octet-stream'),
    bytes: d.bytes, frame, generation: previewGeneration
  };
  if (downloadsApproved) { triggerDownload(dl); return; }
  // Unapproved and not ours to approve: drop the bytes, exactly as a denial
  // does. The artifact keeps running and never learns the difference — which
  // is also all it learned before any of this existed, when the sandbox simply
  // swallowed the download.
  if (!mayApprove()) { pendingDownload = null; return; }
  pendingDownload = dl;
  document.getElementById('dl-filename').textContent = dl.filename;
  if (!downloadDialog.isConnected) { pendingDownload = null; return; }
  document.getElementById('dl-allow').disabled = false;
  downloadDialog.querySelector('[data-permission-status]').textContent = '';
  downloadModal.show(frame);
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
async function setCapabilityApproved(field, approved, label, stillPending = () => true) {
  const st = document.getElementById('al-status');
  const promptID = { downloads_approved: 'dl-modal', clipboard_approved: 'clip-modal', links_approved: 'link-modal', camera_approved: 'media-modal', microphone_approved: 'media-modal', geolocation_approved: 'geo-modal' }[field];
  const promptStatus = document.getElementById(promptID)?.querySelector('[data-permission-status]');
  if (promptStatus) promptStatus.textContent = '';
  const r = await apiFetch('/api/artifacts/' + ID, {
    method: 'PATCH',
    body: JSON.stringify({[field]: approved})
  }).catch(function() { return null; });
  if (!stillPending()) return false;
  if (!r || !r.ok) {
    const message = '✗ Failed to update ' + label + ' permission';
    st.textContent = message;
    if (promptStatus) promptStatus.textContent = message;
    return false;
  }
  return true;
}

async function setDownloadsApproved(approved, request) {
  if (!(await setCapabilityApproved('downloads_approved', approved, 'download', () => pendingDownload === request && currentPreviewRequest(request)))) return false;
  downloadsApproved = approved;
  return true;
}

function closeDownloadModal() {
  pendingDownload = null;
  downloadModal.close();
}

document.getElementById('dl-allow').addEventListener('click', async function() {
  const dl = pendingDownload;
  if (!dl || dl.saving) return;
  dl.saving = true;
  document.getElementById('dl-allow').disabled = true;
  const saved = await setDownloadsApproved(true, dl);
  dl.saving = false;
  if (pendingDownload !== dl || !currentPreviewRequest(dl)) return;
  document.getElementById('dl-allow').disabled = false;
  if (!saved) return;
  closeDownloadModal();
  triggerDownload(dl);
});

// Clipboard bridge: the sandboxed frame's navigator.clipboard is denied by
// permissions policy, so the shim proxies readText/writeText here. Same
// first-use approval model as downloads (clipboard_approved, via PATCH). On
// approval the host performs the op on the app origin — which has clipboard
// access and, from the Allow click, transient user activation — and posts the
// result back into the frame, correlated by id. Denial rejects the shim's
// promise so the artifact sees a normal DOMException.
let pendingClip = null;
const clipDialog = document.getElementById('clip-modal');
const clipModal = modalFor(clipDialog);
function denyPendingClip() {
  const req = pendingClip;
  pendingClip = null;
  if (req) replyClip(req, false, undefined, 'Clipboard access denied');
}
for (const event of ['modal:closed', 'modal:disconnected']) clipDialog.addEventListener(event, denyPendingClip);
document.addEventListener('exhibit:preview-reloaded', () => {
  pendingClip = null;
  clipModal.close();
});

window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avClipboard !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  const op = d.op === 'read' ? 'read' : 'write';
  const req = { id: String(d.id), op: op, text: op === 'write' ? String(d.text == null ? '' : d.text) : null, frame, generation: previewGeneration };
  if (clipboardApproved) { performClipboard(req); return; }
  // Not ours to approve. Reject rather than drop: a clipboard call is a
  // promise the artifact is awaiting, so silence here is a hang, where a
  // rejection is the DOMException a blocked clipboard has always thrown.
  if (!mayApprove() || !clipDialog.isConnected) { replyClip(req, false, undefined, 'Clipboard access denied'); return; }
  if (pendingClip?.id === req.id && currentPreviewRequest(pendingClip)) return;
  denyPendingClip();
  if (clipDialog.open) clipModal.close();
  pendingClip = req;
  document.getElementById('clip-direction').textContent = op === 'read' ? 'read' : 'write to';
  document.getElementById('clip-allow').disabled = false;
  clipDialog.querySelector('[data-permission-status]').textContent = '';
  clipModal.show(frame);
});

// Posts a clipboard result back into the sandbox frame. targetOrigin is '*'
// because the frame's origin is opaque; the payload is only what the artifact
// itself asked to read or write.
function replyClip(req, ok, text, error) {
  if (!currentPreviewRequest(req)) return;
  const frame = req.frame;
  frame.contentWindow.postMessage(
    { __avClipboardResult: true, id: req.id, ok: ok, text: text, error: error }, '*'
  );
}

// Start the browser operation in the Allow click itself. Awaiting the grant
// first would lose the gesture in browsers that require synchronous access.
async function clipboardResult(req) {
  try {
    if (req.op === 'read') return { ok: true, text: await navigator.clipboard.readText() };
    await navigator.clipboard.writeText(req.text);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: err?.message || 'Clipboard operation failed' };
  }
}
async function performClipboard(req) {
  const result = await clipboardResult(req);
  replyClip(req, result.ok, result.text, result.error);
}
async function setClipboardApproved(approved, request) {
  if (!(await setCapabilityApproved('clipboard_approved', approved, 'clipboard', () => pendingClip === request && currentPreviewRequest(request)))) return false;
  clipboardApproved = approved;
  return true;
}

document.getElementById('clip-allow').addEventListener('click', async function() {
  const req = pendingClip;
  if (!req || req.saving) return;
  req.saving = true;
  document.getElementById('clip-allow').disabled = true;
  const operation = clipboardResult(req);
  // Keep this exact request exclusive until both persistence and the browser
  // operation settle. Dismissal stays live and invalidates its eventual reply.
  const [saved, result] = await Promise.all([setClipboardApproved(true, req), operation]);
  req.saving = false;
  if (pendingClip !== req || !currentPreviewRequest(req)) return;
  document.getElementById('clip-allow').disabled = false;
  if (!saved) return;
  pendingClip = null;
  clipModal.close();
  replyClip(req, result.ok, result.text, result.error);
});

// Link navigation bridge (av-r0dk): the sandboxed frame cannot open external
// links itself — a target=_blank anchor is dropped without allow-popups, and a
// plain anchor would replace the iframe with an external page that usually
// refuses framing. The shim posts external link activations here; when approved
// we open the URL in a new tab from the app origin (the click's transient
// activation covers the postMessage roundtrip). Unapproved, we show the
// first-request confirmation (av-e3sj), naming the destination host, and open
// nothing until the user allows.
let pendingLink = null;
let linkRequestSequence = 0;
const linkDialog = document.getElementById('link-modal');
const linkModal = modalFor(linkDialog);
for (const event of ['modal:closed', 'modal:disconnected']) linkDialog.addEventListener(event, () => {
  pendingLink?.tab?.close();
  pendingLink = null;
});
document.addEventListener('exhibit:preview-reloaded', () => {
  linkRequestSequence++;
  linkModal.pending = false;
  closeLinkModal();
});

async function checkedLinkDestination(destination) {
  // The server grants this narrow exemption to uncredentialed views with no
  // connection data policies. A signed-in read-only recipient still checks
  // their own policy through the authenticated viewer endpoint.
  if (typeof PUBLIC_NAVIGATION === 'boolean' && PUBLIC_NAVIGATION) return destination;
  try {
    const response = await window.apiNavigationCheck(ID, destination,
      typeof NAVIGATION_POLICY === 'string' ? NAVIGATION_POLICY : '');
    const result = await response.json();
    if (!response.ok || typeof result.url !== 'string') throw new Error(result.error || 'This link is not permitted.');
    return result.url;
  } catch (error) {
    document.getElementById('al-status').textContent = error.message || 'Could not check this link. Try reopening the artifact.';
    return null;
  }
}

window.addEventListener('message', async function(e) {
  const d = e.data;
  if (!d || d.__avNavigate !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  let url;
  try {
    url = new URL(String(d.url));
  } catch (err) {
    return;
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return;
  if (!linksApproved && !mayApprove()) return;
  const sequence = ++linkRequestSequence;
  const destination = await checkedLinkDestination(url.href);
  if (!destination || sequence !== linkRequestSequence) return;
  if (linksApproved) {
    window.open(destination, '_blank', 'noopener,noreferrer');
    return;
  }
  // Not ours to approve: the destination is dropped, which is what a denial
  // does and what the sandbox did before the bridge existed.
  if (!mayApprove() || !linkDialog.isConnected || linkModal.pending) return;
  closeLinkModal();
  pendingLink = { url: destination, host: url.hostname, frame, generation: previewGeneration };
  document.getElementById('link-host').textContent = url.hostname;
  linkDialog.querySelector('[data-permission-status]').textContent = '';
  linkModal.show(frame);
});

// Persists the first-use grant, then lets the caller open the pending URL. The
// viewer is read-only (av-hwx2) — like downloads/clipboard, it only grants on
// the artifact's first attempt; the revoke control lives on the Edit page.
async function setLinksApproved(approved, request) {
  if (!(await setCapabilityApproved('links_approved', approved, 'link', () => pendingLink === request && currentPreviewRequest(request)))) return false;
  linksApproved = approved;
  return true;
}

// Denial just drops the destination and the artifact keeps running — nothing is
// persisted, mirroring downloads (denial drops, approval persists).
function closeLinkModal() {
  linkModal.close();
}

document.getElementById('link-allow').addEventListener('click', async function() {
  const link = pendingLink;
  if (!link) return;
  const opened = await linkModal.run(async () => {
    // run starts synchronously: claim and isolate the placeholder in the click.
    let tab;
    try { tab = window.open('', '_blank'); if (tab) tab.opener = null; }
    catch { tab?.close(); tab = null; }
    if (!tab) {
      linkDialog.querySelector('[data-permission-status]').textContent = 'Your browser blocked the new tab. Allow popups and try again.';
      return false;
    }
    link.tab = tab;
    const ok = await setLinksApproved(true, link);
    if (pendingLink !== link || !currentPreviewRequest(link)) { tab.close(); return false; }
    if (!ok || tab.closed) { tab.close(); link.tab = null; return false; }
    const destination = await checkedLinkDestination(link.url);
    if (!destination || pendingLink !== link || !currentPreviewRequest(link) || tab.closed) {
      tab.close(); link.tab = null; return false;
    }
    // Keep the blank tab's navigation from disclosing the Exhibit page URL.
    const policy = tab.document.createElement('meta');
    policy.name = 'referrer';
    policy.content = 'no-referrer';
    tab.document.head.appendChild(policy);
    tab.location = destination;
    link.tab = null;
    return true;
  });
  if (opened) closeLinkModal();
});

// Camera / microphone gate (av-mv3k): the only capability here that is decided
// but not delivered, because neither half of the usual trick is available.
// getUserMedia from the frame's opaque origin throws SecurityError before any
// permission is consulted (an allow="camera" delegation does not change that —
// it is refused even with Chrome's auto-accept flag set), and a camera
// MediaStreamTrack is not a transferable object in any shipping engine, so the
// download bridge's "acquire here, transfer the payload in" has nothing to
// transfer.
//
// So this owns the decision and hands the user the place the decision can be
// spent: the top-level render, a real origin where the artifact reaches the
// device itself under the very same approval (it builds that document's
// Permissions-Policy header). Unapproved, we prompt and — on Allow — persist
// and open it. Already approved, there is nothing to prompt for, so the frame
// raises the standard capability banner instead. Either way the artifact's
// getUserMedia rejects promptly rather than hanging on a stream that is never
// coming.
let pendingMedia = null;
const mediaDialog = document.getElementById('media-modal');
const mediaModal = modalFor(mediaDialog);
function denyPendingMedia() {
  const req = pendingMedia;
  pendingMedia = null;
  if (req) replyMedia(req, false, 'Permission denied', 'NotAllowedError');
}
for (const event of ['modal:closed', 'modal:disconnected']) mediaDialog.addEventListener(event, denyPendingMedia);
document.addEventListener('exhibit:preview-reloaded', () => {
  pendingMedia = null;
  mediaModal.close();
});

// Prose for the devices a request named, for the prompt.
function mediaDeviceLabel(req) {
  if (req.video && req.audio) return 'your camera and microphone';
  return req.video ? 'your camera' : 'your microphone';
}

window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avMedia !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  const req = { id: String(d.id), audio: !!d.audio, video: !!d.video, frame, generation: previewGeneration };
  if (!req.audio && !req.video) return;
  if (typeof CONNECTION_PROTECTED === 'boolean' && CONNECTION_PROTECTED) {
    replyMedia(req.id, false, 'Connected data must stay inside Exhibit; capture devices are unavailable in this preview.', 'NotAllowedError');
    return;
  }
  // Approved means approved for every device this request named: a camera-only
  // grant must still prompt when the artifact later asks for the microphone.
  const needsPrompt = (req.video && !cameraApproved) || (req.audio && !microphoneApproved);
  if (!needsPrompt) {
    // Nothing to decide — the grant is already there, it just cannot be spent
    // in this frame. Ask the frame to raise the banner, which offers the
    // top-level render, and settle the call.
    replyMedia(req, true, 'Capture devices are unavailable in the embedded preview; open the artifact directly',
      'NotSupportedError');
    return;
  }
  // Not ours to approve. Settle it as a denial — the same DOMException the
  // Block button produces — so getUserMedia rejects promptly instead of
  // hanging on a stream that is not coming.
  //
  // Nothing about the owner's approval would have handed over the visitor's
  // hardware anyway: it lifts Exhibit's own block, and the browser then asks
  // its own permission on the machine the camera is attached to. That second
  // gate is why an approved artifact is not a device grant, and why nothing
  // here needs to say more than "no".
  if (!mayApprove() || !mediaDialog.isConnected) {
    replyMedia(req, false, 'Permission denied', 'NotAllowedError');
    return;
  }
  // A second request arriving while the prompt is open displaces the first, so
  // settle the one being displaced rather than leaving its getUserMedia promise
  // pending forever — an artifact that never settles looks like a hang, which is
  // the failure mode this gate exists to remove.
  if (pendingMedia) replyMedia(pendingMedia, false, 'Permission denied', 'NotAllowedError');
  pendingMedia = req;
  document.getElementById('media-devices').textContent = mediaDeviceLabel(req);
  document.getElementById('media-icon').className = req.video ? 'ph ph-camera' : 'ph ph-microphone';
  mediaDialog.querySelector('[data-permission-status]').textContent = '';
  mediaModal.show(frame);
});

// Settles one pending getUserMedia inside the frame. targetOrigin is '*'
// because the frame's origin is opaque. banner asks the frame to raise the
// unsupported-capability banner (the frame owns that channel, so there is one
// path to it rather than two); every reply is a rejection, since there is no
// stream to return.
function replyMedia(req, banner, error, name) {
  if (!currentPreviewRequest(req)) return;
  const frame = req.frame;
  frame.contentWindow.postMessage(
    { __avMediaResult: true, id: req.id, ok: false, banner: banner, error: error, name: name }, '*'
  );
}

// Persists only the devices this request named (see the prompt gate above), so
// a dictation tool approved for a microphone never comes away with a camera.
async function setMediaApproved(req) {
  if (req.video && !cameraApproved) {
    if (!(await setCapabilityApproved('camera_approved', true, 'camera', () => pendingMedia === req && currentPreviewRequest(req)))) return false;
    cameraApproved = true;
  }
  if (req.audio && !microphoneApproved) {
    if (!(await setCapabilityApproved('microphone_approved', true, 'microphone', () => pendingMedia === req && currentPreviewRequest(req)))) return false;
    microphoneApproved = true;
  }
  return true;
}

document.getElementById('media-allow').addEventListener('click', async function() {
  const req = pendingMedia;
  if (!req || req.confirming) return;
  req.confirming = true;
  const opened = await approveAndOpenDirectly(() => setMediaApproved(req), () => pendingMedia === req && currentPreviewRequest(req), document.getElementById('media-allow'));
  req.confirming = false;
  if (!opened || pendingMedia !== req || !currentPreviewRequest(req)) return;
  pendingMedia = null;
  mediaModal.close();
  replyMedia(req, false,
    'Capture devices are unavailable in the embedded preview; the artifact was opened directly',
    'NotSupportedError');
});

// Both capability gates request the same app-origin confirmation as the
// toolbar and banner. The module owns the user gesture, placeholder tab and
// browser preference; this legacy script retains the capability transaction.
function approveAndOpenDirectly(approve, stillPending, trigger) {
  return new Promise(function (resolve) {
    document.dispatchEvent(new CustomEvent('exhibit:open-directly', {
      detail: { approve, stillPending, trigger, resolve }
    }));
  });
}

// Geolocation gate (av-f446): the camera gate's shape, for location, with one
// difference in the reason. A browser could deliver a position into this
// frame, and Chromium does given allow="geolocation", but it would spend this
// page's own location grant. The browser's prompt would name the library, not
// the tool, and the library would hold the visitor's location on the
// artifact's behalf. The frame therefore carries no delegation, and the
// approval is spent on the top-level render, whose Permissions-Policy header
// is built from it.
//
// Every path settles the frame's request, so the artifact's error callback
// runs instead of an app waiting forever on a position that is not coming.
// pendingGeolocation holds the request as an object rather than its id: a
// reloaded frame numbers its requests from 1 again, and an id compared by value
// could mistake a new request for the one the user approved.
let pendingGeolocation = null;
const geoDialog = document.getElementById('geo-modal');
const geoModal = modalFor(geoDialog);
function denyPendingGeolocation() {
  const req = pendingGeolocation;
  pendingGeolocation = null;
  if (req) replyGeolocation(req, false, 'User denied Geolocation');
}
for (const event of ['modal:closed', 'modal:disconnected']) geoDialog.addEventListener(event, denyPendingGeolocation);
document.addEventListener('exhibit:preview-reloaded', () => {
  pendingGeolocation = null;
  geoModal.close();
});

window.addEventListener('message', function(e) {
  const d = e.data;
  if (!d || d.__avGeolocation !== true || d.artifactId !== ID) return;
  const frame = document.querySelector('iframe');
  if (!frame || e.source !== frame.contentWindow) return;
  const req = { id: d.id, frame, generation: previewGeneration };
  if (typeof CONNECTION_PROTECTED === 'boolean' && CONNECTION_PROTECTED) {
    replyGeolocation(req, false, 'Connected data must stay inside Exhibit; location is unavailable in this preview.');
    return;
  }
  if (geolocationApproved) {
    // Nothing to decide. The approval exists; it just isn't spent in this
    // frame. Ask the frame to raise the banner, which offers the top-level
    // render.
    replyGeolocation(req, true, 'Location is unavailable in the embedded preview; open the artifact directly');
    return;
  }
  // Not ours to approve: settle it as the denial Block would produce. The
  // owner's approval would not have handed over the visitor's location anyway;
  // the browser asks its own question on the visitor's machine.
  if (!mayApprove() || !geoDialog.isConnected) {
    replyGeolocation(req, false, 'User denied Geolocation');
    return;
  }
  // A second request while the prompt is open displaces the first. Artifacts
  // commonly call getCurrentPosition and watchPosition back to back, and one
  // prompt for both is right, but the displaced one still gets its error
  // callback.
  if (pendingGeolocation) replyGeolocation(pendingGeolocation, false, 'User denied Geolocation');
  pendingGeolocation = req;
  geoDialog.querySelector('[data-permission-status]').textContent = '';
  geoModal.show(frame);
});

// Settles one pending geolocation request inside the frame, which then runs
// the artifact's error callback. targetOrigin is '*' because the frame's
// origin is opaque. banner asks the frame to raise the capability banner, as
// replyMedia's does; there is never a position to send.
function replyGeolocation(req, banner, error) {
  if (!currentPreviewRequest(req)) return;
  const frame = req.frame;
  frame.contentWindow.postMessage({ __avGeolocationResult: true, id: req.id, banner: banner, error: error }, '*');
}

async function setGeolocationApproved(req) {
  if (!(await setCapabilityApproved('geolocation_approved', true, 'location', () => pendingGeolocation === req && currentPreviewRequest(req)))) return false;
  geolocationApproved = true;
  return true;
}

document.getElementById('geo-allow').addEventListener('click', async function() {
  const req = pendingGeolocation;
  if (!req || req.confirming) return;
  req.confirming = true;
  const opened = await approveAndOpenDirectly(() => setGeolocationApproved(req), () => pendingGeolocation === req && currentPreviewRequest(req), document.getElementById('geo-allow'));
  req.confirming = false;
  if (!opened || pendingGeolocation !== req || !currentPreviewRequest(req)) return;
  pendingGeolocation = null;
  geoModal.close();
  replyGeolocation(req, false, 'Location is unavailable in the embedded preview; the artifact was opened directly');
});

// What a blocked origin becomes when the visitor cannot approve it (av-awr4).
//
// This is the whole of the recipient's half of the network model, and it is a
// statement rather than a control: a shared artifact is frozen at whatever its
// owner allowed, and widening that from here would be one person editing
// another person's CSP. Correct — and it fails invisibly unless somebody says
// so, which is what the ticket is actually about. Without this the tool draws
// an empty chart and no surface anywhere mentions the request that died.
//
// It accumulates rather than reporting the first and stopping: an artifact
// reaching four unapproved origins has four things wrong with it, and naming
// one would leave the visitor fixing a list they cannot see the end of. The
// preamble already reports each origin at most once per load, so the set here
// is the set of distinct blocked origins and the guard is belt to that.
const blockedOrigins = [];

function explainBlockedOrigin(origin) {
  const banner = document.getElementById('origin-blocked-banner');
  // Rendered only for a read-only visitor, and this only runs for one — but
  // the two facts are decided in different files, so the null check is what
  // keeps a future divergence a no-op instead of an exception at page load.
  if (!banner || !origin || blockedOrigins.indexOf(origin) !== -1) return;
  blockedOrigins.push(origin);

  // textContent throughout: an origin arrives from the artifact's own blocked
  // request and must never be interpreted as markup on the app origin.
  const headline = document.getElementById('origin-blocked-headline');
  if (headline) {
    headline.textContent = blockedOrigins.length === 1
      ? 'This tool tried to reach ' + origin + '. Its owner has not allowed that.'
      : 'This tool tried to reach ' + blockedOrigins.length +
        ' origins its owner has not allowed.';
  }
  const list = document.getElementById('origin-blocked-list');
  if (list) {
    const row = document.createElement('div');
    row.className = 'banner-detail-url';
    const code = document.createElement('code');
    code.textContent = origin;
    row.appendChild(code);
    list.appendChild(row);
  }
  banner.hidden = false;
}

// Network permission prompt (av-kmwj): the dialog and its whole behaviour
// live in network-prompt.js, shared with the agent chat page (av-6xvs). This
// page only supplies the things the two surfaces differ in.
//
// reload goes through the app origin's /open route rather than reusing the
// frame's current src: that src carries a render token minted when this page
// rendered, which expires — /open mints a fresh one on redirect, so the reload
// works on a page that has been open all afternoon. The stamp only defeats the
// browser's cache of the redirect itself.
networkPrompt.install({
  frame: function () { return document.querySelector('iframe'); },
  artifactId: function () { return ID; },
  readOnly: function () { return !mayApprove(); },
  explain: explainBlockedOrigin,
  report: function (text) {
    const st = document.getElementById('al-status');
    if (st) st.textContent = text;
  },
  reload: function () {
    // The host's policy is bound to the approved origins and current source.
    // Refresh it together with the frame after an origin approval.
    if (needsHostReload()) { window.location.reload(); return; }
    const frame = document.querySelector('iframe');
    frame.src = OPEN_URL + '?r=' + Date.now();
    frame.addEventListener('load', function onload() {
      frame.removeEventListener('load', onload);
      const st = document.getElementById('al-status');
      if (st) st.textContent = '';
    });
  }
});

// "Update from source" — only reachable from the controls' ⋮ menu, which the
// server renders only for URL-ingested artifacts (SOURCE_URL is '' otherwise).
async function refetchSource() {
  const warning = 'Re-fetch a fresh snapshot from the source URL?\n\n' +
    SOURCE_URL + '\n\n' +
    'This replaces the stored content with whatever the URL returns now, and ' +
    'saves its images, styles, scripts and runtime files into the artifact. ' +
    'The allowlist is never changed: new origins need your approval before ' +
    'they load. It is NOT versioned and cannot be undone. ' +
    "The artifact's saved state/data may break if the new content changed.";
  if (!confirm(warning)) return;
  const st = document.getElementById('al-status');
  st.textContent = 'Fetching…';
  try {
    const r = await apiFetch('/api/artifacts/' + ID + '/refetch', {
      method: 'POST',
      body: JSON.stringify({expected_source_revision: sourceRevision})
    });
    if (!r.ok) {
      const txt = await r.text().catch(() => '');
      st.textContent = '✗ Error: ' + (txt.trim() || r.statusText);
      return;
    }
    const data = await r.json().catch(() => ({}));
    if (data.updated === false) {
      st.textContent = '✓ Already up to date.';
      return;
    }
    sourceRevision = data.artifact.source_revision;
    const note = refetchNote(data);
    if (!note) {
      st.textContent = '✓ Updated — reloading…';
      window.location.reload();
      return;
    }
    // Something here needs reading, so the page stays until the user is done
    // with it rather than reloading out from under them.
    st.textContent = '✓ Updated. ' + note + ' ';
    const reload = document.createElement('button');
    reload.type = 'button';
    reload.textContent = 'Reload';
    reload.addEventListener('click', () => window.location.reload());
    st.appendChild(reload);
  } catch (e) {
    st.textContent = '✗ Error: ' + e.message;
  }
}

// refetchNote names what a refetch left for the user to act on: new origins
// nobody has allowed or refused yet (the response reports them and never
// approves them, av-b17a), and assets that could not be saved into the file.
// Empty when there is nothing to say.
function refetchNote(data) {
  const unapproved = data.footprint_changed ? (data.undecided_origins || []) : [];
  const failures = (data.snapshot && data.snapshot.failures) || [];
  const parts = [];
  if (unapproved.length > 0) {
    parts.push('It now contacts ' + unapproved.join(', ') +
      ', which will ask for approval when used.');
  }
  if (failures.length > 0) {
    parts.push(failures.length + (failures.length === 1 ? ' asset' : ' assets') +
      ' could not be saved into the file and still load from the source site.');
  }
  return parts.join(' ');
}

// Existing server-rendered toolbar action.
window.refetchSource = refetchSource;
