/* Agent chat surface script (Exh-jlbt). Served from the app origin at
 * /assets/gallery/agent.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads (and reassigns) before it loads:
 *   TOKEN         - API bearer token
 *   RENDER_ORIGIN - the render surface origin, for the preview iframe/links
 *   artifact      - {id,title} when opened in modify mode, else null (mutable)
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

const messagesEl = document.getElementById('messages');
const inputEl = document.getElementById('input');
const frameEl = document.getElementById('pv-frame');

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

async function apiFetch(path, opts) {
  opts = opts || {};
  opts.headers = Object.assign({'Authorization':'Bearer '+TOKEN}, opts.headers || {});
  if (opts.body) opts.headers['Content-Type'] = 'application/json';
  return fetch(path, opts);
}

// --- API key management --------------------------------------------------
async function refreshKeyStatus() {
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
  if (eventSource) { eventSource.close(); eventSource = null; }
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
  connectEvents();
  return true;
}

function connectEvents() {
  eventSource = new EventSource('/api/agent/sessions/' + sessionId + '/events?token=' + encodeURIComponent(TOKEN));
  eventSource.onmessage = (e) => {
    let ev;
    try { ev = JSON.parse(e.data); } catch { return; }
    handleAgentEvent(ev);
  };
  eventSource.onerror = () => { /* EventSource retries automatically */ };
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
        thinkingEl = el('div', 'thinking', 'thinking…');
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
      artifact = {id: ev.artifactId, title: ev.title || 'Artifact'};
      showArtifact(ev.artifactId, artifact.title);
      nudgePreview();
      let note = (ev.action === 'created' ? 'Artifact created' : 'Artifact updated') +
        (mobileQuery.matches ? ' — tap Preview to view it.' : ' — preview on the right.');
      if (ev.footprint && ev.footprint.length) {
        note += ' It references external origins (' + ev.footprint.join(', ') + '); they stay blocked until you approve them on the artifact page.';
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
  let message = text;
  pendingSnippets.forEach((s, i) => {
    message += '\n\n[Snippet ' + (i + 1) + '] The user selected this element in the current artifact' +
      (s.image ? ' (screenshot attached)' : '') + ':\n' + describeSnippet(s.descriptor);
  });

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
    body: JSON.stringify({message, images})
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
function showArtifact(id, title) {
  document.getElementById('pv-title').textContent = title || 'Artifact';
  const open = document.getElementById('pv-open');
  open.href = RENDER_ORIGIN + '/a/' + id;
  open.style.display = '';
  const detail = document.getElementById('pv-detail');
  detail.href = '/artifacts/' + id;
  detail.style.display = '';
  document.getElementById('empty-preview').style.display = 'none';
  frameEl.style.display = '';
  // The render doc is no-store; a fresh query string forces the iframe to
  // re-fetch it after every agent save.
  frameEl.src = RENDER_ORIGIN + '/a/' + id + '?r=' + Date.now();
  document.getElementById('snip-btn').disabled = false;
}

function toggleSnippet() {
  if (!frameEl.src) return;
  snippetMode = !snippetMode;
  document.getElementById('snip-btn').classList.toggle('active', snippetMode);
  frameEl.contentWindow.postMessage({__exSnippet: snippetMode ? 'activate' : 'deactivate'}, '*');
  if (snippetMode) {
    showPane('preview');   // you can't pick an element you can't see
    addMsg('sys', 'Snippet mode: click an element in the preview (Esc to cancel).');
  }
}

document.addEventListener('keydown', (e) => {
  if ((e.key === 'S' || e.key === 's') && e.ctrlKey && e.shiftKey) { e.preventDefault(); toggleSnippet(); }
  if (e.key === 'Escape' && snippetMode) toggleSnippet();
});

// State bridge (same contract as the detail page): the sandboxed preview
// iframe can't call the API itself, so its storage shim posts writes here
// and this authenticated host forwards them.
window.addEventListener('message', (e) => {
  const d = e.data;
  if (!d || d.__avState !== true || !artifact || d.artifactId !== artifact.id) return;
  if (e.source !== frameEl.contentWindow) return;
  apiFetch('/api/artifacts/' + encodeURIComponent(artifact.id) + '/state', {
    method: 'PUT',
    body: JSON.stringify({key: d.key, value: d.value})
  }).catch(() => {});
});

window.addEventListener('message', (e) => {
  const d = e.data;
  if (!d || !d.__exSnippet || e.source !== frameEl.contentWindow) return;
  if (d.__exSnippet === 'captured') {
    snippetMode = false;
    document.getElementById('snip-btn').classList.remove('active');
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
    snippetMode = false;
    document.getElementById('snip-btn').classList.remove('active');
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

// --- Boot -------------------------------------------------------------------
(async function boot() {
  const configured = await refreshKeyStatus();
  if (artifact) {
    showArtifact(artifact.id, artifact.title);
    addMsg('sys', 'Editing "' + artifact.title + '". Describe the change you want — or snippet an element from the preview first.');
    inputEl.placeholder = 'Describe the change to make…';
  } else {
    addMsg('sys', 'Describe a small self-contained tool and the agent will build it and save it to your library.');
  }
  if (!configured) openKeyModal();
})();
