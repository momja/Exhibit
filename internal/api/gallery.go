package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/go-chi/chi/v5"
)

func (ro *Router) galleryIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	arts, err := ro.cfg.Store.ListArtifacts(r.Context(), store.ListOptions{Query: q, Limit: 100})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if arts == nil {
		arts = []*store.Artifact{}
	}

	tags, _ := ro.cfg.Store.ListTags(r.Context(), 1)
	cols, _ := ro.cfg.Store.ListCollections(r.Context(), 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderGalleryPage(arts, tags, cols, q, ro.cfg.RenderOrigin, ro.cfg.AuthToken))
}

func (ro *Router) galleryDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil || a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		http.Error(w, "blob not found", http.StatusInternalServerError)
		return
	}
	defer rc.Close()
	src, _ := io.ReadAll(rc)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderDetailPage(a, string(src), ro.cfg.RenderOrigin, ro.cfg.AuthToken))
}

func (ro *Router) galleryEdit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil || a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		http.Error(w, "blob not found", http.StatusInternalServerError)
		return
	}
	defer rc.Close()
	src, _ := io.ReadAll(rc)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderEditPage(a, string(src), ro.cfg.AuthToken))
}

func renderGalleryPage(arts []*store.Artifact, tags []*store.Tag, cols []*store.Collection, query, renderOrigin, token string) string {
	var cards strings.Builder
	if len(arts) == 0 {
		cards.WriteString(`<p class="empty">No artifacts yet. Upload one above.</p>`)
	}
	for _, a := range arts {
		cards.WriteString(fmt.Sprintf(`
<div class="card">
  <a class="card-title" href="/artifacts/%s">%s</a>
  <div class="card-meta">%s</div>
  <div class="card-actions">
    <a href="/artifacts/%s">Details</a>
    <a href="%s/a/%s" target="_blank">Open ↗</a>
  </div>
</div>`, a.ID, htmlEsc(a.Title), a.CreatedAt.Format("Jan 2, 2006"), a.ID, renderOrigin, a.ID))
	}

	searchVal := htmlEsc(query)

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Artifact Viewer</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;background:#f0f0f0;color:#111;min-height:100vh}
header{background:#fff;border-bottom:1px solid #e0e0e0;padding:12px 24px;display:flex;align-items:center;gap:16px}
header h1{font-size:18px;font-weight:600}
header a{color:#0070f3;text-decoration:none;font-size:14px}
main{padding:24px;max-width:1200px;margin:0 auto}
.upload{background:#fff;border-radius:10px;padding:20px;margin-bottom:24px;box-shadow:0 1px 3px rgba(0,0,0,.08)}
.upload h2{font-size:15px;font-weight:600;margin-bottom:12px;color:#333}
.upload textarea{width:100%;height:160px;font-family:monospace;font-size:12px;border:1px solid #ddd;border-radius:6px;padding:10px;resize:vertical;outline:none}
.upload textarea:focus{border-color:#0070f3}
.upload input[type=text]{width:100%;padding:8px 10px;border:1px solid #ddd;border-radius:6px;font-size:14px;margin-bottom:8px;outline:none}
.upload input[type=text]:focus{border-color:#0070f3}
.upload-row{display:flex;gap:8px;margin-top:10px}
.btn{padding:8px 18px;background:#0070f3;color:#fff;border:none;border-radius:6px;font-size:14px;cursor:pointer;font-weight:500}
.btn:hover{background:#005ed4}
.btn-sm{padding:5px 12px;font-size:13px}
.search-row{display:flex;gap:8px;margin-bottom:20px}
.search-row input{flex:1;padding:9px 12px;border:1px solid #ddd;border-radius:6px;font-size:14px;outline:none}
.search-row input:focus{border-color:#0070f3}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:16px}
.card{background:#fff;border-radius:10px;padding:16px;box-shadow:0 1px 3px rgba(0,0,0,.08);display:flex;flex-direction:column;gap:8px}
.card-title{font-size:15px;font-weight:600;color:#0070f3;text-decoration:none;word-break:break-word}
.card-title:hover{text-decoration:underline}
.card-meta{font-size:12px;color:#888}
.card-actions{display:flex;gap:12px;font-size:13px}
.card-actions a{color:#555;text-decoration:none}
.card-actions a:hover{color:#0070f3}
.empty{color:#888;font-size:14px;padding:20px 0}
#status{margin-top:10px;font-size:13px;color:#555}
#scan-result{margin-top:10px;background:#f8f8f8;border:1px solid #e0e0e0;border-radius:6px;padding:12px;font-size:13px;display:none}
.mode-tabs{display:flex;gap:6px;margin-bottom:8px}
.tab-btn{padding:5px 14px;font-size:13px;border:1px solid #ddd;border-radius:5px;background:#fff;cursor:pointer;color:#555}
.tab-btn.active{background:#0070f3;color:#fff;border-color:#0070f3}
</style>
</head>
<body>
<header>
  <h1>Artifact Viewer</h1>
</header>
<main>
<div class="upload">
  <h2>Upload Artifact</h2>
  <input type="text" id="title" placeholder="Title (optional)">
  <div class="mode-tabs">
    <button class="tab-btn active" onclick="setMode('paste')">Paste HTML</button>
    <button class="tab-btn" onclick="setMode('url')">From URL</button>
  </div>
  <textarea id="body" placeholder="Paste HTML artifact source here…"></textarea>
  <input type="text" id="url-input" placeholder="https://example.com/tool.html" style="display:none;width:100%;padding:8px 10px;border:1px solid #ddd;border-radius:6px;font-size:14px;outline:none;margin-bottom:8px">
  <div id="scan-result"></div>
  <div class="upload-row">
    <button class="btn" onclick="ingest()">Upload</button>
  </div>
  <div id="status"></div>
</div>

<form class="search-row" method="GET" action="/">
  <input type="text" name="q" placeholder="Search…" value="` + searchVal + `">
  <button class="btn btn-sm" type="submit">Search</button>
</form>

<div class="grid">` + cards.String() + `</div>
</main>

<script>
const TOKEN = ` + fmt.Sprintf("%q", token) + `;
let currentMode = 'paste';

function setMode(mode) {
  currentMode = mode;
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  event.target.classList.add('active');
  document.getElementById('body').style.display = mode === 'paste' ? 'block' : 'none';
  document.getElementById('url-input').style.display = mode === 'url' ? 'block' : 'none';
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
    payload = {title: title || '', url, network_allowlist: []};
  } else {
    const body = document.getElementById('body').value.trim();
    if (!body) { status.textContent = 'Paste an artifact first.'; return; }
    payload = {title: title || 'Untitled', body, network_allowlist: []};
  }

  const resp = await fetch('/api/artifacts', {
    method: 'POST',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify(payload)
  });
  const data = await resp.json();
  if (!resp.ok) { status.textContent = 'Error: ' + (data.error || resp.statusText); return; }

  const id = data.artifact.id;
  const footprint = data.network_footprint || [];
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

// showApproval presents the scanned origins for explicit approval. Origins are
// blocked until the user approves them; nothing is written to the allowlist here.
function showApproval(id, footprint) {
  const scanDiv = document.getElementById('scan-result');
  const rows = footprint.map(o =>
    '<label style="display:block;margin:4px 0">' +
    '<input type="checkbox" class="al-origin" value="' + o + '" checked> ' +
    '<code style="background:#eee;padding:1px 5px;border-radius:3px">' + o + '</code></label>'
  ).join('');
  scanDiv.style.display = 'block';
  scanDiv.innerHTML =
    '<strong>This artifact wants to contact these origins.</strong>' +
    '<div style="color:#a00;margin:4px 0 8px">They are blocked until you approve them.</div>' +
    rows +
    '<div class="upload-row">' +
    '<button class="btn btn-sm" onclick="approveOrigins(\'' + id + '\')">Approve selected &amp; enable</button>' +
    '<button class="btn btn-sm" style="background:#888" onclick="finishIngest(\'' + id + '\')">Keep all blocked</button>' +
    '</div>';
}

// approveOrigins writes the user-selected origins to the artifact's allowlist.
async function approveOrigins(id) {
  const selected = Array.from(document.querySelectorAll('.al-origin:checked')).map(c => c.value);
  const status = document.getElementById('status');
  status.textContent = 'Applying…';
  const r = await fetch('/api/artifacts/' + id, {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({network_allowlist: selected})
  });
  if (!r.ok) { status.textContent = '✗ Failed to update allowlist'; return; }
  finishIngest(id);
}

function finishIngest(id) {
  const status = document.getElementById('status');
  status.textContent = '✓ Saved — ';
  const link = document.createElement('a');
  link.href = '/artifacts/' + id;
  link.textContent = 'View artifact';
  status.appendChild(link);
  setTimeout(() => location.reload(), 1200);
}
</script>
</body>
</html>`
}

func renderDetailPage(a *store.Artifact, src, renderOrigin, token string) string {
	allowlistJSON := "[]"
	if len(a.NetworkAllowlist) > 0 {
		parts := make([]string, len(a.NetworkAllowlist))
		for i, o := range a.NetworkAllowlist {
			parts[i] = fmt.Sprintf("%q", o)
		}
		allowlistJSON = "[" + strings.Join(parts, ",") + "]"
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + htmlEsc(a.Title) + ` — Artifact Viewer</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;background:#f0f0f0;color:#111;display:flex;flex-direction:column;height:100vh}
header{background:#fff;border-bottom:1px solid #e0e0e0;padding:10px 20px;display:flex;align-items:center;gap:12px;flex-shrink:0}
header a{color:#0070f3;text-decoration:none;font-size:13px}
header h1{font-size:16px;font-weight:600;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.toolbar{background:#fff;border-bottom:1px solid #e0e0e0;padding:8px 20px;display:flex;gap:12px;align-items:center;flex-shrink:0;font-size:13px}
.toolbar a{color:#0070f3;text-decoration:none}
.toolbar button{padding:4px 12px;font-size:13px;border:1px solid #ddd;border-radius:5px;background:#fff;cursor:pointer}
.toolbar button:hover{border-color:#0070f3;color:#0070f3}
.panels{display:flex;flex:1;overflow:hidden;gap:0}
.panel{flex:1;overflow:auto;background:#fff}
.panel+.panel{border-left:1px solid #e0e0e0}
iframe{width:100%;height:100%;border:none;display:block}
pre{padding:16px;font-family:monospace;font-size:12px;line-height:1.5;white-space:pre-wrap;word-break:break-all;color:#333;background:#fafafa;min-height:100%}
.allowlist{padding:8px 20px;background:#fffbe6;border-bottom:1px solid #ffe58f;font-size:13px;flex-shrink:0}
.allowlist code{background:#fff3;padding:1px 5px;border-radius:3px;font-size:12px}
.allowlist input{border:1px solid #ddd;border-radius:4px;padding:3px 8px;font-size:12px;width:240px;margin:0 6px}
</style>
</head>
<body>
<header>
  <a href="/">← Gallery</a>
  <h1>` + htmlEsc(a.Title) + `</h1>
  <span style="font-size:12px;color:#888">` + a.CreatedAt.Format("Jan 2, 2006 15:04") + `</span>
</header>
<div class="toolbar">
  <a href="` + renderOrigin + `/a/` + a.ID + `" target="_blank">Open in new tab ↗</a>
  <span style="color:#ddd">|</span>
  <a href="/artifacts/` + a.ID + `/edit">Edit</a>
  <span style="color:#ddd">|</span>
  <span style="color:#888">Allowlist:</span>
  <span id="al-display">` + renderAllowlistBadges(a.NetworkAllowlist) + `</span>
  <input id="al-input" type="text" placeholder="Add origin (https://example.com)" style="display:none">
  <button onclick="addOrigin()">+ Add origin</button>
  <span id="al-status" style="color:#888"></span>
</div>
<div class="panels">
  <iframe src="` + renderOrigin + `/a/` + a.ID + `" sandbox="allow-scripts" loading="lazy"></iframe>
  <div class="panel"><pre>` + htmlEsc(src) + `</pre></div>
</div>
<script>
const TOKEN = ` + fmt.Sprintf("%q", token) + `;
const ID = ` + fmt.Sprintf("%q", a.ID) + `;
let allowlist = ` + allowlistJSON + `;

function renderBadges() {
  document.getElementById('al-display').innerHTML = allowlist.length
    ? allowlist.map(o => '<code>' + o + '</code> ').join('')
    : '<span style="color:#aaa">none</span>';
}

async function addOrigin() {
  const inp = document.getElementById('al-input');
  if (inp.style.display === 'none') { inp.style.display='inline-block'; inp.focus(); return; }
  const val = inp.value.trim();
  if (!val) { inp.style.display='none'; return; }
  allowlist.push(val);
  inp.value = '';
  inp.style.display = 'none';
  await saveAllowlist();
}

async function saveAllowlist() {
  const st = document.getElementById('al-status');
  st.textContent = 'Saving…';
  const r = await fetch('/api/artifacts/' + ID, {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({network_allowlist: allowlist})
  });
  st.textContent = r.ok ? '✓ Saved — reload to apply' : '✗ Error';
  renderBadges();
}
</script>
</body>
</html>`
}

func renderAllowlistBadges(list []string) string {
	if len(list) == 0 {
		return `<span style="color:#aaa">none</span>`
	}
	var b strings.Builder
	for _, o := range list {
		b.WriteString(`<code style="background:#f0f0f0;padding:1px 6px;border-radius:3px;margin-right:4px">` + htmlEsc(o) + `</code>`)
	}
	return b.String()
}

func renderEditPage(a *store.Artifact, src, token string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Edit: ` + htmlEsc(a.Title) + ` — Artifact Viewer</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;background:#f0f0f0;color:#111;min-height:100vh}
header{background:#fff;border-bottom:1px solid #e0e0e0;padding:12px 24px;display:flex;align-items:center;gap:16px}
header h1{font-size:18px;font-weight:600;flex:1}
header a{color:#0070f3;text-decoration:none;font-size:14px}
main{padding:24px;max-width:900px;margin:0 auto}
.edit-card{background:#fff;border-radius:10px;padding:20px;box-shadow:0 1px 3px rgba(0,0,0,.08)}
.edit-card h2{font-size:15px;font-weight:600;margin-bottom:12px;color:#333}
.edit-card input[type=text]{width:100%;padding:8px 10px;border:1px solid #ddd;border-radius:6px;font-size:14px;margin-bottom:10px;outline:none}
.edit-card input[type=text]:focus{border-color:#0070f3}
.edit-card textarea{width:100%;height:400px;font-family:monospace;font-size:12px;border:1px solid #ddd;border-radius:6px;padding:10px;resize:vertical;outline:none}
.edit-card textarea:focus{border-color:#0070f3}
.btn-row{display:flex;gap:8px;margin-top:12px;align-items:center}
.btn{padding:8px 18px;background:#0070f3;color:#fff;border:none;border-radius:6px;font-size:14px;cursor:pointer;font-weight:500}
.btn:hover{background:#005ed4}
.btn-sec{background:#fff;color:#333;border:1px solid #ddd}
.btn-sec:hover{border-color:#0070f3;color:#0070f3;background:#fff}
#status{font-size:13px;color:#555}
</style>
</head>
<body>
<header>
  <a href="/artifacts/` + a.ID + `">← Back</a>
  <h1>Edit Artifact</h1>
</header>
<main>
<div class="edit-card">
  <h2>Edit</h2>
  <input type="text" id="title" value="` + htmlEsc(a.Title) + `" placeholder="Title">
  <textarea id="body">` + htmlEsc(src) + `</textarea>
  <div class="btn-row">
    <button class="btn" onclick="save()">Save</button>
    <a href="/artifacts/` + a.ID + `"><button class="btn btn-sec" type="button">Cancel</button></a>
    <span id="status"></span>
  </div>
</div>
</main>
<script>
const TOKEN = ` + fmt.Sprintf("%q", token) + `;
const ID = ` + fmt.Sprintf("%q", a.ID) + `;

async function save() {
  const title = document.getElementById('title').value.trim();
  const body  = document.getElementById('body').value;
  const status = document.getElementById('status');
  if (!body.trim()) { status.textContent = 'Body cannot be empty.'; return; }
  status.textContent = 'Saving…';
  const resp = await fetch('/api/artifacts/' + ID, {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({title: title || 'Untitled', body})
  });
  if (!resp.ok) {
    const data = await resp.json().catch(() => ({}));
    status.textContent = '✗ Error: ' + (data.error || resp.statusText);
    return;
  }
  status.textContent = '✓ Saved';
  setTimeout(() => { window.location.href = '/artifacts/' + ID; }, 500);
}
</script>
</body>
</html>`
}

func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}
