/* Agent chat surface script (Exh-jlbt). Served from the app origin at
 * /assets/gallery/agent.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads (and reassigns) before it loads:
 *   TOKEN / READ_ONLY - this visitor's API credential, decided server-side
 *                   per request (av-5imk); spent via api.js's apiFetch
 *   artifact      - {id,title} when opened in modify mode, else null (mutable)
 *
 * The preview pane's markup (title, links, iframe) is not built here: it is a
 * server-rendered fragment htmx swaps in after every agent save (av-6m3e), so
 * the render-origin URLs are composed server-side.
 */
const MODEL_SUGGESTIONS = {
  'anthropic':   ['claude-sonnet-4-5', 'claude-opus-4-8', 'claude-haiku-4-5'],
  'openai':      ['gpt-5.2', 'gpt-5-mini'],
  'google':      ['gemini-2.5-pro', 'gemini-2.5-flash'],
  'openrouter':  ['anthropic/claude-sonnet-4.5'],
  // image-capable models first — snippet mode attaches screenshots
  'opencode-go': ['kimi-k2.7-code', 'minimax-m3', 'qwen3.6-plus', 'mimo-v2.5', 'glm-5.2', 'deepseek-v4-pro'],
  'exhibit-mock':['exhibit-mock-1']
};

let sessionId = null;
let eventSource = null;
let streaming = false;
let keyConfigured = false;
let configuredProvider = null;   // provider the stored key currently belongs to, or null
let pendingSnippets = [];   // [{image:{data,mimeType}, descriptor, thumbUrl}]
let snippetMode = false;
// A brief arrived from /new but there was no key to send it with. saveKey
// spends this, so "Start building" still means the brief is sent (nw-d1dd).
let briefAwaitingKey = false;

const messagesEl = document.getElementById('messages');
const inputEl = document.getElementById('input');

// The preview pane is a server-rendered fragment htmx swaps in after every
// agent save (av-6m3e), so #pv-frame is a *new element* each time — a cached
// reference would go stale on the first swap and silently break the state
// bridge and snippet mode. Always look it up on use.
function previewFrame() { return document.getElementById('pv-frame'); }

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}
function addMsg(cls, text) {
  const m = el('div', 'msg ' + cls, text);
  messagesEl.appendChild(m);
  messagesEl.scrollTop = messagesEl.scrollHeight;
  return m;
}

// --- API key management --------------------------------------------------
// All of it is BYOK-only (av-siqf). In platform mode the instance supplies the
// credential, the key markup is absent from the page rather than hidden, and
// the key route does not exist — so every entry point below returns early
// instead of reaching for elements that aren't there. refreshKeyStatus reports
// "configured", because from this page's point of view a key is: boot must not
// prompt for one nobody can enter.
async function refreshKeyStatus() {
  if (!BYOK) {
    // The instance holds the credential, so a key *is* configured as far as
    // this page is concerned. Saying so here is what keeps send() from
    // stopping at its own key check on the way to a session nobody has to
    // configure.
    keyConfigured = true;
    return true;
  }
  const r = await apiFetch('/api/agent/key');
  const d = await r.json();
  keyConfigured = !!d.configured;
  configuredProvider = keyConfigured ? d.provider : null;
  const btn = document.getElementById('key-btn');
  const label = document.getElementById('key-btn-label');
  if (keyConfigured) {
    btn.classList.remove('warn');
    label.textContent = d.provider + ' · ' + (d.model || 'default');
    document.getElementById('key-provider').value = d.provider;
    document.getElementById('key-model').value = d.model || '';
    const cur = document.getElementById('current-key');
    cur.hidden = false;
    cur.textContent = 'A key is already configured for ' + d.provider + '. Saving keeps it unless you delete the masked value below and enter a new one.';
    document.getElementById('key-delete').hidden = false;
  } else {
    btn.classList.add('warn');
    label.textContent = 'Set API key';
    document.getElementById('current-key').hidden = true;
    document.getElementById('key-delete').hidden = true;
  }
  providerChanged();
  return keyConfigured;
}

function providerChanged() {
  const p = document.getElementById('key-provider').value;
  const dl = document.getElementById('model-suggestions');
  dl.innerHTML = '';
  (MODEL_SUGGESTIONS[p] || []).forEach(m => {
    const o = document.createElement('option');
    o.value = m;
    dl.appendChild(o);
  });
  const modelInput = document.getElementById('key-model');
  if (!modelInput.value && (MODEL_SUGGESTIONS[p] || []).length) {
    modelInput.value = MODEL_SUGGESTIONS[p][0];
  }
  // The masked key belongs to configuredProvider; switching away from it
  // means that key can't be reused, so prompt for a fresh one.
  const secret = document.getElementById('key-secret');
  if (secret.dataset.masked === 'true' && p !== configuredProvider) {
    clearMaskedKey(secret);
  }
}

const MASKED_KEY_PLACEHOLDER = '••••••••';

function showMaskedKey(secret) {
  secret.value = MASKED_KEY_PLACEHOLDER;
  secret.readOnly = true;
  secret.dataset.masked = 'true';
}
function clearMaskedKey(secret) {
  secret.value = '';
  secret.readOnly = false;
  secret.dataset.masked = 'false';
}

function openKeyModal() {
  if (!BYOK) return;
  document.getElementById('key-error').hidden = true;
  const secret = document.getElementById('key-secret');
  if (keyConfigured) { showMaskedKey(secret); } else { clearMaskedKey(secret); }
  document.getElementById('key-modal').hidden = false;
  secret.focus();
}
function closeKeyModal() { document.getElementById('key-modal').hidden = true; }

// The masked placeholder is a single unit, not editable text: it can only be
// cleared in full (Backspace/Delete), never edited in place. Cancel discards
// any clear — reopening the modal re-derives the mask from server state
// (keyConfigured), never from whatever was left in the field.
(function () {
  const secret = document.getElementById('key-secret');
  if (!secret) return;   // platform mode: there is no key field to guard
  secret.addEventListener('keydown', (e) => {
    if (secret.dataset.masked !== 'true') return;
    if (e.key === 'Backspace' || e.key === 'Delete') {
      e.preventDefault();
      clearMaskedKey(secret);
    } else if (!['Tab', 'Shift', 'Control', 'Alt', 'Meta', 'Escape'].includes(e.key)) {
      e.preventDefault();
    }
  });
  secret.addEventListener('paste', (e) => {
    if (secret.dataset.masked === 'true') e.preventDefault();
  });
})();

async function saveKey() {
  const provider = document.getElementById('key-provider').value;
  const model = document.getElementById('key-model').value.trim();
  const secretInput = document.getElementById('key-secret');
  const errEl = document.getElementById('key-error');
  errEl.hidden = true;
  let api_key = '';
  if (secretInput.dataset.masked === 'true') {
    // Field untouched: keep the existing key. providerChanged() already
    // clears the mask if the provider no longer matches, so reaching here
    // masked means provider === configuredProvider and it's safe to reuse.
  } else {
    api_key = secretInput.value.trim();
    if (!api_key) { errEl.textContent = 'Enter the API key.'; errEl.hidden = false; return; }
  }
  const r = await apiFetch('/api/agent/key', {method:'PUT', body: JSON.stringify({provider, model, api_key})});
  if (!r.ok) {
    const d = await r.json().catch(() => ({}));
    errEl.textContent = d.error || 'Failed to save key.';
    errEl.hidden = false;
    return;
  }
  closeKeyModal();
  await refreshKeyStatus();
  // A new key means the next prompt should start a fresh session.
  resetSession();
  addMsg('sys', 'API key saved. The key is encrypted on the server and never returned to the browser.');
  // The brief that couldn't be sent a moment ago goes now. send() reads the
  // composer rather than the stored brief, so an edit made while the modal was
  // up is what gets sent. Only a *save* spends it: cancelling the modal leaves
  // the text in the composer for the user to send when they're ready.
  if (briefAwaitingKey) {
    briefAwaitingKey = false;
    if (inputEl.value.trim()) send();
  }
}

async function deleteKey() {
  if (!confirm('Remove the stored API key?')) return;
  await apiFetch('/api/agent/key', {method:'DELETE'});
  closeKeyModal();
  await refreshKeyStatus();
  resetSession();
}

// --- Session + SSE --------------------------------------------------------
function resetSession() {
  closeEvents();
  if (sessionId) { apiFetch('/api/agent/sessions/' + sessionId, {method:'DELETE'}).catch(()=>{}); }
  sessionId = null;
  setStreaming(false);
}

async function ensureSession() {
  if (sessionId) return true;
  const body = artifact ? {artifact_id: artifact.id} : {};
  const r = await apiFetch('/api/agent/sessions', {method:'POST', body: JSON.stringify(body)});
  const d = await r.json().catch(() => ({}));
  if (!r.ok) {
    addMsg('err', d.error || 'Could not start an agent session.');
    if (r.status === 412) openKeyModal();
    return false;
  }
  sessionId = d.id;
  connectEvents(d.sse_ticket);
  return true;
}

// api.js credentials the stream: a single-use, seconds-lived, session-bound
// ticket on a single-user instance — never the service token, which a URL would
// leak into request logs, proxy logs, and history (av-rgp1) — and nothing at all
// when a session cookie authenticates it.
//
// Because a ticket is spent on connect, EventSource's own automatic retry
// cannot reconnect us: it would replay a URL whose ticket is already gone. So
// we drive reconnection ourselves, minting a ticket per attempt. The session's
// backlog replay makes that lossless.
const EVENTS_RETRY_MS = 1000;
const EVENTS_RETRY_MAX_MS = 15000;
const TICKET_REFRESH_TIMEOUT_MS = 10000;
let eventsRetryMs = EVENTS_RETRY_MS;
let eventsRetryTimer = null;
let ticketRefreshController = null;

function closeEvents() {
  if (eventsRetryTimer) { clearTimeout(eventsRetryTimer); eventsRetryTimer = null; }
  if (ticketRefreshController) { ticketRefreshController.abort(); ticketRefreshController = null; }
  if (eventSource) { eventSource.close(); eventSource = null; }
}

function connectEvents(ticket) {
  eventSource = apiEventSource(
    '/api/agent/sessions/' + encodeURIComponent(sessionId) + '/events', ticket);
  eventSource.onopen = () => { eventsRetryMs = EVENTS_RETRY_MS; };
  eventSource.onmessage = (e) => {
    let ev;
    try { ev = JSON.parse(e.data); } catch { return; }
    handleAgentEvent(ev);
  };
  eventSource.onerror = () => { reconnectEvents(); };
}

function reconnectEvents() {
  if (!sessionId || eventsRetryTimer) return;
  closeEvents();
  const wait = eventsRetryMs;
  eventsRetryMs = Math.min(eventsRetryMs * 2, EVENTS_RETRY_MAX_MS);
  eventsRetryTimer = setTimeout(async () => {
    eventsRetryTimer = null;
    if (!sessionId) return;
    const watching = sessionId;
    const controller = new AbortController();
    ticketRefreshController = controller;
    const timeout = setTimeout(() => controller.abort(), TICKET_REFRESH_TIMEOUT_MS);
    let r = null;
    try {
      r = await apiFetch('/api/agent/sessions/' + encodeURIComponent(watching) + '/ticket',
        {method:'POST', signal: controller.signal});
    } catch { /* offline, timed out, or aborted — fall through to another retry */ }
    clearTimeout(timeout);
    if (ticketRefreshController === controller) ticketRefreshController = null;
    if (sessionId !== watching) return;   // session was reset while we waited
    if (!r || !r.ok) {
      // 404 means the session is gone (closed or reaped): stop retrying.
      if (r && r.status === 404) { sessionId = null; setStreaming(false); return; }
      reconnectEvents();
      return;
    }
    const d = await r.json().catch(() => ({}));
    if (d.sse_ticket) connectEvents(d.sse_ticket);
    else reconnectEvents();
  }, wait);
}

// Streaming display state
let curAssistantEl = null;   // bubble receiving text deltas
let thinkingEl = null;
let toolChips = {};          // toolCallId -> chip element

function handleAgentEvent(ev) {
  switch (ev.type) {
    case 'agent_start':
      setStreaming(true);
      break;
    case 'agent_settled':
      setStreaming(false);
      curAssistantEl = null;
      removeThinking();
      break;
    case 'message_update': {
      const d = ev.assistantMessageEvent;
      if (!d) break;
      if (d.type === 'text_delta') {
        removeThinking();
        if (!curAssistantEl) curAssistantEl = addMsg('assistant', '');
        curAssistantEl.textContent += d.delta;
        messagesEl.scrollTop = messagesEl.scrollHeight;
      } else if (d.type === 'thinking_start' && !thinkingEl) {
        thinkingEl = el('div', 'thinking');
        thinkingEl.appendChild(el('i', 'ph ph-circle-notch'));
        thinkingEl.appendChild(document.createTextNode('thinking…'));
        messagesEl.appendChild(thinkingEl);
      } else if (d.type === 'text_end') {
        curAssistantEl = null;   // next text block gets its own bubble
      }
      break;
    }
    case 'tool_execution_start': {
      removeThinking();
      const chip = el('div', 'tool-chip');
      const label = toolLabel(ev.toolName, ev.args);
      chip.dataset.label = label;
      chip.innerHTML = '<i class="ph ph-gear"></i> ';
      chip.appendChild(document.createTextNode(label + '…'));
      toolChips[ev.toolCallId] = chip;
      messagesEl.appendChild(chip);
      messagesEl.scrollTop = messagesEl.scrollHeight;
      break;
    }
    case 'tool_execution_end': {
      const chip = toolChips[ev.toolCallId];
      if (chip) {
        chip.className = 'tool-chip ' + (ev.isError ? 'fail' : 'done');
        // The end event carries no args; reuse the label captured at start.
        const label = chip.dataset.label || toolLabel(ev.toolName, ev.args);
        chip.innerHTML = (ev.isError ? '<i class="ph ph-x-circle"></i> ' : '<i class="ph ph-check-circle"></i> ');
        chip.appendChild(document.createTextNode(label));
        if (ev.isError) {
          const detail = (ev.result && ev.result.content && ev.result.content[0] && ev.result.content[0].text) || '';
          if (detail) addMsg('err', detail.slice(0, 400));
        }
      }
      break;
    }
    case 'exhibit_artifact_saved': {
      // The server-side hook behind this event (Session.noteArtifactSaved)
      // fires once create_artifact/update_artifact has landed, which makes it
      // the trigger for re-rendering the preview pane.
      artifact = {id: ev.artifactId, title: ev.title || 'Artifact'};
      refreshPreview();
      nudgePreview();
      let note = (ev.action === 'created' ? 'Artifact created' : 'Artifact updated') +
        (mobileQuery.matches ? ' — tap Preview to view it.' : ' — preview on the right.');
      if (ev.footprint && ev.footprint.length) {
        note += ' It references external origins (' + ev.footprint.join(', ') + '); they stay blocked until you approve them on the artifact page.';
      }
      addMsg('sys', note);
      break;
    }
    case 'exhibit_state_changed': {
      // State edits are inlined into the document at render time, so the
      // preview iframe is stale until something re-renders it — reuse the
      // same htmx swap a save uses rather than a second refresh mechanism.
      if (!artifact || artifact.id !== ev.artifactId) {
        artifact = {id: ev.artifactId, title: artifact ? artifact.title : 'Artifact'};
      }
      refreshPreview();
      nudgePreview();
      const label = ev.action === 'cleared_all' ? 'Erased all state'
        : ev.action === 'deleted_key' ? 'Deleted state key "' + ev.key + '"'
        : 'Set state key "' + ev.key + '"';
      addMsg('sys', label + (mobileQuery.matches ? ' — tap Preview to see it.' : ' — preview refreshed.'));
      break;
    }
    case 'exhibit_widget_saved': {
      // set_widget landed (av-fafu). The artifact body is unchanged, so this
      // is not an artifact save — but the pane is re-rendered by the same
      // fragment fetch, which is what brings the new tile in. A widget shares
      // the artifact's allowlist, so unapproved origins are already blocked
      // rather than merely pending; say so plainly.
      if (!artifact) artifact = {id: ev.artifactId, title: 'Artifact'};
      refreshPreview();
      let note = 'Gallery widget saved — it appears on this artifact’s card in the library.';
      if (ev.unapproved && ev.unapproved.length) {
        note += ' It references ' + ev.unapproved.join(', ') + ', which the artifact’s allowlist does not cover, so the browser blocks those.';
      }
      addMsg('sys', note);
      break;
    }
    case 'extension_error':
      addMsg('err', 'Extension error: ' + (ev.error || 'unknown'));
      break;
    case 'exhibit_session_closed':
      setStreaming(false);
      if (sessionId) addMsg('sys', 'Agent session ended. Your next message starts a new one.');
      if (eventSource) { eventSource.close(); eventSource = null; }
      sessionId = null;
      break;
    case 'auto_retry_start':
      addMsg('sys', 'Provider hiccup — retrying (' + ev.attempt + '/' + ev.maxAttempts + ')…');
      break;
  }
}

function toolLabel(name, args) {
  args = args || {};
  switch (name) {
    case 'create_artifact': return 'Creating "' + (args.title || 'artifact') + '"';
    case 'update_artifact': return 'Updating artifact';
    case 'get_artifact': return 'Reading artifact source';
    case 'get_state': return 'Reading artifact state';
    case 'set_state': return 'Setting state key "' + (args.key || '') + '"';
    case 'delete_state': return args.key ? 'Deleting state key "' + args.key + '"' : 'Erasing all state';
    default: return name;
  }
}

function removeThinking() {
  if (thinkingEl) { thinkingEl.remove(); thinkingEl = null; }
}

function setStreaming(on) {
  streaming = on;
  document.getElementById('stop-btn').style.display = on ? '' : 'none';
}

async function stopAgent() {
  if (!sessionId) return;
  await apiFetch('/api/agent/sessions/' + sessionId + '/abort', {method:'POST'});
}

// --- Sending ---------------------------------------------------------------
async function send() {
  const text = inputEl.value.trim();
  if (!text) return;
  if (!keyConfigured) { openKeyModal(); return; }
  if (!(await ensureSession())) return;

  const images = pendingSnippets.filter(s => s.image).map(s => ({data: s.image.data, mime_type: s.image.mimeType}));
  // A snippet descriptor carries the picked element's outerHTML — artifact
  // content, i.e. untrusted. It travels as its own field so the server can
  // fence it as data (av-e0yj); splicing it into `message` here would hand it
  // to the model as part of the user's instruction.
  const snippets = pendingSnippets.map(s => describeSnippet(s.descriptor));

  const bubble = addMsg('user', text);
  pendingSnippets.forEach(s => {
    if (s.thumbUrl) {
      const img = document.createElement('img');
      img.className = 'snip-thumb';
      img.src = s.thumbUrl;
      bubble.appendChild(img);
    }
  });
  clearSnippets();
  inputEl.value = '';
  autoGrow();

  const r = await apiFetch('/api/agent/sessions/' + sessionId + '/prompt', {
    method: 'POST',
    body: JSON.stringify({message: text, images, snippets})
  });
  if (!r.ok) {
    const d = await r.json().catch(() => ({}));
    addMsg('err', d.error || 'The agent rejected the message.');
  }
}

function describeSnippet(d) {
  if (!d) return '(no descriptor)';
  const lines = [
    'selector: ' + d.selector,
    'tag: <' + d.tag + '>' + (d.id ? ' id="' + d.id + '"' : '') + (d.classes && d.classes.length ? ' class="' + d.classes.join(' ') + '"' : ''),
  ];
  if (d.text) lines.push('text: ' + JSON.stringify(d.text));
  if (d.rect) lines.push('size: ' + Math.round(d.rect.width) + 'x' + Math.round(d.rect.height) + 'px');
  if (d.outerHTML) lines.push('outerHTML:\n' + d.outerHTML);
  return lines.join('\n');
}

inputEl.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
});
function autoGrow() {
  inputEl.style.height = 'auto';
  inputEl.style.height = Math.min(inputEl.scrollHeight, 140) + 'px';
}
inputEl.addEventListener('input', autoGrow);

// --- Mobile panes (av-td4y) ------------------------------------------------
// Below 640px the chat and the preview each take the whole screen and the
// segmented control picks between them; above it the media query never fires,
// so every call here is inert bookkeeping on an invisible control.
const mobileQuery = window.matchMedia('(max-width:640px)');
const tabChatEl = document.getElementById('tab-chat');
const tabPreviewEl = document.getElementById('tab-preview');
const nudgeEl = document.getElementById('pv-nudge');

function showPane(pane) {
  const preview = pane === 'preview';
  document.body.classList.toggle('show-preview', preview);
  tabChatEl.setAttribute('aria-selected', String(!preview));
  tabPreviewEl.setAttribute('aria-selected', String(preview));
  if (preview) clearPreviewNudge();
}

// An agent save the user can't see — they're on the Chat pane — lights the
// Preview segment so the new render isn't missed.
function nudgePreview() {
  if (document.body.classList.contains('show-preview')) return;
  tabPreviewEl.classList.add('has-update');
  tabPreviewEl.setAttribute('aria-label', 'Preview, updated');
  nudgeEl.hidden = false;
}
function clearPreviewNudge() {
  tabPreviewEl.classList.remove('has-update');
  tabPreviewEl.removeAttribute('aria-label');
  nudgeEl.hidden = true;
}

// --- Preview + snippet mode (Exh-edjk) -------------------------------------
// Read by the pane's hx-vals: the fragment is fetched for whichever artifact
// the session is currently bound to.
function previewArtifactId() { return artifact ? artifact.id : ''; }

// The agent saved something — hand the pane to htmx (av-6m3e). The fragment it
// fetches carries the title, the links, a freshly stamped iframe src, and the
// widget tile (av-fafu), so nothing here rebuilds markup the template already
// owns. Both save events route through here: the pane is re-rendered whole, so
// it does not need to know which of the two documents changed.
function refreshPreview() {
  document.body.dispatchEvent(new CustomEvent('exhibit:artifact-saved'));
}

// A swap replaces the iframe, so the artifact reloads from scratch: any
// snippet pick in flight is against a document that no longer exists. Drop the
// mode rather than leave the button lit over a dead selection.
document.getElementById('pane-preview').addEventListener('htmx:afterSwap', () => {
  if (snippetMode) endSnippetMode();
});

function toggleSnippet() {
  const frame = previewFrame();
  if (!frame) return;
  snippetMode = !snippetMode;
  document.getElementById('snip-btn').classList.toggle('active', snippetMode);
  frame.contentWindow.postMessage({__exSnippet: snippetMode ? 'activate' : 'deactivate'}, '*');
  if (snippetMode) {
    showPane('preview');   // you can't pick an element you can't see
    addMsg('sys', 'Snippet mode: click an element in the preview (Esc to cancel).');
  }
}

// Leave snippet mode without messaging the frame — used when the pick is over
// (captured, cancelled by the artifact, or invalidated by a swap).
function endSnippetMode() {
  snippetMode = false;
  const btn = document.getElementById('snip-btn');
  if (btn) btn.classList.remove('active');
}

document.addEventListener('keydown', (e) => {
  if ((e.key === 'S' || e.key === 's') && e.ctrlKey && e.shiftKey) { e.preventDefault(); toggleSnippet(); }
  if (e.key === 'Escape' && snippetMode) toggleSnippet();
});

// Network permission prompt (av-6xvs): the same dialog the detail page hosts,
// from the same shared module. This pane embeds the same render document
// behind the same sandbox on the same app origin, so it has the trusted chrome
// the prompt needs — it simply never had the prompt, and an artifact reaching
// an unapproved origin while being built here failed silently.
//
// Everything page-specific is here. The frame and the id are resolved on each
// use rather than captured, because both change: htmx replaces #pv-frame after
// every agent save, and there is no artifact at all until the agent creates
// one. reload re-renders the preview pane through that same htmx path rather
// than reassigning src, so the fragment mints a fresh render token and the
// pane keeps its one definition (av-6m3e).
window.ExhibitNetworkPrompt.install({
  frame: previewFrame,
  artifactId: previewArtifactId,
  // The transcript is this page's status line. #pv-title holds the artifact's
  // name and must not be overwritten with a progress message.
  report: function (text) { addMsg('sys', text); },
  reload: refreshPreview
});

// Tell each preview frame the host is listening, so the preamble flushes the
// reports it buffered at load. The frame is a *new element* after every swap,
// so this runs again on each one — without it a violation raised before the
// page noticed the new frame would be lost, which is the failure mode the
// handshake exists to remove.
document.getElementById('pane-preview').addEventListener('htmx:afterSwap', function () {
  window.ExhibitNetworkPrompt.announceTo(previewFrame());
});
window.ExhibitNetworkPrompt.announceTo(previewFrame());

// State bridge (same contract as the detail page): the sandboxed preview
// iframe can't call the API itself, so its storage shim posts writes here
// and this authenticated host forwards them.
window.addEventListener('message', (e) => {
  const d = e.data;
  if (!d || d.__avState !== true || !artifact || d.artifactId !== artifact.id) return;
  const frame = previewFrame();
  if (!frame || e.source !== frame.contentWindow) return;
  // URL construction lives in state-api.js so this and detail.js share one
  // definition — the ".." path-traversal bug (av-hh1o) had to be fixed in
  // three copies of it.
  if (d.op === 'clear') {
    apiFetch(window.ExhibitState.deleteURL(artifact.id), { method: 'DELETE' }).catch(() => {});
  } else if (d.op === 'delete') {
    apiFetch(window.ExhibitState.deleteURL(artifact.id, d.key), { method: 'DELETE' }).catch(() => {});
  } else if (d.op === 'set' || d.op === undefined) {
    // Only a recognized write reaches the API. An unknown op used to fall
    // through to this branch, so a future typo would silently become a write.
    apiFetch(window.ExhibitState.url(artifact.id), {
      method: 'PUT',
      body: JSON.stringify({key: d.key, value: d.value})
    }).catch(() => {});
  }
});

window.addEventListener('message', (e) => {
  const d = e.data;
  if (!d || !d.__exSnippet) return;
  const frame = previewFrame();
  if (!frame || e.source !== frame.contentWindow) return;
  if (d.__exSnippet === 'captured') {
    endSnippetMode();
    const snip = {descriptor: d.descriptor, image: d.image || null, thumbUrl: null};
    if (d.image && d.image.data) {
      snip.thumbUrl = 'data:' + (d.image.mimeType || 'image/png') + ';base64,' + d.image.data;
    }
    pendingSnippets.push(snip);
    renderSnippetChips();
    // The element is attached to the composer, so the next step is typing.
    showPane('chat');
    inputEl.focus();
  } else if (d.__exSnippet === 'cancelled') {
    endSnippetMode();
  }
});

function renderSnippetChips() {
  const wrap = document.getElementById('snippet-chips');
  wrap.innerHTML = '';
  pendingSnippets.forEach((s, i) => {
    const chip = el('div', 'snippet-chip');
    if (s.thumbUrl) {
      const img = document.createElement('img');
      img.src = s.thumbUrl;
      chip.appendChild(img);
    }
    const code = document.createElement('code');
    code.textContent = s.descriptor ? s.descriptor.selector : 'element';
    chip.appendChild(code);
    const x = document.createElement('button');
    x.innerHTML = '<i class="ph ph-x"></i>';
    x.onclick = () => { pendingSnippets.splice(i, 1); renderSnippetChips(); };
    chip.appendChild(x);
    wrap.appendChild(chip);
  });
}
function clearSnippets() { pendingSnippets = []; renderSnippetChips(); }

// --- Brief handoff from /new (nw-d1dd) -------------------------------------
// /new's agent panel is a form, not a chat box, so what arrives here is a set
// of named answers rather than a sentence. It travels in sessionStorage rather
// than a query string: it is the user's own content, and a URL is copied into
// the server's request log, the operator's proxy log and browser history.
const BRIEF_KEY = 'exhibit:agent-brief';

// One entry per brief field: the key it arrives under and the label it wears
// in the opening message. Adding a field to /new's form means adding a line
// here, and that is the whole of the change on this side.
const BRIEF_FIELDS = [
  {key: 'title', label: 'Title'},
  {key: 'description', label: 'What it should do'}
];

// takeBrief reads the brief and removes it in the same breath: a brief is one
// session's opening move, and one left behind would start the *next* session
// on a stale description.
function takeBrief() {
  let raw = null;
  try {
    raw = sessionStorage.getItem(BRIEF_KEY);
    if (raw) sessionStorage.removeItem(BRIEF_KEY);
  } catch (e) {
    return null;   // storage refused (private mode); the composer starts empty
  }
  try { return raw ? JSON.parse(raw) : null; } catch (e) { return null; }
}

// briefToMessage flattens the answers into the first chat message. Fields left
// blank are dropped rather than sent as empty labels, so an optional field the
// user skipped never reads to the model as "no title wanted".
function briefToMessage(brief) {
  const lines = BRIEF_FIELDS
    .filter(f => brief[f.key] && String(brief[f.key]).trim())
    .map(f => f.label + ': ' + String(brief[f.key]).trim());
  return lines.length ? 'Build a self-contained tool.\n\n' + lines.join('\n') : '';
}

// --- Boot -------------------------------------------------------------------
(async function boot() {
  const configured = await refreshKeyStatus();
  const brief = takeBrief();
  if (artifact) {
    // The pane is already showing this artifact — the page render included the
    // same fragment a swap would fetch — so boot only has to say so.
    addMsg('sys', 'Editing "' + artifact.title + '". Describe the change you want — or snippet an element from the preview first.');
    inputEl.placeholder = 'Describe the change to make…';
  } else {
    addMsg('sys', 'Describe a small self-contained tool and the agent will build it and save it to your library.');
  }
  // A brief only opens a *new* build. In modify mode the session already has a
  // subject, and a brief that somehow survived would ask for a second one.
  const opening = (!artifact && brief) ? briefToMessage(brief) : '';
  // Into the composer first, then sent: with no key configured the modal opens
  // over the text and the user gets it back after saving one, rather than
  // watching what they typed on the last page disappear.
  if (opening) { inputEl.value = opening; autoGrow(); }
  if (!configured) { briefAwaitingKey = !!opening; openKeyModal(); return; }
  if (opening) send();
})();
