/* Artifact detail (viewer) page script. Served from the app origin at
 * /assets/gallery/detail.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads (and reassigns) before it loads:
 *   TOKEN / READ_ONLY  - this visitor's API credential, decided server-side
 *                        per request (av-5imk); spent via api.js's apiFetch
 *   ID                 - the artifact id
 *   SOURCE_URL         - source URL for URL-ingested artifacts ('' otherwise;
 *                        the Update-from-source button only renders when set)
 *   OPEN_URL           - the app-origin route that mints a fresh render token
 *                        and redirects to it: the "Open in new tab" destination,
 *                        where capture devices actually work (av-mv3k), and what
 *                        the network prompt reloads the frame through (av-kmwj)
 *   downloadsApproved  - persisted first-use download approval (mutable)
 *   clipboardApproved  - persisted first-use clipboard approval (mutable)
 *   linksApproved      - persisted first-use external-link approval (mutable)
 *   cameraApproved     - persisted first-use camera approval (mutable)
 *   microphoneApproved - persisted first-use microphone approval (mutable)
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
  // URL construction lives in state-api.js so this and agent.js share one
  // definition — the ".." path-traversal bug (av-hh1o) had to be fixed in
  // three copies of it.
  if (d.op === 'clear') {
    apiFetch(window.ExhibitState.deleteURL(ID), {
      method: 'DELETE'
    }).catch(function(){});
  } else if (d.op === 'delete') {
    apiFetch(window.ExhibitState.deleteURL(ID, d.key), {
      method: 'DELETE'
    }).catch(function(){});
  } else if (d.op === 'set' || d.op === undefined) {
    // Only a recognized write reaches the API. An unknown op used to fall
    // through to this branch, so a future typo would silently become a write.
    apiFetch(window.ExhibitState.url(ID), {
      method: 'PUT',
      body: JSON.stringify({ key: d.key, value: d.value })
    }).catch(function(){});
  }
});

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
window.ExhibitNetworkPrompt.announceTo(document.querySelector('iframe'));

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
  const r = await apiFetch('/api/artifacts/' + ID, {
    method: 'PATCH',
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

// Link navigation bridge (av-r0dk): the sandboxed frame cannot open external
// links itself — a target=_blank anchor is dropped without allow-popups, and a
// plain anchor would replace the iframe with an external page that usually
// refuses framing. The shim posts external link activations here; when approved
// we open the URL in a new tab from the app origin (the click's transient
// activation covers the postMessage roundtrip). Unapproved, we show the
// first-request confirmation (av-e3sj), naming the destination host, and open
// nothing until the user allows.
let pendingLink = null;
let linkAllowPending = false;

window.addEventListener('message', function(e) {
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
  if (linksApproved) {
    window.open(url.href, '_blank', 'noopener');
    return;
  }
  pendingLink = { url: url.href, host: url.hostname };
  document.getElementById('link-host').textContent = url.hostname;
  document.getElementById('link-modal').hidden = false;
  // Move focus into the dialog so keyboard users land on the decision rather
  // than on whatever sat behind the overlay.
  document.getElementById('link-block').focus();
});

// Persists the first-use grant, then lets the caller open the pending URL. The
// viewer is read-only (av-hwx2) — like downloads/clipboard, it only grants on
// the artifact's first attempt; the revoke control lives on the Edit page.
async function setLinksApproved(approved) {
  if (!(await setCapabilityApproved('links_approved', approved, 'link'))) return false;
  linksApproved = approved;
  return true;
}

// Denial just drops the destination and the artifact keeps running — nothing is
// persisted, mirroring downloads (denial drops, approval persists).
function closeLinkModal() {
  // While an Allow request is in flight, dismissal must not invalidate the
  // transaction: the allow handler re-verifies pendingLink is still the same
  // link after the PATCH and skips opening if anything changed.
  if (linkAllowPending) return;
  document.getElementById('link-modal').hidden = true;
  pendingLink = null;
  // Hand focus back to the artifact so keyboard users return where they were.
  const frame = document.querySelector('iframe');
  if (frame) frame.focus();
}

document.getElementById('link-block').addEventListener('click', closeLinkModal);
document.getElementById('link-modal').addEventListener('click', function(e) {
  if (e.target.id === 'link-modal') closeLinkModal();
});
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape' && !document.getElementById('link-modal').hidden) closeLinkModal();
});
document.getElementById('link-allow').addEventListener('click', async function() {
  const link = pendingLink;
  linkAllowPending = true;
  const ok = await setLinksApproved(true);
  linkAllowPending = false;
  // Only settle the transaction if the pending destination is still the one
  // the user approved — a dismissal or a newer link must not be overridden by
  // opening this URL after the fact.
  if (!ok || pendingLink !== link) return;
  closeLinkModal();
  if (link) window.open(link.url, '_blank', 'noopener');
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
  const req = { id: String(d.id), audio: !!d.audio, video: !!d.video };
  if (!req.audio && !req.video) return;
  // Approved means approved for every device this request named: a camera-only
  // grant must still prompt when the artifact later asks for the microphone.
  const needsPrompt = (req.video && !cameraApproved) || (req.audio && !microphoneApproved);
  if (!needsPrompt) {
    // Nothing to decide — the grant is already there, it just cannot be spent
    // in this frame. Ask the frame to raise the banner, which offers the
    // top-level render, and settle the call.
    replyMedia(req.id, true, 'Capture devices are unavailable in the embedded preview; open the artifact directly',
      'NotSupportedError');
    return;
  }
  // A second request arriving while the prompt is open displaces the first, so
  // settle the one being displaced rather than leaving its getUserMedia promise
  // pending forever — an artifact that never settles looks like a hang, which is
  // the failure mode this gate exists to remove.
  if (pendingMedia) replyMedia(pendingMedia.id, false, 'Permission denied', 'NotAllowedError');
  pendingMedia = req;
  document.getElementById('media-devices').textContent = mediaDeviceLabel(req);
  document.getElementById('media-icon').className = req.video ? 'ph ph-camera' : 'ph ph-microphone';
  document.getElementById('media-modal').hidden = false;
  document.getElementById('media-block').focus();
});

// Settles one pending getUserMedia inside the frame. targetOrigin is '*'
// because the frame's origin is opaque. banner asks the frame to raise the
// unsupported-capability banner (the frame owns that channel, so there is one
// path to it rather than two); every reply is a rejection, since there is no
// stream to return.
function replyMedia(id, banner, error, name) {
  const frame = document.querySelector('iframe');
  if (!frame) return;
  frame.contentWindow.postMessage(
    { __avMediaResult: true, id: id, ok: false, banner: banner, error: error, name: name }, '*'
  );
}

// Persists only the devices this request named (see the prompt gate above), so
// a dictation tool approved for a microphone never comes away with a camera.
async function setMediaApproved(req) {
  if (req.video && !cameraApproved) {
    if (!(await setCapabilityApproved('camera_approved', true, 'camera'))) return false;
    cameraApproved = true;
  }
  if (req.audio && !microphoneApproved) {
    if (!(await setCapabilityApproved('microphone_approved', true, 'microphone'))) return false;
    microphoneApproved = true;
  }
  return true;
}

// deny=true settles the pending call so the artifact's getUserMedia rejects
// with the DOMException a blocked call throws, instead of hanging.
function closeMediaModal(deny) {
  document.getElementById('media-modal').hidden = true;
  if (deny && pendingMedia) replyMedia(pendingMedia.id, false, 'Permission denied', 'NotAllowedError');
  pendingMedia = null;
  const frame = document.querySelector('iframe');
  if (frame) frame.focus();
}

document.getElementById('media-block').addEventListener('click', function() { closeMediaModal(true); });
document.getElementById('media-modal').addEventListener('click', function(e) {
  if (e.target.id === 'media-modal') closeMediaModal(true);
});
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape' && !document.getElementById('media-modal').hidden) closeMediaModal(true);
});
document.getElementById('media-allow').addEventListener('click', async function() {
  const req = pendingMedia;
  if (!req) return;
  // Claim the tab in the click's own task, before the PATCH. A window.open that
  // waits on a roundtrip first is an unsolicited popup: Safari blocks it
  // outright, and Chrome allows it only while the transient activation is still
  // live, so a slow PATCH loses the tab and the artifact is then told it was
  // opened directly when nothing opened. The placeholder is navigated once the
  // approval is persisted and closed if it isn't.
  //
  // 'noopener' can't be used here — it returns null, leaving nothing to
  // navigate — so the opener is severed by hand instead, while the tab is still
  // about:blank and therefore same-origin enough for the property to be
  // writable. It stays severed across the navigation. This is not decoration:
  // the top-level render runs the artifact's own script, and an opener handle
  // would let it navigate the library tab out from under the user.
  const tab = window.open('', '_blank');
  if (tab) {
    try { tab.opener = null; } catch (err) { /* cross-origin already; nothing to sever */ }
  }
  if (!(await setMediaApproved(req))) {
    if (tab) tab.close();
    return;
  }
  // Only settle the transaction if the pending request is still the one the
  // user approved — a dismissal or a newer request must not be answered by
  // opening a tab for this one after the fact.
  if (pendingMedia !== req) {
    if (tab) tab.close();
    return;
  }
  document.getElementById('media-modal').hidden = true;
  pendingMedia = null;
  // No banner: the user is already looking at the place the grant works.
  if (tab) tab.location = OPEN_URL;
  else window.open(OPEN_URL, '_blank', 'noopener');
  replyMedia(req.id, false,
    'Capture devices are unavailable in the embedded preview; the artifact was opened directly',
    'NotSupportedError');
});

// Network permission prompt (av-kmwj): the dialog and its whole behaviour
// live in network-prompt.js, shared with the agent chat page (av-6xvs). This
// page only supplies the four things the two surfaces differ in.
//
// reload goes through the app origin's /open route rather than reusing the
// frame's current src: that src carries a render token minted when this page
// rendered, which expires — /open mints a fresh one on redirect, so the reload
// works on a page that has been open all afternoon. The stamp only defeats the
// browser's cache of the redirect itself.
window.ExhibitNetworkPrompt.install({
  frame: function () { return document.querySelector('iframe'); },
  artifactId: function () { return ID; },
  readOnly: function () { return typeof READ_ONLY === 'boolean' && READ_ONLY; },
  report: function (text) {
    const st = document.getElementById('al-status');
    if (st) st.textContent = text;
  },
  reload: function () {
    const frame = document.querySelector('iframe');
    frame.src = OPEN_URL + '?r=' + Date.now();
    frame.addEventListener('load', function onload() {
      frame.removeEventListener('load', onload);
      const st = document.getElementById('al-status');
      if (st) st.textContent = '';
    });
  }
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
    const r = await apiFetch('/api/artifacts/' + ID + '/refetch', {
      method: 'POST'
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
