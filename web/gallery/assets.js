/* Out-of-line asset panel — the edit page's view of the binary payloads stored
 * beside an artifact (av-20fk). Served from the app origin at
 * /assets/gallery/assets.js, after edit.js, and reads the per-request globals
 * the page's inline bootstrap defines: ID and (through api.js's apiFetch) this
 * visitor's credential.
 *
 * Why this panel exists at all. An asset's deletability is normally decided by
 * a rule — the artifact went, or a refetch superseded the generation — and
 * those need no UI. One case cannot be decided that way: the owner edited away
 * the feature that used a payload. Nothing the server can inspect settles it,
 * because the render manifest matches resolved URLs at call time, so an asset
 * whose original fetch literal is gone may still be loaded by rewritten code.
 * Only a person knows. This is where they say so.
 *
 * Which is why the source URL is the primary column rather than a detail:
 * deleting an asset that is still in use breaks the artifact at render, and
 * that address is what the owner matches against their own source. The
 * confirmation names it too.
 *
 * Contents load on first open, not with the page — asset metadata is cold data
 * nothing else here needs. Every value below is artifact-controlled text
 * rendered on the *app* origin, so it reaches the DOM through createElement +
 * textContent and never through interpolated markup (same rule as state.js and
 * edit.js's buildOriginRow).
 */

(function () {
  const panel = document.getElementById('assets-panel');
  if (!panel) return;

  const rowsEl = document.getElementById('assets-rows');
  const statusEl = document.getElementById('assets-status');
  const summaryEl = document.getElementById('assets-summary-text');

  let loaded = false;

  function formatBytes(n) {
    if (n < 1024) return n + ' B';
    const units = ['KB', 'MB', 'GB'];
    let v = n / 1024;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return (v >= 10 ? Math.round(v) : v.toFixed(1)) + ' ' + units[i];
  }

  function setStatus(text, isError) {
    statusEl.textContent = text || '';
    statusEl.classList.toggle('error', !!isError);
  }

  function buildRow(asset) {
    const row = document.createElement('div');
    row.className = 'asset-row';

    // The address the artifact asks for, and the thing a person decides on.
    const url = document.createElement('code');
    url.className = 'asset-url';
    url.textContent = asset.source_url;
    row.appendChild(url);

    const meta = document.createElement('span');
    meta.className = 'text-sm muted asset-meta';
    meta.textContent = formatBytes(asset.size_bytes) + ' · ' + asset.content_type;
    row.appendChild(meta);

    const del = document.createElement('button');
    del.type = 'button';
    del.className = 'btn btn-sm btn-danger';
    del.innerHTML = '<i class="ph ph-trash"></i>';
    del.title = 'Delete this asset';
    del.addEventListener('click', function () { remove(asset, row, del); });
    row.appendChild(del);

    return row;
  }

  function remove(asset, row, btn) {
    // Name the address in the prompt: "delete this asset?" is not a question
    // anyone can answer, and the consequence is an artifact that stops working.
    if (!window.confirm(
      'Delete ' + asset.source_url + '?\n\n' +
      'This cannot be undone. If the artifact still loads that file, it will stop working.'
    )) return;

    btn.disabled = true;
    setStatus('Deleting…');
    window.apiFetch('/api/artifacts/' + encodeURIComponent(ID) + '/assets/' + encodeURIComponent(asset.id), {
      method: 'DELETE',
    }).then(function (res) {
      if (!res.ok) throw new Error('HTTP ' + res.status);
      row.remove();
      setStatus('Deleted.');
      loaded = false;
      load();
    }).catch(function (err) {
      btn.disabled = false;
      setStatus('Could not delete: ' + err.message, true);
    });
  }

  function render(data) {
    rowsEl.replaceChildren();
    const assets = data.assets || [];
    if (assets.length === 0) {
      const empty = document.createElement('p');
      empty.className = 'text-sm muted';
      empty.textContent = 'This artifact stores no assets of its own.';
      rowsEl.appendChild(empty);
      summaryEl.textContent = 'Stored assets';
      return;
    }
    assets.forEach(function (a) { rowsEl.appendChild(buildRow(a)); });
    summaryEl.textContent = 'Stored assets (' + assets.length + ', ' + formatBytes(data.total_bytes) + ')';
  }

  function load() {
    if (loaded) return;
    loaded = true;
    setStatus('Loading…');
    window.apiFetch('/api/artifacts/' + encodeURIComponent(ID) + '/assets')
      .then(function (res) {
        if (!res.ok) throw new Error('HTTP ' + res.status);
        return res.json();
      })
      .then(function (data) { render(data); setStatus(''); })
      .catch(function (err) {
        loaded = false;
        setStatus('Could not load assets: ' + err.message, true);
      });
  }

  panel.addEventListener('toggle', function () { if (panel.open) load(); });
  if (panel.open) load();
})();
