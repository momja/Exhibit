/* Account administration page script (av-utap). Served from the app origin at
 * /assets/gallery/settings.js. The page's inline bootstrap <script> defines
 * the per-request globals this file reads before it loads:
 *   TOKEN / READ_ONLY - this visitor's API credential, decided server-side
 *                       per request (av-5imk); spent via api.js's apiFetch
 *   SELF_ID           - the viewer's own account id, so the page can warn
 *                       before they switch themselves off
 *
 * Everything here goes through /api/admin/users, which is the single write
 * path and where the admin check actually lives (internal/api/admin.go). This
 * script guards nothing: it is a client, and hiding a button is a courtesy to
 * the person clicking, never a control. The server refuses the same requests
 * whether or not this file ran.
 *
 * After a successful change the page reloads rather than patching the table in
 * place. The server already renders this list, the refusals (last admin) can
 * change what the *other* rows may do, and reloading is how the page stays
 * one source of truth instead of two.
 */
(function() {
  const status = document.getElementById('admin-status');

  function say(message, isError) {
    status.textContent = message;
    status.classList.toggle('is-error', !!isError);
    status.hidden = false;
    if (isError) status.scrollIntoView({block: 'nearest'});
  }

  // The server's own words, whichever shape it used. writeError sends JSON,
  // http.Error sends text, and a refusal an admin can act on (the last-admin
  // 409) is worth reading in full — so nothing here substitutes a generic
  // message for one the server took the trouble to write.
  async function failureText(response) {
    const body = await response.text();
    try {
      const parsed = JSON.parse(body);
      if (parsed && parsed.error) return parsed.error;
    } catch (e) { /* not JSON — use the text as-is */ }
    return body.trim() || ('Request failed (' + response.status + ')');
  }

  async function patchUser(id, changes, doing) {
    say(doing + '…', false);
    const r = await apiFetch('/api/admin/users/' + encodeURIComponent(id), {
      method: 'PATCH',
      body: JSON.stringify(changes)
    });
    if (!r.ok) { say(await failureText(r), true); return; }
    window.location.reload();
  }

  // One delegated listener for the whole table, so a row added by a reload
  // needs no re-wiring and the actions stay declared in the markup.
  document.querySelector('.settings-table').addEventListener('click', function(event) {
    const button = event.target.closest('button[data-action]');
    if (!button) return;
    const id = button.dataset.userId;
    const name = button.dataset.userName;

    if (button.dataset.action === 'password') {
      const password = window.prompt('New password for ' + name + ' (at least 8 characters).\n\n' +
        'Tell them out of band — nothing is emailed.');
      if (password === null) return;
      patchUser(id, {password: password}, 'Setting the password');
      return;
    }
    if (button.dataset.action === 'role') {
      const promoting = button.dataset.admin !== 'true';
      patchUser(id, {is_admin: promoting}, promoting ? 'Promoting ' + name : 'Demoting ' + name);
      return;
    }
    if (button.dataset.action === 'enable') {
      patchUser(id, {disabled: false}, 'Enabling ' + name);
      return;
    }
    if (button.dataset.action === 'disable') {
      // The self-disable warning is the one thing this page confirms, because
      // it is the one action whose consequence lands on the person taking it
      // and is not obvious from the button. Everything else is reversible from
      // this same page a moment later.
      const mine = String(SELF_ID) === String(id);
      const warning = mine
        ? 'Disable your own account? You will be signed out immediately and will not be able to sign back in.'
        : 'Disable ' + name + '? They are signed out everywhere immediately. Nothing is deleted.';
      if (!window.confirm(warning)) return;
      patchUser(id, {disabled: true}, 'Disabling ' + name);
    }
  });

  document.getElementById('create-user').addEventListener('click', async function() {
    const username = document.getElementById('new-username').value.trim();
    const password = document.getElementById('new-password').value;
    if (!username) { say('A login name is required.', true); return; }
    say('Creating the account…', false);
    const r = await apiFetch('/api/admin/users', {
      method: 'POST',
      body: JSON.stringify({
        username: username,
        password: password,
        is_admin: document.getElementById('new-is-admin').checked
      })
    });
    if (!r.ok) { say(await failureText(r), true); return; }
    window.location.reload();
  });
})();
