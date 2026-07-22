package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/momja/Exhibit/internal/color"
)

// agentPage serves the agent chat surface (Exh-jlbt): a build/modify-with-AI
// chat on the left, a live sandboxed preview of the session's artifact on the
// right. `?artifact=<id>` opens the page in modify mode bound to that
// artifact. Like the rest of the gallery it is one server-rendered document
// with vanilla-JS islands; streaming arrives over SSE from the session's Pi
// sidecar.
func (ro *Router) agentPage(w http.ResponseWriter, r *http.Request) {
	artifactJSON := "null"
	if id := r.URL.Query().Get("artifact"); id != "" {
		if a, err := ro.cfg.Store.GetArtifact(r.Context(), id); err == nil && a != nil {
			j, _ := json.Marshal(map[string]string{"id": a.ID, "title": a.Title})
			artifactJSON = string(j)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderAgentPage(ro.cfg.RenderOrigin, ro.cfg.AuthToken, artifactJSON, ro.cfg.MockEnabled, ro.cfg.Agent != nil))
}

func renderAgentPage(renderOrigin, token, artifactJSON string, mockEnabled, agentEnabled bool) string {
	mockOption := ""
	if mockEnabled {
		mockOption = `<option value="exhibit-mock">Exhibit Mock (testing)</option>`
	}
	disabledBanner := ""
	if !agentEnabled {
		disabledBanner = `<div class="banner-err">Agent support is disabled: the <code>pi</code> binary was not found on this server.</div>`
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Agent — Exhibit</title>
` + phosphorCSSLink + `
<style>
:root{--brand-blue:` + color.BrandBlue + `;--brand-blue-hover:` + color.BrandBlueHover + `}
*{box-sizing:border-box;margin:0;padding:0}
[hidden]{display:none!important}
body{font-family:system-ui,sans-serif;background:#f0f0f0;color:#111;display:flex;flex-direction:column;height:100vh}
header{background:#fff;border-bottom:1px solid #e0e0e0;padding:10px 20px;display:flex;align-items:center;gap:12px;flex-shrink:0}
header a{color:var(--brand-blue);text-decoration:none;font-size:13px}
header h1{font-size:16px;font-weight:600;flex:1}
.banner-err{background:#fee;border-bottom:1px solid #fbb;color:#900;padding:8px 20px;font-size:13px}
.key-btn{display:inline-flex;align-items:center;gap:6px;padding:5px 12px;font-size:13px;border:1px solid #ddd;border-radius:6px;background:#fff;cursor:pointer;color:#333}
.key-btn:hover{border-color:var(--brand-blue);color:var(--brand-blue)}
.key-btn.warn{border-color:#e0a800;color:#8a6d00;background:#fff8e1}
.layout{display:flex;flex:1;overflow:hidden}
.chat{width:460px;min-width:360px;display:flex;flex-direction:column;background:#fff;border-right:1px solid #e0e0e0}
#messages{flex:1;overflow-y:auto;padding:16px;display:flex;flex-direction:column;gap:10px}
.msg{max-width:92%;padding:9px 12px;border-radius:10px;font-size:14px;line-height:1.45;white-space:pre-wrap;word-break:break-word}
.msg.user{align-self:flex-end;background:var(--brand-blue);color:#fff;border-bottom-right-radius:3px}
.msg.assistant{align-self:flex-start;background:#f1f2f4;border-bottom-left-radius:3px}
.msg.sys{align-self:center;background:#fff8e1;border:1px solid #ffe58f;color:#664d03;font-size:12.5px;max-width:100%}
.msg.err{align-self:center;background:#fee;border:1px solid #fbb;color:#900;font-size:12.5px;max-width:100%}
.msg .snip-thumb{display:block;max-width:180px;max-height:110px;border-radius:6px;border:1px solid rgba(255,255,255,.5);margin-top:6px}
.tool-chip{align-self:flex-start;display:inline-flex;align-items:center;gap:6px;font-size:12.5px;color:#555;background:#f8f8f8;border:1px solid #e4e4e4;border-radius:999px;padding:4px 12px}
.tool-chip.done{color:#2a7d2a;border-color:#bfe3bf;background:#f2faf2}
.tool-chip.fail{color:#a00;border-color:#f2c4c4;background:#fdf3f3}
.tool-chip i{font-size:13px}
.thinking{align-self:flex-start;color:#999;font-size:12.5px;font-style:italic}
.composer{border-top:1px solid #e0e0e0;padding:10px 12px;display:flex;flex-direction:column;gap:8px;flex-shrink:0}
.pv-nudge{display:none;align-self:center;align-items:center;gap:6px;border:1px solid #c4d7f7;background:#eef4ff;color:var(--brand-blue);border-radius:999px;padding:6px 14px;font:inherit;font-size:13px;cursor:pointer}
#snippet-chips{display:flex;gap:8px;flex-wrap:wrap}
.snippet-chip{display:flex;align-items:center;gap:8px;background:#eef4ff;border:1px solid #c4d7f7;border-radius:8px;padding:5px 8px;font-size:12px;color:#234}
.snippet-chip img{height:36px;max-width:80px;border-radius:4px;border:1px solid #c4d7f7;background:#fff}
.snippet-chip code{background:#dbe7fb;padding:1px 5px;border-radius:3px}
.snippet-chip button{border:none;background:transparent;cursor:pointer;color:#567;display:inline-flex}
.compose-row{display:flex;gap:8px;align-items:flex-end}
#input{flex:1;resize:none;border:1px solid #ddd;border-radius:8px;padding:9px 11px;font-size:14px;font-family:inherit;outline:none;max-height:140px;min-height:40px}
#input:focus{border-color:var(--brand-blue)}
.btn{display:inline-flex;align-items:center;gap:6px;padding:9px 14px;background:var(--brand-blue);color:#fff;border:none;border-radius:8px;font-size:14px;cursor:pointer;font-weight:500}
.btn:hover{background:var(--brand-blue-hover)}
.btn:disabled{background:#b7c3d9;cursor:default}
.btn-stop{background:#c33}
.btn-stop:hover{background:#a22}
#hint{font-size:11.5px;color:#999}
.preview{flex:1;display:flex;flex-direction:column;background:#fafafa}
.preview-bar{background:#fff;border-bottom:1px solid #e0e0e0;padding:8px 16px;display:flex;align-items:center;gap:12px;font-size:13px;flex-shrink:0;min-height:41px}
.preview-bar a{color:var(--brand-blue);text-decoration:none}
.preview-bar .title{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.preview-bar .spacer{flex:1}
.snip-btn{display:inline-flex;align-items:center;gap:6px;padding:4px 12px;font-size:13px;border:1px solid #ddd;border-radius:6px;background:#fff;cursor:pointer}
.snip-btn:hover:not(:disabled){border-color:var(--brand-blue);color:var(--brand-blue)}
.snip-btn:disabled{color:#bbb;cursor:default}
.snip-btn.active{background:var(--brand-blue);color:#fff;border-color:var(--brand-blue)}
.snip-btn .kbd{color:#aaa;font-size:11px}
.snip-more{display:none}
#frame-wrap{flex:1;position:relative}
#frame-wrap iframe{position:absolute;inset:0;width:100%;height:100%;border:none}
#empty-preview{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:8px;color:#999;font-size:14px}
#empty-preview i{font-size:40px;color:#ccc}
.modal-overlay{position:fixed;inset:0;background:rgba(0,0,0,.4);display:flex;align-items:center;justify-content:center;z-index:100}
.modal-overlay[hidden]{display:none}
.modal{background:#fff;border-radius:10px;padding:20px;width:380px;max-width:92vw;box-shadow:0 4px 24px rgba(0,0,0,.25)}
.modal h2{font-size:16px;font-weight:600}
.modal p.note{font-size:12px;color:#777;margin-top:6px;line-height:1.5}
.modal label{display:block;font-size:12px;color:#555;margin:12px 0 4px}
.modal input,.modal select{width:100%;padding:7px 10px;border:1px solid #ddd;border-radius:6px;font-size:14px;outline:none;background:#fff;color:#111}
.modal input:focus,.modal select:focus{border-color:var(--brand-blue)}
.modal input:read-only{background:#f2f2f2;color:#555;cursor:default}
.modal-error{color:#c00;font-size:12px;margin-top:10px}
.modal-actions{display:flex;gap:8px;align-items:center;margin-top:18px}
.btn-sec{background:#fff;color:#333;border:1px solid #ddd}
.btn-sec:hover{border-color:var(--brand-blue);color:var(--brand-blue);background:#fff}
.btn-danger{background:#e00}
.btn-danger:hover{background:#c00}
.spacer{flex:1}
.current-key{font-size:12.5px;color:#555;background:#f6f6f6;border:1px solid #e4e4e4;border-radius:6px;padding:8px 10px;margin-top:10px}

/* Mobile (av-td4y): the two panes don't fit side by side, so a segmented
   control shows one at a time. Everything here is additive — above 640px the
   switch is hidden and the side-by-side sidecar is untouched. */
.pane-switch{display:none;background:#fff;border-bottom:1px solid #e0e0e0;padding:8px 12px;flex-shrink:0}
.pane-tabs{display:flex;gap:4px;background:#f0f1f3;border-radius:10px;padding:4px}
.pane-tab{flex:1;display:inline-flex;align-items:center;justify-content:center;gap:7px;padding:9px 10px;border:none;border-radius:8px;background:transparent;color:#666;font:inherit;font-size:15px;font-weight:600;cursor:pointer}
.pane-tab[aria-selected="true"]{background:#fff;color:#111;box-shadow:0 1px 2px rgba(0,0,0,.14)}
.pane-tab .dot{display:none;width:8px;height:8px;border-radius:50%;background:var(--brand-blue)}
.pane-tab.has-update .dot{display:block}
@media (max-width:640px){
  /* dvh keeps the composer above the mobile URL bar instead of under it. */
  body{height:100dvh}
  header{padding:10px 12px;gap:10px}
  header a{font-size:18px;line-height:1}
  header a .lbl{display:none}
  header h1{font-size:15px}
  .key-btn{max-width:45vw;white-space:nowrap;overflow:hidden}
  #key-btn-label{overflow:hidden;text-overflow:ellipsis}
  .pane-switch{display:block}
  .chat,.preview{width:100%;min-width:0;flex:1}
  .chat{border-right:none}
  .preview{display:none}
  body.show-preview .chat{display:none}
  body.show-preview .preview{display:flex}
  .pv-nudge{display:inline-flex}
  #hint{display:none}
  /* The preview bar becomes a bottom action bar led by the snippet button. */
  #frame-wrap{order:1}
  .preview-bar{order:2;border-bottom:none;border-top:1px solid #e0e0e0;padding:10px 12px}
  .preview-bar .title,.preview-bar .spacer,.preview-bar #pv-detail{display:none}
  .preview-bar a{display:inline-flex;align-items:center;justify-content:center;width:46px;height:42px;border:1px solid #ddd;border-radius:8px;font-size:16px}
  .preview-bar a .lbl{display:none}
  .snip-btn{order:-1;flex:1;justify-content:center;height:42px;font-size:15px;font-weight:500;background:var(--brand-blue);border-color:var(--brand-blue);color:#fff}
  .snip-btn:disabled{background:#b9c4d8;border-color:#b9c4d8;color:#f2f5fb}
  .snip-btn.active{background:var(--brand-blue-hover);box-shadow:inset 0 0 0 2px rgba(255,255,255,.6)}
  .snip-btn .kbd{display:none}
  .snip-more{display:inline}
}
</style>
</head>
<body>
<header>
  <a href="/">←<span class="lbl"> Gallery</span></a>
  <h1><i class="ph ph-robot"></i> Agent</h1>
  <button class="key-btn" id="key-btn" onclick="openKeyModal()"><i class="ph ph-key"></i> <span id="key-btn-label">API key</span></button>
</header>
` + disabledBanner + `
<div class="pane-switch">
  <div class="pane-tabs" role="tablist" aria-label="Chat or preview">
    <button type="button" class="pane-tab" id="tab-chat" role="tab" aria-selected="true" aria-controls="pane-chat" onclick="showPane('chat')"><i class="ph ph-chat-circle"></i> Chat</button>
    <button type="button" class="pane-tab" id="tab-preview" role="tab" aria-selected="false" aria-controls="pane-preview" onclick="showPane('preview')"><i class="ph ph-monitor"></i> Preview <span class="dot" aria-hidden="true"></span></button>
  </div>
</div>
<div class="layout">
  <div class="chat" id="pane-chat">
    <div id="messages"></div>
    <div class="composer">
      <button type="button" class="pv-nudge" id="pv-nudge" hidden onclick="showPane('preview')"><i class="ph ph-check-circle"></i> Preview updated — tap Preview to view</button>
      <div id="snippet-chips"></div>
      <div class="compose-row">
        <textarea id="input" rows="1" placeholder="Describe the tool to build…"></textarea>
        <button class="btn" id="send-btn" onclick="send()"><i class="ph ph-paper-plane-right"></i></button>
        <button class="btn btn-stop" id="stop-btn" onclick="stopAgent()" style="display:none"><i class="ph ph-stop"></i></button>
      </div>
      <div id="hint">Enter to send · Shift+Enter for a new line · Ctrl+Shift+S to snippet an element from the preview</div>
    </div>
  </div>
  <div class="preview" id="pane-preview">
    <div class="preview-bar">
      <span class="title" id="pv-title">No artifact yet</span>
      <a id="pv-open" href="#" target="_blank" style="display:none"><span class="lbl">Open </span>↗</a>
      <a id="pv-detail" href="#" style="display:none">Details</a>
      <span class="spacer"></span>
      <button class="snip-btn" id="snip-btn" disabled onclick="toggleSnippet()"><i class="ph ph-scissors"></i> Snippet<span class="snip-more">an element</span> <span class="kbd">Ctrl+Shift+S</span></button>
    </div>
    <div id="frame-wrap">
      <div id="empty-preview"><i class="ph ph-frame-corners"></i><span>The artifact preview appears here once the agent saves one.</span></div>
      <iframe id="pv-frame" sandbox="allow-scripts" allow="clipboard-read; clipboard-write" style="display:none"></iframe>
    </div>
  </div>
</div>

<div id="key-modal" class="modal-overlay" hidden>
  <div class="modal" role="dialog" aria-modal="true">
    <h2>Agent API key</h2>
    <p class="note">Bring your own key. It is sent to your Exhibit server once, encrypted there at rest, and used only by the server-side agent — this page never sees it again.</p>
    <div class="current-key" id="current-key" hidden></div>
    <label for="key-provider">Provider</label>
    <select id="key-provider" onchange="providerChanged()">
      <option value="anthropic">Anthropic</option>
      <option value="openai">OpenAI</option>
      <option value="google">Google Gemini</option>
      <option value="openrouter">OpenRouter</option>
      <option value="opencode-go">OpenCode Go</option>
      ` + mockOption + `
    </select>
    <label for="key-model">Model</label>
    <input type="text" id="key-model" list="model-suggestions" placeholder="e.g. claude-sonnet-4-5">
    <datalist id="model-suggestions"></datalist>
    <label for="key-secret">API key</label>
    <input type="password" id="key-secret" placeholder="sk-…" autocomplete="off">
    <div id="key-error" class="modal-error" hidden></div>
    <div class="modal-actions">
      <button type="button" class="btn btn-danger" id="key-delete" onclick="deleteKey()" hidden><i class="ph ph-trash"></i></button>
      <span class="spacer"></span>
      <button type="button" class="btn btn-sec" onclick="closeKeyModal()">Cancel</button>
      <button type="button" class="btn" onclick="saveKey()"><i class="ph ph-check"></i> Save</button>
    </div>
  </div>
</div>

<script>
const TOKEN = ` + fmt.Sprintf("%q", token) + `;
const RENDER_ORIGIN = ` + fmt.Sprintf("%q", renderOrigin) + `;
let artifact = ` + artifactJSON + `;   // {id,title} when editing, else null

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
</script>
</body>
</html>`
}
