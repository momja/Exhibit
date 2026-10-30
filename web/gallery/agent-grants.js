/* The owner's External agents panel (Exh-l2xj). Issuing and revoking persist
 * immediately; htmx then re-renders the list from the server. The new token
 * is held only in #agent-grant-issued, outside the swapped region, because
 * the server keeps a digest and this is the one chance to copy it.
 */
import { apiFetch } from './api-module.js';

function initializeAgentGrantsPanel() {
  const panel = document.getElementById('agent-grants-panel');
  if (!panel) return;
  const byId = (id) => document.getElementById(id);
  const artifactID = () => panel.querySelector('.agent-grants').dataset.artifactId;
  let busy = false;
  // The grant whose token is on screen, so revoking it takes the token away
  // while revoking another grant leaves it to be copied.
  let issuedGrantID = '';

  function status(message) {
    byId('agent-grants-status').textContent = message;
  }

  function controls() {
    panel.querySelectorAll('#agent-grants-body input, #agent-grants-body button').forEach((control) => {
      control.disabled = busy;
    });
  }

  function forgetToken() {
    issuedGrantID = '';
    ['agent-grant-token', 'agent-grant-command', 'agent-grant-config'].forEach((id) => { byId(id).value = ''; });
    byId('agent-grant-issued').hidden = true;
  }

  function close() {
    if (busy) return;
    // The token is not kept past the dialog: closing it is the owner saying
    // they have what they need.
    forgetToken();
    status('');
    panel.hidden = true;
    byId('agent-grants-open').focus();
  }

  byId('agent-grants-open').addEventListener('click', function() {
    panel.hidden = false;
    (byId('agent-grant-label') || byId('agent-grants-close')).focus();
  });
  byId('agent-grants-close').addEventListener('click', close);
  panel.addEventListener('click', function(e) { if (e.target === panel) close(); });
  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape' && !panel.hidden) close();
  });

  // The connector's configuration, for the token just issued. Values only:
  // nothing here is parsed as markup.
  function showConfig(token) {
    const origin = byId('agent-grant-issued').dataset.appOrigin || '';
    byId('agent-grant-token').value = token;
    byId('agent-grant-command').value = 'claude mcp add exhibit --env EXHIBIT_URL=' + origin +
      ' --env EXHIBIT_TOKEN=' + token + ' -- exhibit-mcp';
    byId('agent-grant-config').value = JSON.stringify({ mcpServers: { exhibit: {
      command: 'exhibit-mcp', env: { EXHIBIT_URL: origin, EXHIBIT_TOKEN: token }
    } } }, null, 2);
  }

  async function mutate(operation, failureMessage) {
    if (busy) return;
    busy = true;
    controls();
    status('Saving changes…');
    try {
      status(await operation() || '✓ Changes saved');
    } catch {
      status(failureMessage);
    }
    document.body.dispatchEvent(new CustomEvent('exhibit:agent-grants-changed'));
  }

  panel.addEventListener('submit', function(e) {
    if (e.target.id !== 'agent-grant-form') return;
    e.preventDefault();
    const label = byId('agent-grant-label').value.trim();
    return mutate(async () => {
      const response = await apiFetch('/api/artifacts/' + encodeURIComponent(artifactID()) + '/agent-grants', {
        method: 'POST', body: JSON.stringify({ label })
      });
      if (!response || !response.ok) throw new Error('issue failed');
      const data = await response.json();
      if (!data.token || !data.grant) throw new Error('missing token');
      issuedGrantID = data.grant.id;
      showConfig(data.token);
      byId('agent-grant-issued').hidden = false;
      return '✓ Token issued';
    }, 'Could not issue a token. Try again.');
  });

  function copyField(id, what) {
    const field = byId(id);
    field.select();
    const fallback = 'Select the ' + what.toLowerCase() + ' and copy it.';
    if (!navigator.clipboard) return status(fallback);
    return navigator.clipboard.writeText(field.value)
      .then(() => status('✓ ' + what + ' copied'))
      .catch(() => status(fallback));
  }

  panel.addEventListener('click', function(e) {
    const target = e.target;
    if (!target.closest) return;
    for (const [button, field, what] of [['agent-grant-copy', 'agent-grant-token', 'Token'],
      ['agent-grant-command-copy', 'agent-grant-command', 'Command'], ['agent-grant-config-copy', 'agent-grant-config', 'Configuration']]) {
      if (target.closest('#' + button)) return copyField(field, what);
    }
    const revoke = target.closest('[data-action="revoke-agent-grant"]');
    if (!revoke || busy) return;
    return mutate(async () => {
      const response = await apiFetch('/api/artifacts/' + encodeURIComponent(artifactID()) +
        '/agent-grants/' + encodeURIComponent(revoke.dataset.grantId), { method: 'DELETE' });
      // A concurrent revoke already did what this one asked.
      if (!response || (!response.ok && response.status !== 404)) throw new Error('revoke failed');
      if (revoke.dataset.grantId === issuedGrantID) forgetToken();
      return '✓ Token revoked';
    }, 'Could not revoke the token. It is still listed; try again.');
  });

  function settled() {
    busy = false;
    controls();
  }
  panel.addEventListener('htmx:after:swap', function(e) {
    if (e.target.id === 'agent-grants-body') settled();
  });
  panel.addEventListener('htmx:error', function(e) {
    if (e.target.id !== 'agent-grants-body') return;
    settled();
    status('The list could not refresh. Close and reopen the page to see it.');
  });
}

initializeAgentGrantsPanel();
