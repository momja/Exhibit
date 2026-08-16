/* Own-account page script (av-4wyq). Served from the app origin at
 * /assets/gallery/profile.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads before it loads:
 *   TOKEN / READ_ONLY - this visitor's API credential, decided server-side per
 *                       request (av-5imk); spent via api.js's apiFetch
 *
 * It drives one action, DELETE /api/account, and it guards nothing. Every
 * interlock here — revealing the second step, requiring the phrase, keeping the
 * final button disabled until it matches — is a courtesy to the person
 * clicking. The server requires the same phrase and refuses the same requests
 * whether or not this file ran (internal/api/profile.go). Treating these as
 * controls would be the mistake; the reason they exist is that this operation
 * has no undo anywhere near it and a mis-tap must not be able to reach it.
 *
 * The confirmation's markup is rendered by the server and only revealed here.
 * Its sentences are the most important on the page — what is destroyed, and
 * that signing in again produces an empty account rather than this one — and
 * they have exactly one definition, in profile.tmpl.
 */
(function() {
  const CONFIRM_PHRASE = 'delete my library';

  const trigger = document.getElementById('delete-account');
  const panel = document.getElementById('delete-confirm');
  const status = document.getElementById('profile-status');
  // Blocked accounts (the last enabled admin, or an instance with no login at
  // all) render the button disabled and no panel behind it. Nothing to wire.
  if (!trigger || !panel) return;

  const phrase = document.getElementById('delete-phrase');
  const go = document.getElementById('delete-confirm-go');
  const cancel = document.getElementById('delete-cancel');

  function say(message, isError) {
    status.textContent = message;
    status.classList.toggle('is-error', !!isError);
    status.hidden = false;
    if (isError) status.scrollIntoView({block: 'nearest'});
  }

  // The server's own words, whichever shape it used: writeError sends JSON,
  // http.Error sends text. The refusals this page can still get after its
  // button was enabled — the last-admin one above all — explain what to do
  // instead, so nothing here substitutes a generic message for one the server
  // took the trouble to write.
  async function failureText(response) {
    const body = await response.text();
    try {
      const parsed = JSON.parse(body);
      if (parsed && parsed.error) return parsed.error;
    } catch (e) { /* not JSON — use the text as-is */ }
    return body.trim() || ('Request failed (' + response.status + ')');
  }

  function open(isOpen) {
    panel.hidden = !isOpen;
    trigger.setAttribute('aria-expanded', String(isOpen));
    if (isOpen) {
      phrase.focus();
    } else {
      // Cleared on the way out, so re-opening never starts one keystroke from
      // an irreversible action.
      phrase.value = '';
      go.disabled = true;
      trigger.focus();
    }
  }

  trigger.addEventListener('click', function() { open(panel.hidden); });
  cancel.addEventListener('click', function() { open(false); });

  // Trimmed and case-folded, because the requirement is that the phrase was
  // typed deliberately, not that a trailing space was avoided. The server
  // compares exactly, so the request below sends the canonical phrase rather
  // than whatever was typed.
  function typedPhrase() {
    return phrase.value.trim().toLowerCase();
  }

  phrase.addEventListener('input', function() {
    go.disabled = typedPhrase() !== CONFIRM_PHRASE;
  });

  go.addEventListener('click', async function() {
    if (typedPhrase() !== CONFIRM_PHRASE) return;
    go.disabled = true;
    say('Deleting your account…', false);

    const r = await apiFetch('/api/account', {
      method: 'DELETE',
      body: JSON.stringify({confirm: CONFIRM_PHRASE})
    });
    if (!r.ok) {
      say(await failureText(r), true);
      go.disabled = false;
      return;
    }
    // The account is gone and its sessions with it, so there is no page on
    // this instance left to return to. Sending the browser to the root lets
    // whatever this instance does with an unauthenticated visitor happen —
    // the login page, or the public library — rather than this script
    // deciding on its behalf.
    window.location.href = '/';
  });
})();
