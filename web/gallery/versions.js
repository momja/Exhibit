/* Version history panel — the edit page's list of an artifact's earlier
 * versions, and the one action on it: restore. Served from the app origin at
 * /assets/gallery/versions.js, after edit.js, and reads the per-request global
 * the page's inline bootstrap defines (ID) plus, through api.js's apiFetch,
 * this visitor's credential.
 *
 * The list itself is server-rendered (edit.tmpl); there is nothing to fetch.
 *
 * Restoring changes the artifact's code AND its saved data at once, and is the
 * one thing on this page that rewrites both together, so it asks first and says
 * so. It is not destructive in the sense the asset delete is: what it replaces
 * is kept as a version of its own. The prompt says that too, because "this will
 * overwrite your work" is the thing a person is actually afraid of here.
 *
 * It reloads on success rather than patching the page, because the source and
 * widget editors above hold the text of the version it just replaced.
 */
(function () {
  const rows = document.getElementById('versions-rows');
  if (!rows) return;
  const statusEl = document.getElementById('versions-status');

  function setStatus(text, isError) {
    statusEl.textContent = text || '';
    statusEl.classList.toggle('error', !!isError);
  }

  function failureMessage(res) {
    return res.json()
      .then(function (data) { return (data && data.error) || ('HTTP ' + res.status); })
      .catch(function () { return 'HTTP ' + res.status; });
  }

  rows.addEventListener('click', function (e) {
    const btn = e.target.closest('[data-action="restore"]');
    if (!btn) return;
    const seq = btn.dataset.seq;

    if (!window.confirm(
      'Restore v' + seq + '?\n\n' +
      "This sets the artifact's code and its saved data back to how they were when v" + seq +
      ' was replaced. What you have now is kept as its own version, so you can come back to it.\n\n' +
      'Unsaved changes in the editors on this page will be lost.'
    )) return;

    btn.disabled = true;
    setStatus('Restoring…');
    return window.apiFetch(
      '/api/artifacts/' + encodeURIComponent(ID) + '/versions/' + encodeURIComponent(seq) + '/restore',
      { method: 'POST' }
    ).then(function (res) {
      if (res.ok) {
        window.location.reload();
        return;
      }
      return failureMessage(res).then(function (msg) { throw new Error(msg); });
    }).catch(function (err) {
      btn.disabled = false;
      setStatus('Could not restore: ' + err.message, true);
    });
  });
})();
