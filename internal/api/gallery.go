package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/artifact-viewer/artifact-viewer/internal/color"
	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/go-chi/chi/v5"
)

func (ro *Router) galleryIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	arts, err := ro.cfg.Store.ListArtifacts(r.Context(), store.ListOptions{Query: q, Limit: 100})
	if err != nil {
		serverError(w, r, "gallery index list artifacts", err)
		return
	}
	if arts == nil {
		arts = []*store.Artifact{}
	}

	tags, _ := ro.cfg.Store.ListTags(r.Context(), 1)
	cols, _ := ro.cfg.Store.ListCollections(r.Context(), 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderGalleryPage(arts, tags, cols, q, ro.cfg.AuthToken))
}

func (ro *Router) galleryDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		serverError(w, r, "gallery detail lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		serverError(w, r, "gallery detail blob", err)
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
	if err != nil {
		serverError(w, r, "gallery edit lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		serverError(w, r, "gallery edit blob", err)
		return
	}
	defer rc.Close()
	src, _ := io.ReadAll(rc)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderEditPage(a, string(src), ro.cfg.AuthToken))
}

func renderGalleryPage(arts []*store.Artifact, tags []*store.Tag, cols []*store.Collection, query, token string) string {
	var cards strings.Builder
	if len(arts) == 0 {
		cards.WriteString(`<p class="empty">No artifacts yet. Upload one above.</p>`)
	}
	// The whole card opens the artifact's detail/viewer page; the explicit
	// 'Details' action does the same. The separate 'Open' card action was
	// removed so there is exactly one way to open an artifact from a card.
	for _, a := range arts {
		cards.WriteString(fmt.Sprintf(`
<div class="card" data-href="/artifacts/%s">
  <a class="card-title" href="/artifacts/%s">%s</a>
  <div class="card-meta">%s</div>
  %s
  <div class="card-actions">
    <a href="/artifacts/%s">Details</a>
  </div>
</div>`, a.ID, a.ID, htmlEsc(a.Title), a.CreatedAt.Format("Jan 2, 2006"), renderTagRow(a.ID, a.Tags), a.ID))
	}

	searchVal := htmlEsc(query)

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Exhibit</title>
<link rel="icon" type="image/svg+xml" href="` + exhibitFaviconDataURI + `">
` + phosphorCSSLink + `
<style>
:root{--brand-blue:` + color.BrandBlue + `;--brand-blue-hover:` + color.BrandBlueHover + `}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;background:#f0f0f0;color:#111;min-height:100vh}
header{background:#fff;border-bottom:1px solid #e0e0e0;padding:12px 24px;display:flex;align-items:center;gap:16px}
header h1{font-size:18px;font-weight:600}
header .logo{height:32px;width:auto;display:block;flex:0 0 auto}
header a{color:var(--brand-blue);text-decoration:none;font-size:14px}
main{padding:24px;max-width:1200px;margin:0 auto}
.upload{background:#fff;border-radius:10px;padding:20px;margin-bottom:24px;box-shadow:0 1px 3px rgba(0,0,0,.08)}
.upload h2{font-size:15px;font-weight:600;margin-bottom:12px;color:#333}
.upload textarea{width:100%;height:160px;font-family:monospace;font-size:12px;border:1px solid #ddd;border-radius:6px;padding:10px;resize:vertical;outline:none}
.upload textarea:focus{border-color:var(--brand-blue)}
.upload input[type=text]{width:100%;padding:8px 10px;border:1px solid #ddd;border-radius:6px;font-size:14px;margin-bottom:8px;outline:none}
.upload input[type=text]:focus{border-color:var(--brand-blue)}
.upload-row{display:flex;gap:8px;margin-top:10px}
.btn{display:inline-flex;align-items:center;gap:6px;padding:8px 18px;background:var(--brand-blue);color:#fff;border:none;border-radius:6px;font-size:14px;cursor:pointer;font-weight:500}
.btn:hover{background:var(--brand-blue-hover)}
.btn-sm{padding:5px 12px;font-size:13px}
.search-row{display:flex;gap:8px;margin-bottom:20px}
.search-row input{flex:1;padding:9px 12px;border:1px solid #ddd;border-radius:6px;font-size:14px;outline:none}
.search-row input:focus{border-color:var(--brand-blue)}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:16px}
.card{background:#fff;border-radius:10px;padding:16px;box-shadow:0 1px 3px rgba(0,0,0,.08);display:flex;flex-direction:column;gap:8px;cursor:pointer;transition:box-shadow .12s ease}
.card:hover{box-shadow:0 2px 8px rgba(0,0,0,.12)}
.card-title{font-size:15px;font-weight:600;color:var(--brand-blue);text-decoration:none;word-break:break-word}
.card-title:hover{text-decoration:underline}
.card-meta{font-size:12px;color:#888}
.card-actions{display:flex;gap:12px;font-size:13px}
.card-actions a{color:#555;text-decoration:none}
.card-actions a:hover{color:var(--brand-blue)}
.tag-row{display:flex;align-items:center;flex-wrap:wrap;gap:6px}
.tag-add-btn{display:inline-flex;align-items:center;justify-content:center;flex:0 0 auto;width:20px;height:20px;padding:0;border:1px dashed #ccc;border-radius:50%;background:transparent;color:#888;cursor:pointer}
.tag-add-btn:hover,.tag-add-btn:focus-visible{border-color:var(--brand-blue);border-style:solid;color:var(--brand-blue)}
.tag-add-btn i{font-size:11px;line-height:1}
.tag-pills{display:flex;flex-wrap:wrap;gap:6px;list-style:none}
.tag-pill{display:inline-flex;align-items:center;gap:4px;max-width:100%;padding:3px 8px;border-radius:999px;font-size:12px;font-weight:600;line-height:1.4}
.tag-pill-label{overflow-wrap:anywhere}
.tag-pill-edit,.tag-pill-detach{display:inline-flex;align-items:center;justify-content:center;flex:0 0 auto;width:14px;height:14px;padding:0;border:none;border-radius:50%;background:transparent;color:inherit;font:inherit;cursor:pointer;opacity:0;pointer-events:none;transition:opacity .12s ease,background .12s ease}
.tag-pill-edit i,.tag-pill-detach i{font-size:11px;line-height:1}
.tag-pill:hover .tag-pill-edit,.tag-pill:hover .tag-pill-detach,
.tag-pill:focus-within .tag-pill-edit,.tag-pill:focus-within .tag-pill-detach,
.tag-pill-edit:focus-visible,.tag-pill-detach:focus-visible{opacity:1;pointer-events:auto}
.tag-pill-edit:hover,.tag-pill-detach:hover{background:rgba(0,0,0,.15)}
.empty{color:#888;font-size:14px;padding:20px 0}
#status{margin-top:10px;font-size:13px;color:#555}
#scan-result{margin-top:10px;background:#f8f8f8;border:1px solid #e0e0e0;border-radius:6px;padding:12px;font-size:13px;display:none}
.mode-tabs{display:flex;gap:6px;margin-bottom:8px}
.tab-btn{padding:5px 14px;font-size:13px;border:1px solid #ddd;border-radius:5px;background:#fff;cursor:pointer;color:#555}
.tab-btn.active{background:var(--brand-blue);color:#fff;border-color:var(--brand-blue)}
.btn-sec{background:#fff;color:#333;border:1px solid #ddd}
.btn-sec:hover{border-color:var(--brand-blue);color:var(--brand-blue);background:#fff}
.btn-danger{background:#e00;color:#fff;border:none}
.btn-danger:hover{background:#c00}
.spacer{flex:1}
.modal-overlay{position:fixed;inset:0;background:rgba(0,0,0,.4);display:flex;align-items:center;justify-content:center;z-index:100}
.modal-overlay[hidden]{display:none}
.modal{background:#fff;border-radius:10px;padding:20px;width:320px;max-width:90vw;box-shadow:0 4px 24px rgba(0,0,0,.25)}
.modal h2{font-size:16px;font-weight:600}
.modal label{display:block;font-size:12px;color:#555;margin:12px 0 4px}
.modal input[type=text],.modal select{width:100%;padding:7px 10px;border:1px solid #ddd;border-radius:6px;font-size:14px;outline:none;background:#fff;color:#111}
.modal input[type=text]:focus,.modal select:focus{border-color:var(--brand-blue)}
.color-presets{display:flex;gap:6px;flex-wrap:wrap;margin-top:4px}
.color-swatch{width:22px;height:22px;border-radius:50%;border:2px solid transparent;cursor:pointer;padding:0}
.color-swatch.selected{border-color:#333}
.color-custom-row{display:flex;gap:8px;align-items:center;margin-top:8px}
.color-custom-row input[type=color]{width:36px;height:30px;padding:0;border:1px solid #ddd;border-radius:6px;cursor:pointer}
.color-custom-row input[type=text]{flex:1}
.modal-error{color:#c00;font-size:12px;margin-top:10px}
.modal-actions{display:flex;gap:8px;align-items:center;margin-top:18px}
</style>
</head>
<body>
<header>
  ` + exhibitLogoSVG + `
  <h1>Exhibit</h1>
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
    <button class="btn" onclick="ingest()"><i class="ph ph-plus"></i> Upload</button>
  </div>
  <div id="status"></div>
</div>

<form class="search-row" method="GET" action="/">
  <input type="text" name="q" placeholder="Search…" value="` + searchVal + `">
  <button class="btn btn-sm" type="submit">Search</button>
</form>

<div class="grid">` + cards.String() + `</div>
</main>

` + renderEditTagModal() + `
` + renderAddTagModal(tags) + `

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
    '<div style="color:#888;margin:4px 0 8px">The most secure option will <em>always</em> be to disable all external origins. Use your own discretion when allowing access to the listed networks below. This is a static scan and may not include every origin the application needs to work.</div>' +
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

// Tag pill hover controls: detach (x) removes this tag from this artifact
// only; edit (pencil) opens the edit-tag modal. The trailing '+' opens the
// add-tag modal for that card.
document.addEventListener('click', function(e) {
  const detachBtn = e.target.closest('.tag-pill-detach');
  if (detachBtn) {
    e.preventDefault();
    detachTag(detachBtn);
    return;
  }
  const editBtn = e.target.closest('.tag-pill-edit');
  if (editBtn) {
    e.preventDefault();
    openEditTagModal(editBtn.dataset.tagId, editBtn.dataset.tagName, editBtn.dataset.tagColor);
    return;
  }
  const addBtn = e.target.closest('.tag-add-btn');
  if (addBtn) {
    e.preventDefault();
    openAddTagModal(addBtn.dataset.artifactId);
  }
});

// Clicking anywhere on a card opens that artifact's detail/viewer page — the
// card itself is the way in. Clicks that land on an interactive child (the
// title or Details link, or anything in the tag row — pills, edit/detach,
// the '+' button) are left alone so those keep their own behavior. The 'Open'
// card action was removed; this is the single open affordance per card.
document.addEventListener('click', function(e) {
  if (e.target.closest('a, button, .tag-row, .card-actions')) return;
  const card = e.target.closest('.card');
  if (!card || !card.dataset.href) return;
  window.location.href = card.dataset.href;
});

async function detachTag(btn) {
  const pill = btn.closest('.tag-pill');
  btn.disabled = true;
  try {
    const r = await fetch('/api/tags/' + encodeURIComponent(btn.dataset.tagId) + '/artifacts/' + encodeURIComponent(btn.dataset.artifactId), {
      method: 'DELETE',
      headers: {'Authorization':'Bearer '+TOKEN}
    });
    if (r.ok) {
      pill.remove();
      return;
    }
  } catch (e) {}
  btn.disabled = false;
}

// Edit-tag modal: rename + recolor (PATCH) or delete (DELETE) a tag. Both
// mutations are global, so on success we reload the gallery rather than
// patching just the one card — every pill of that tag updates/disappears
// everywhere at once.
let editingTagId = null;

function openEditTagModal(tagId, tagName, tagColor) {
  editingTagId = tagId;
  document.getElementById('tag-edit-name').value = tagName;
  setModalColor('tag-edit', tagColor);
  setModalError('tag-edit', '');
  document.getElementById('tag-edit-modal').hidden = false;
  document.getElementById('tag-edit-name').focus();
}

function closeTagEditModal() {
  document.getElementById('tag-edit-modal').hidden = true;
  editingTagId = null;
}

// setModalColor/setModalError are shared by the edit-tag modal (tww.2.4)
// and the add-tag modal (tww.2.5), which reuse the same field ids under a
// different prefix (e.g. 'tag-edit' / 'tag-add').
function setModalColor(prefix, hex) {
  document.getElementById(prefix + '-color-hex').value = hex;
  document.getElementById(prefix + '-color-picker').value = hex;
  document.querySelectorAll('#' + prefix + '-modal .color-swatch').forEach(function(sw) {
    sw.classList.toggle('selected', sw.dataset.color.toLowerCase() === hex.toLowerCase());
  });
}

function setModalError(prefix, message) {
  const el = document.getElementById(prefix + '-error');
  el.textContent = message;
  el.hidden = !message;
}

function wireColorControls(prefix) {
  document.querySelectorAll('#' + prefix + '-modal .color-swatch').forEach(function(sw) {
    sw.addEventListener('click', function() { setModalColor(prefix, sw.dataset.color); });
  });
  document.getElementById(prefix + '-color-picker').addEventListener('input', function(e) {
    setModalColor(prefix, e.target.value);
  });
  document.getElementById(prefix + '-color-hex').addEventListener('input', function(e) {
    if (/^#[0-9a-fA-F]{6}$/.test(e.target.value)) setModalColor(prefix, e.target.value);
  });
}
wireColorControls('tag-edit');

document.getElementById('tag-edit-cancel').addEventListener('click', closeTagEditModal);
document.getElementById('tag-edit-modal').addEventListener('click', function(e) {
  if (e.target.id === 'tag-edit-modal') closeTagEditModal();
});
document.addEventListener('keydown', function(e) {
  if (e.key !== 'Escape') return;
  if (!document.getElementById('tag-edit-modal').hidden) closeTagEditModal();
  if (!document.getElementById('tag-add-modal').hidden) closeTagAddModal();
});

document.getElementById('tag-edit-save').addEventListener('click', async function() {
  const name = document.getElementById('tag-edit-name').value.trim();
  const color = document.getElementById('tag-edit-color-hex').value.trim();
  if (!name) { setModalError('tag-edit', 'Name is required.'); return; }
  const r = await fetch('/api/tags/' + encodeURIComponent(editingTagId), {
    method: 'PATCH',
    headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
    body: JSON.stringify({name: name, color: color})
  });
  if (!r.ok) {
    const data = await r.json().catch(function() { return {}; });
    setModalError('tag-edit', data.error || 'Failed to save tag.');
    return;
  }
  location.reload();
});

document.getElementById('tag-edit-delete').addEventListener('click', async function() {
  const name = document.getElementById('tag-edit-name').value;
  if (!confirm('Delete tag "' + name + '"? It will be removed from every artifact. This cannot be undone.')) return;
  const r = await fetch('/api/tags/' + encodeURIComponent(editingTagId), {
    method: 'DELETE',
    headers: {'Authorization':'Bearer '+TOKEN}
  });
  if (!r.ok) {
    const data = await r.json().catch(function() { return {}; });
    setModalError('tag-edit', data.error || 'Failed to delete tag.');
    return;
  }
  location.reload();
});

// Add-tag modal: pick an existing tag from the dropdown, or "create new" to
// reveal the same name+color fields as the edit-tag modal. Confirm creates
// the tag first (if new) and always attaches it; attaching a tag the
// artifact already has is a no-op on the server, so no special-casing is
// needed here.
let addingArtifactId = null;

function openAddTagModal(artifactId) {
  addingArtifactId = artifactId;
  document.getElementById('tag-add-select').value = '';
  document.getElementById('tag-add-create-fields').hidden = true;
  document.getElementById('tag-add-name').value = '';
  setModalColor('tag-add', '` + store.DefaultTagColor + `');
  setModalError('tag-add', '');
  document.getElementById('tag-add-modal').hidden = false;
  document.getElementById('tag-add-select').focus();
}

function closeTagAddModal() {
  document.getElementById('tag-add-modal').hidden = true;
  addingArtifactId = null;
}

document.getElementById('tag-add-select').addEventListener('change', function(e) {
  document.getElementById('tag-add-create-fields').hidden = e.target.value !== '__new__';
});
wireColorControls('tag-add');

document.getElementById('tag-add-cancel').addEventListener('click', closeTagAddModal);
document.getElementById('tag-add-modal').addEventListener('click', function(e) {
  if (e.target.id === 'tag-add-modal') closeTagAddModal();
});

document.getElementById('tag-add-confirm').addEventListener('click', async function() {
  const choice = document.getElementById('tag-add-select').value;
  if (!choice) { setModalError('tag-add', 'Choose a tag or create a new one.'); return; }

  let tagId = choice;
  if (choice === '__new__') {
    const name = document.getElementById('tag-add-name').value.trim();
    if (!name) { setModalError('tag-add', 'Name is required.'); return; }
    const color = document.getElementById('tag-add-color-hex').value.trim();
    const created = await fetch('/api/tags', {
      method: 'POST',
      headers: {'Content-Type':'application/json','Authorization':'Bearer '+TOKEN},
      body: JSON.stringify({name: name, color: color})
    });
    const data = await created.json().catch(function() { return {}; });
    if (!created.ok) { setModalError('tag-add', data.error || 'Failed to create tag.'); return; }
    tagId = data.id;
  }

  const attached = await fetch('/api/tags/' + encodeURIComponent(tagId) + '/artifacts/' + encodeURIComponent(addingArtifactId), {
    method: 'POST',
    headers: {'Authorization':'Bearer '+TOKEN}
  });
  if (!attached.ok) {
    const data = await attached.json().catch(function() { return {}; });
    setModalError('tag-add', data.error || 'Failed to attach tag.');
    return;
  }
  location.reload();
});
</script>
</body>
</html>`
}

// renderTagPills renders an artifact's tags as colored pills. It returns ""
// when there are no tags so cards without tags render with no empty pill
// row. Each pill carries a hover/focus-revealed edit (pencil) control on the
// left and a detach (x) control on the right; both are real <button>s so
// they're reachable by keyboard without extra handling, and they occupy
// fixed space at all times so revealing them on hover never shifts the pill
// layout — only their opacity changes.
func renderTagPills(artifactID string, tags []*store.Tag) string {
	if len(tags) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<ul class="tag-pills" data-artifact-id="%s">`, artifactID))
	for _, t := range tags {
		bg := color.Normalize(t.Color)
		fg := color.ContrastText(bg)
		name := htmlEsc(t.Name)
		b.WriteString(fmt.Sprintf(
			`<li class="tag-pill" data-tag-id="%s" style="background:%s;color:%s">`+
				`<button type="button" class="tag-pill-edit" data-tag-id="%s" data-tag-name="%s" data-tag-color="%s" aria-label="Edit tag %s"><i class="ph ph-pencil-simple"></i></button>`+
				`<span class="tag-pill-label">%s</span>`+
				`<button type="button" class="tag-pill-detach" data-tag-id="%s" data-artifact-id="%s" aria-label="Remove tag %s from this artifact"><i class="ph ph-x"></i></button>`+
				`</li>`,
			t.ID, bg, fg,
			t.ID, name, bg, name,
			name,
			t.ID, artifactID, name))
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// renderTagRow wraps an artifact's tag pills with a trailing '+' button that
// opens the add-tag modal. Unlike the pills themselves, the '+' always
// renders — even for an untagged artifact, it's the only way to attach a
// first tag — so it lives outside renderTagPills's empty-tags short circuit.
func renderTagRow(artifactID string, tags []*store.Tag) string {
	return `<div class="tag-row">` + renderTagPills(artifactID, tags) + fmt.Sprintf(
		`<button type="button" class="tag-add-btn" data-artifact-id="%s" aria-label="Add tag"><i class="ph ph-plus"></i></button>`,
		artifactID) + `</div>`
}

// renderColorSwatches renders the shared preset-color palette used by both
// the edit-tag modal (tww.2.4) and the add-tag modal (tww.2.5). Swatches are
// plain buttons scoped by the caller's modal id in CSS/JS, so the markup
// itself carries no modal-specific state.
func renderColorSwatches() string {
	var b strings.Builder
	b.WriteString(`<div class="color-presets">`)
	for _, c := range color.Presets {
		b.WriteString(fmt.Sprintf(
			`<button type="button" class="color-swatch" data-color="%s" style="background:%s" aria-label="%s"></button>`,
			c, c, c))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderEditTagModal renders the (initially hidden) edit-tag modal shell
// shared by every card's pencil control. There is one instance per gallery
// page load; openEditTagModal (in the page script) populates it for
// whichever tag was clicked.
func renderEditTagModal() string {
	return `<div id="tag-edit-modal" class="modal-overlay" hidden>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="tag-edit-title">
    <h2 id="tag-edit-title">Edit tag</h2>
    <label for="tag-edit-name">Name</label>
    <input type="text" id="tag-edit-name" maxlength="60">
    <label>Color</label>
    ` + renderColorSwatches() + `
    <div class="color-custom-row">
      <input type="color" id="tag-edit-color-picker" aria-label="Custom color">
      <input type="text" id="tag-edit-color-hex" placeholder="#rrggbb" maxlength="7" aria-label="Color hex code">
    </div>
    <div id="tag-edit-error" class="modal-error" hidden></div>
    <div class="modal-actions">
      <button type="button" class="btn btn-danger" id="tag-edit-delete"><i class="ph ph-trash"></i> Delete</button>
      <span class="spacer"></span>
      <button type="button" class="btn btn-sec" id="tag-edit-cancel">Cancel</button>
      <button type="button" class="btn" id="tag-edit-save"><i class="ph ph-check"></i> Save</button>
    </div>
  </div>
</div>`
}

// renderAddTagModal renders the (initially hidden) add-tag modal shell shared
// by every card's trailing '+' button. It offers a dropdown of every existing
// tag (server-rendered from the same ListTags call the gallery already makes)
// plus a "create new" option that reveals the same name+color fields as the
// edit-tag modal (openAddTagModal/wireColorControls('tag-add') in the page
// script).
func renderAddTagModal(tags []*store.Tag) string {
	var opts strings.Builder
	for _, t := range tags {
		opts.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`, t.ID, htmlEsc(t.Name)))
	}
	return `<div id="tag-add-modal" class="modal-overlay" hidden>
  <div class="modal" role="dialog" aria-modal="true" aria-labelledby="tag-add-title">
    <h2 id="tag-add-title">Add tag</h2>
    <label for="tag-add-select">Tag</label>
    <select id="tag-add-select">
      <option value="">Choose a tag…</option>
      ` + opts.String() + `
      <option value="__new__">+ Create new tag</option>
    </select>
    <div id="tag-add-create-fields" hidden>
      <label for="tag-add-name">Name</label>
      <input type="text" id="tag-add-name" maxlength="60">
      <label>Color</label>
      ` + renderColorSwatches() + `
      <div class="color-custom-row">
        <input type="color" id="tag-add-color-picker" aria-label="Custom color">
        <input type="text" id="tag-add-color-hex" placeholder="#rrggbb" maxlength="7" aria-label="Color hex code">
      </div>
    </div>
    <div id="tag-add-error" class="modal-error" hidden></div>
    <div class="modal-actions">
      <span class="spacer"></span>
      <button type="button" class="btn btn-sec" id="tag-add-cancel">Cancel</button>
      <button type="button" class="btn" id="tag-add-confirm"><i class="ph ph-check"></i> Add</button>
    </div>
  </div>
</div>`
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

	// "Update from source" is only offered for artifacts created from a URL.
	refetchToolbar := ""
	refetchScript := ""
	if a.SourceURL != "" {
		refetchToolbar = `
  <span style="color:#ddd">|</span>
  <button onclick="refetchSource()">Update from source ↻</button>`
		refetchScript = `
const SOURCE_URL = ` + fmt.Sprintf("%q", a.SourceURL) + `;

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
}`
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + htmlEsc(a.Title) + ` — Exhibit</title>
` + phosphorCSSLink + `
<style>
:root{--brand-blue:` + color.BrandBlue + `;--brand-blue-hover:` + color.BrandBlueHover + `}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;background:#f0f0f0;color:#111;display:flex;flex-direction:column;height:100vh}
header{background:#fff;border-bottom:1px solid #e0e0e0;padding:10px 20px;display:flex;align-items:center;gap:12px;flex-shrink:0}
header a{color:var(--brand-blue);text-decoration:none;font-size:13px}
header h1{font-size:16px;font-weight:600;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.toolbar{background:#fff;border-bottom:1px solid #e0e0e0;padding:8px 20px;display:flex;gap:12px;align-items:center;flex-shrink:0;font-size:13px}
.toolbar a{color:var(--brand-blue);text-decoration:none;display:inline-flex;align-items:center;gap:4px}
.toolbar button{display:inline-flex;align-items:center;gap:4px;padding:4px 12px;font-size:13px;border:1px solid #ddd;border-radius:5px;background:#fff;cursor:pointer}
.toolbar button:hover{border-color:var(--brand-blue);color:var(--brand-blue)}
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
  <a href="/artifacts/` + a.ID + `/edit"><i class="ph ph-pencil-simple"></i> Edit</a>` + refetchToolbar + `
  <span style="color:#ddd">|</span>
  <span style="color:#888">Allowlist:</span>
  <span id="al-display">` + renderAllowlistBadges(a.NetworkAllowlist) + `</span>
  <input id="al-input" type="text" placeholder="Add origin (https://example.com)" style="display:none">
  <button onclick="addOrigin()"><i class="ph ph-plus"></i> Add origin</button>
  <span id="al-status" style="color:#888"></span>
</div>
<div class="panels">
  <!-- allow= delegates the clipboard Permissions Policy into the sandboxed frame.
       Without it, the frame's opaque origin has clipboard-read/write denied and
       copy/paste (a common artifact interaction) throws a permissions-policy
       violation. This is a local capability, not network egress — the artifact's
       origin stays opaque and connect-src is still governed by the allowlist. -->
  <iframe src="` + renderOrigin + `/a/` + a.ID + `" sandbox="allow-scripts" allow="clipboard-read; clipboard-write" loading="lazy"></iframe>
  <div class="panel"><pre>` + htmlEsc(src) + `</pre></div>
</div>
<script>
const TOKEN = ` + fmt.Sprintf("%q", token) + `;
const ID = ` + fmt.Sprintf("%q", a.ID) + `;
let allowlist = ` + allowlistJSON + `;

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
` + refetchScript + `
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
<title>Edit: ` + htmlEsc(a.Title) + ` — Exhibit</title>
` + phosphorCSSLink + `
<style>
:root{--brand-blue:` + color.BrandBlue + `;--brand-blue-hover:` + color.BrandBlueHover + `}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,sans-serif;background:#f0f0f0;color:#111;min-height:100vh}
header{background:#fff;border-bottom:1px solid #e0e0e0;padding:12px 24px;display:flex;align-items:center;gap:16px}
header h1{font-size:18px;font-weight:600;flex:1}
header a{color:var(--brand-blue);text-decoration:none;font-size:14px}
main{padding:24px;max-width:900px;margin:0 auto}
.edit-card{background:#fff;border-radius:10px;padding:20px;box-shadow:0 1px 3px rgba(0,0,0,.08)}
.edit-card h2{font-size:15px;font-weight:600;margin-bottom:12px;color:#333}
.edit-card input[type=text]{width:100%;padding:8px 10px;border:1px solid #ddd;border-radius:6px;font-size:14px;margin-bottom:10px;outline:none}
.edit-card input[type=text]:focus{border-color:var(--brand-blue)}
.edit-card textarea{width:100%;height:400px;font-family:monospace;font-size:12px;border:1px solid #ddd;border-radius:6px;padding:10px;resize:vertical;outline:none}
.edit-card textarea:focus{border-color:var(--brand-blue)}
.edit-card .cm-editor{height:400px;font-size:12px;border:1px solid #ddd;border-radius:6px;overflow:hidden}
.edit-card .cm-editor.cm-focused{outline:none;border-color:var(--brand-blue)}
.edit-card .cm-scroller{overflow:auto}
.btn-row{display:flex;gap:8px;margin-top:12px;align-items:center}
.btn{display:inline-flex;align-items:center;gap:6px;padding:8px 18px;background:var(--brand-blue);color:#fff;border:none;border-radius:6px;font-size:14px;cursor:pointer;font-weight:500}
.btn:hover{background:var(--brand-blue-hover)}
.btn-sec{background:#fff;color:#333;border:1px solid #ddd}
.btn-sec:hover{border-color:var(--brand-blue);color:var(--brand-blue);background:#fff}
.btn-danger{background:#e00;color:#fff;border:none}
.btn-danger:hover{background:#c00}
.spacer{flex:1}
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
    <button class="btn" onclick="save()"><i class="ph ph-check"></i> Save</button>
    <a href="/artifacts/` + a.ID + `"><button class="btn btn-sec" type="button"><i class="ph ph-x"></i> Cancel</button></a>
    <span id="status"></span>
    <span class="spacer"></span>
    <button class="btn btn-danger" type="button" onclick="deleteArtifact()"><i class="ph ph-trash"></i> Delete</button>
  </div>
</div>
</main>
<script src="/assets/editor.js"></script>
<script>
const TOKEN = ` + fmt.Sprintf("%q", token) + `;
const ID = ` + fmt.Sprintf("%q", a.ID) + `;

// Mount the CodeMirror island over the body textarea. The editor keeps
// textarea.value in sync, so save() below is oblivious to it — and if the
// bundle failed to load, the plain textarea still works.
if (window.ArtifactEditor) {
  ArtifactEditor.mount(document.getElementById('body'));
}

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

async function deleteArtifact() {
  if (!confirm('Are you sure you want to delete this artifact? The action cannot be reversed and all data will be lost.')) return;
  const status = document.getElementById('status');
  status.textContent = 'Deleting…';
  try {
    const resp = await fetch('/api/artifacts/' + ID, {
      method: 'DELETE',
      headers: {'Authorization':'Bearer '+TOKEN}
    });
    if (!resp.ok) {
      const txt = await resp.text().catch(() => '');
      status.textContent = '✗ Error: ' + (txt.trim() || resp.statusText);
      return;
    }
    window.location.href = '/';
  } catch (e) {
    status.textContent = '✗ Error: ' + e.message;
  }
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
