// Account connections persist through the API. htmx owns the refreshed list;
// only the newly issued secret and its connection snippets live in this module.
import { apiFetch } from './api-module.js';

function initializeAgentGrantsPanel() {
  const panel = document.getElementById('agent-grants-panel');
  if (!panel) return;
  const byId = (id) => document.getElementById(id);
  let busy = false;
  let issuedGrantID = '';

  function status(message) {
    byId('agent-grants-status').textContent = message;
    byId('agent-grants-status').hidden = !message;
  }

  function controls() {
    panel.querySelectorAll('#agent-grant-form input, #agent-grant-form button, [data-action="revoke-agent-grant"]').forEach((control) => {
      control.disabled = busy;
    });
  }

  function forgetToken() {
    issuedGrantID = '';
    if (!byId('agent-grant-token')) return;
    byId('agent-grant-token').value = '';
    if (panel.dataset.mcpUrl) {
      byId('agent-grant-remote-command').textContent = '';
      byId('agent-grant-remote-config').textContent = '';
    }
    byId('agent-grant-issued').hidden = true;
  }

  // URLs are operator configuration; shell quoting keeps their bytes data
  // in the command even if it contains shell metacharacters.
  function shellQuote(value) {
    return "'" + value.replaceAll("'", "'\\''") + "'";
  }

  async function mutate(operation, failureMessage) {
    if (busy) return;
    busy = true;
    controls();
    status('Saving changes…');
    try {
      status(await operation());
    } catch {
      status(failureMessage);
    }
    document.body.dispatchEvent(new CustomEvent('exhibit:agent-grants-changed'));
  }

  panel.addEventListener('submit', function(event) {
    if (event.target.id !== 'agent-grant-form') return;
    event.preventDefault();
    const label = byId('agent-grant-label').value.trim();
    return mutate(async () => {
      const response = await apiFetch('/api/agent-grants', {
        method: 'POST', body: JSON.stringify({ label })
      });
      if (!response || !response.ok) throw new Error('issue failed');
      const data = await response.json();
      if (!data.token || !data.grant) throw new Error('missing token');
      issuedGrantID = data.grant.id;
      byId('agent-grant-token').value = data.token;
      if (panel.dataset.mcpUrl) {
        const authorization = 'Bearer ' + data.token;
        byId('agent-grant-remote-command').textContent = 'claude mcp add --transport http exhibit ' +
          shellQuote(panel.dataset.mcpUrl) + ' --header ' + shellQuote('Authorization: ' + authorization);
        byId('agent-grant-remote-config').textContent = JSON.stringify({
          type: 'http', url: panel.dataset.mcpUrl, headers: { Authorization: authorization }
        }, null, 2);
      }
      byId('agent-grant-issued').hidden = false;
      byId('agent-grant-label').value = '';
      return 'Token issued. Copy it before leaving this page.';
    }, 'Could not issue a token. Try again.');
  });

  panel.addEventListener('click', function(event) {
    const target = event.target;
    if (!target.closest) return;
    for (const [button, fieldID, label] of [
      ['#agent-mcp-url-copy', 'agent-mcp-url', 'URL'],
      ['#agent-mcp-command-copy', 'agent-mcp-command', 'Command'],
      ['#agent-grant-copy', 'agent-grant-token', 'Token']
    ]) {
      if (!target.closest(button)) continue;
      const field = byId(fieldID);
      field.select();
      const fallback = 'Select the ' + label.toLowerCase() + ' and copy it.';
      if (!navigator.clipboard) return status(fallback);
      return navigator.clipboard.writeText(field.value)
        .then(() => status(label + ' copied.'))
        .catch(() => status(fallback));
    }
    if (target.closest('#agent-grant-dismiss')) {
      forgetToken();
      status('');
      byId('agent-grant-label').focus();
      return;
    }
    const revoke = target.closest('[data-action="revoke-agent-grant"]');
    if (!revoke || busy) return;
    return mutate(async () => {
      const response = await apiFetch('/api/agent-grants/' + encodeURIComponent(revoke.dataset.grantId), {
        method: 'DELETE'
      });
      // A concurrent revoke has already achieved the requested result.
      if (!response || (!response.ok && response.status !== 404)) throw new Error('revoke failed');
      if (revoke.dataset.grantId === issuedGrantID) forgetToken();
      return 'Token revoked.';
    }, 'Could not revoke the token. Try again.');
  });

  function settled() {
    busy = false;
    controls();
  }
  panel.addEventListener('htmx:after:swap', function(event) {
    if (event.target.id === 'agent-grants-body') settled();
  });
  panel.addEventListener('htmx:error', function(event) {
    if (event.target.id !== 'agent-grants-body') return;
    settled();
    status('The list could not refresh. Reload the page to see your connections.');
  });
  // Also clear the secret before this document enters the back/forward cache.
  window.addEventListener('pagehide', forgetToken);
}

initializeAgentGrantsPanel();
