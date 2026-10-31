// App-origin gate for top-level artifact opens. Markup and copy live in the
// template. /open also reloads frames, so the redirect itself stays ungated.
const modal = document.getElementById('direct-open-modal');
const remember = document.getElementById('direct-open-remember');
const confirm = document.getElementById('direct-open-confirm');
const status = document.getElementById('direct-open-status');
const { viewer } = modal.dataset;
let pending = null;

function remembered(preferenceKey) {
  try { return preferenceKey && localStorage.getItem(preferenceKey) === '1'; }
  catch (err) { return false; }
}

function focusTarget(request) {
  const trigger = request.trigger;
  if (trigger && trigger.isConnected !== false && !trigger.closest('[hidden], dialog:not([open])')) return trigger;
  return document.getElementById('pv-open') || document.getElementById('pip-more');
}

function show(message = '') {
  status.textContent = message;
  status.hidden = !message;
  confirm.disabled = false;
  remember.disabled = false;
  modal.dispatchEvent(new CustomEvent('exhibit:modal-open', { detail: { trigger: focusTarget(pending) } }));
}

function finish(opened, disconnected = false) {
  const request = pending;
  if (!request) return;
  pending = null;
  if (!opened && request.tab) request.tab.close();
  remember.checked = false;
  confirm.disabled = false;
  remember.disabled = false;
  modal.dispatchEvent(new CustomEvent('exhibit:modal-close'));
  // Shared modal returns to the original trigger. Resolve a feature fallback
  // only when htmx or the toolbar menu has made that trigger unavailable.
  const target = focusTarget(request);
  if (!disconnected && target !== request.trigger && target?.isConnected !== false) target?.focus();
  request.resolve?.(opened);
}

modal.addEventListener('modal:closed', () => finish(false));
modal.addEventListener('modal:disconnected', () => finish(false, true));

async function open(request, saveChoice) {
  if (pending !== request || request.busy) return;
  if (request.stillPending && !request.stillPending()) { finish(false); return; }
  request.busy = true;
  confirm.disabled = true;
  remember.disabled = true;
  status.hidden = true;
  // Claim exactly one tab in this click's task, before any capability PATCH.
  // A noopener feature would return null even on success. Sever the blank
  // tab's opener immediately instead, before any artifact code can run, so
  // popup blocking is distinguishable and can offer a fresh click to retry.
  let tab;
  try {
    tab = window.open('', '_blank');
    if (tab) tab.opener = null;
  } catch (err) {
    tab?.close();
    tab = null;
  }
  request.tab = tab;
  if (!tab) {
    request.busy = false;
    show('Your browser blocked the new tab. Allow popups for Exhibit and try again.');
    return;
  }
  try {
    const approved = request.approve ? await request.approve() : true;
    // Dismissal, a displaced frame request, or a closed placeholder must not
    // navigate later or remember a choice for a transaction that never opened.
    if (pending !== request) { tab.close(); return; }
    if ((request.stillPending && !request.stillPending()) || tab.closed) {
      finish(false);
      return;
    }
    if (!approved) throw new Error('approval failed');
    tab.location = request.openUrl;
    if (saveChoice && request.preferenceKey) {
      try { localStorage.setItem(request.preferenceKey, '1'); }
      catch (err) { /* Ask again next time when persistence is unavailable. */ }
    }
    finish(true);
  } catch (err) {
    tab.close();
    if (pending !== request) return;
    request.tab = null;
    request.busy = false;
    show('Unable to open the artifact. Try again.');
  }
}

function requestOpen(options = {}) {
  if (pending || !modal.isConnected) { options.resolve?.(false); return; }
  const trigger = options.trigger || document.activeElement;
  // Agent controls are replaced by server-rendered preview swaps. Capture
  // their artifact and URL together for this decision, rather than at boot.
  const artifact = trigger?.dataset.directOpenArtifact ?? modal.dataset.artifact;
  const openUrl = trigger?.dataset.directOpenUrl ?? modal.dataset.openUrl;
  if (!artifact || !openUrl) { options.resolve?.(false); return; }
  const preferenceKey = viewer
    ? 'exhibit:direct-open:v1:' + JSON.stringify([viewer, artifact]) : null;
  pending = { ...options, trigger, openUrl, preferenceKey, tab: null, busy: false };
  remember.checked = false;
  if (remembered(preferenceKey)) open(pending, false);
  else show();
}

for (const id of ['direct-open-toolbar', 'direct-open-banner']) {
  const trigger = document.getElementById(id);
  trigger?.addEventListener('click', () => requestOpen({ trigger }));
}
document.addEventListener('click', (event) => {
  const trigger = event.target.closest('[data-direct-open-artifact]');
  if (trigger) requestOpen({ trigger });
});
// The existing capability handlers supply their approval and stale-request
// guard. No API writes belong to this viewer preference, so recipients can
// remember their own choice as well.
document.addEventListener('exhibit:open-directly', (event) => requestOpen(event.detail));
confirm.addEventListener('click', () => { if (pending) return open(pending, remember.checked); });
