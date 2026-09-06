/* The owner's share panel (av-6xjd). Served from the app origin at
 * /assets/gallery/share.js, loaded by detail.tmpl only where the panel is
 * rendered — which is only for the artifact's owner.
 *
 * Globals from the page's inline bootstrap:
 *   ID                - the artifact id
 *   TOKEN / READ_ONLY - this visitor's API credential (av-5imk), spent by
 *                       api.js's apiFetch, never by a header built here
 *
 * Every mutation goes through the HTTP API — the single write path — and the
 * panel then fires `exhibit:shares-changed`, which htmx turns into a re-fetch
 * of /partials/share-panel. That is why nothing in this file builds a grant row:
 * the server owns that markup, in one definition shared with the full page
 * render, and recipient names are attacker-influenced text that html/template
 * escapes and hand-built markup would not.
 *
 * Listeners are delegated from #share-panel, which is stable — htmx replaces
 * the contents of #share-body inside it, so anything bound to an element in
 * there would be bound to a node that no longer exists after the first change.
 */
(function() {
  const panel = document.getElementById('share-panel');
  if (!panel) return;

  const byId = (id) => document.getElementById(id);

  // The status line and the per-name results live OUTSIDE the swapped region,
  // which is the whole reason they are separate elements: a bulk grant's
  // report is exactly what the swap that follows the grant would otherwise
  // wipe before anyone could read it.
  function status(text) {
    const el = byId('share-status');
    if (el) el.textContent = text;
  }

  // Tells htmx to re-render the panel from the server. Every mutation ends
  // here rather than patching the DOM, so what the panel shows is what the
  // database says and not what this file believes it just did.
  function changed() {
    document.body.dispatchEvent(new CustomEvent('exhibit:shares-changed'));
  }

  async function post(body) {
    return apiFetch('/api/shares', { method: 'POST', body: JSON.stringify(body) });
  }

  // --- grants ------------------------------------------------------------

  // One field, several names, one submit. Commas, whitespace and newlines all
  // separate, because a list pasted out of a chat is not going to arrive in
  // the one format this field asked for.
  function splitNames(raw) {
    return raw.split(/[,\n]+/).map((n) => n.trim()).filter((n) => n !== '');
  }

  // The per-name report, and the reason the whole grant is not all-or-nothing:
  // "alice, bob, tpyo" must not refuse three people over one typo, and must
  // not succeed without ever mentioning the typo.
  const GRANT_MESSAGE = {
    granted: 'now has access',
    already_shared: 'already had access',
    unknown: 'no account with that name or address',
    ambiguous: 'that address matches more than one account',
    self: 'is you — you already own this artifact'
  };

  function reportGrants(results) {
    const list = byId('share-add-results');
    if (!list) return;
    list.innerHTML = '';
    results.forEach(function(r) {
      const row = document.createElement('div');
      row.className = 'share-result share-result-' + r.status;
      const name = document.createElement('code');
      // textContent: the name is echoed straight back from what was typed.
      name.textContent = r.name;
      row.appendChild(name);
      const msg = document.createElement('span');
      msg.textContent = ' — ' + (GRANT_MESSAGE[r.status] || r.status);
      row.appendChild(msg);
      list.appendChild(row);
    });
  }

  async function grant() {
    const input = byId('share-add-input');
    const names = splitNames(input ? input.value : '');
    if (names.length === 0) return;
    status('Giving access…');
    const r = await post({ artifact_id: ID, recipients: names })
      .catch(function() { return null; });
    if (!r || !r.ok) {
      status('✗ Could not give access' + (r ? ' (' + r.status + ')' : ''));
      return;
    }
    const data = await r.json().catch(() => ({}));
    reportGrants(data.results || []);
    status('');
    changed();
  }

  async function revoke(shareID) {
    status('Revoking…');
    const r = await apiFetch('/api/shares/' + encodeURIComponent(shareID), { method: 'DELETE' })
      .catch(function() { return null; });
    if (!r || !r.ok) { status('✗ Could not revoke access'); return; }
    status('');
    changed();
  }

  // --- the public link ---------------------------------------------------

  // A toggle, not a create button: on mints the artifact's one anonymous link,
  // off deletes it and the URL dies. The checkbox is re-rendered from the
  // server by the swap, so a failed write leaves it showing the truth rather
  // than the state the click implied.
  async function setLink(on) {
    if (!on) {
      const shareID = linkID();
      if (!shareID) { changed(); return; }
      status('Removing the link…');
      const r = await apiFetch('/api/shares/' + encodeURIComponent(shareID), { method: 'DELETE' })
        .catch(function() { return null; });
      status(r && r.ok ? '' : '✗ Could not remove the link');
      changed();
      return;
    }
    status('Creating a link…');
    const r = await post({ artifact_id: ID }).catch(function() { return null; });
    status(r && r.ok ? '' : '✗ Could not create a link');
    changed();
  }

  // Replace is ONE request, not a delete followed by a create. The failure
  // between those two halves leaves the artifact with no link at all, which is
  // worse than either the old link or a new one — so the server does both in a
  // transaction and this asks for it in one call.
  async function replaceLink() {
    if (!confirm('Replace this link?\n\nA new link is created and the current one stops working immediately. Anyone still holding the old link loses access.')) return;
    status('Replacing the link…');
    const r = await post({ artifact_id: ID, replace: true }).catch(function() { return null; });
    status(r && r.ok ? '✓ New link created — the old one no longer works' : '✗ Could not replace the link');
    changed();
  }

  function linkID() {
    const toggle = byId('share-link-toggle');
    return toggle ? toggle.dataset.shareId : '';
  }

  function copyLink() {
    const field = byId('share-link-url');
    if (!field) return;
    field.select();
    // The app origin is not sandboxed, so this is the ordinary clipboard API
    // and not the host bridge the artifact frame has to use.
    navigator.clipboard.writeText(field.value)
      .then(function() { status('✓ Link copied'); })
      .catch(function() { status('Select the link and copy it.'); });
  }

  // --- whose data --------------------------------------------------------

  // One control per artifact, applied immediately: it is a single answer to a
  // single question, and a Save button beside one select is a second thing to
  // press for no gain. The swap that follows re-renders it from the stored
  // value, so a rejected write corrects itself on screen.
  async function setStateMode(mode) {
    status('Saving…');
    const r = await apiFetch('/api/artifacts/' + ID, {
      method: 'PATCH',
      body: JSON.stringify({ share_state_mode: mode })
    }).catch(function() { return null; });
    status(r && r.ok ? '✓ Saved' : '✗ Could not change how data is shared');
    changed();
  }

  panel.addEventListener('click', function(e) {
    const target = e.target;
    const revokeBtn = target.closest ? target.closest('[data-action="revoke"]') : null;
    if (revokeBtn) return revoke(revokeBtn.dataset.shareId);
    if (target.id === 'share-add-btn') return grant();
    if (target.id === 'share-link-replace') return replaceLink();
    if (target.id === 'share-link-copy') return copyLink();
    return undefined;
  });

  panel.addEventListener('change', function(e) {
    const target = e.target;
    if (target.id === 'share-link-toggle') return setLink(!!target.checked);
    if (target.id === 'share-state-mode') return setStateMode(target.value);
    return undefined;
  });

  panel.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && e.target && e.target.id === 'share-add-input') {
      e.preventDefault();
      grant();
    }
  });
})();
