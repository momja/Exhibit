import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/agent-state-access-controller.js', import.meta.url));

async function setup({ input, url, method, field, respond }) {
  const calls = [];
  const page = loadPageScript([], {
    apiFetch: async (path, options) => {
      calls.push({ path, method: options.method, data: JSON.parse(options.body) });
      return respond();
    },
  });
  const { default: AgentStateAccess } = await page.loadModule(script);
  const controller = Object.create(AgentStateAccess.prototype);
  Object.defineProperty(controller, 'element', { value: { isConnected: true } });
  Object.assign(controller, {
    inputTarget: input,
    statusTarget: { textContent: '', hidden: true, classList: { toggle() {} } },
    hasStatusTarget: true,
    urlValue: url, methodValue: method, fieldValue: field,
  });
  controller.connect();
  return { calls, controller };
}

test('the account default is saved as a boolean the moment it changes', async () => {
  const input = { type: 'checkbox', checked: false, disabled: false };
  const { calls, controller } = await setup({
    input, url: '/api/settings/agent-state-access', method: 'PUT', field: 'allowed',
    respond: () => Response.json({ allowed: true }),
  });
  input.checked = true;
  await controller.save();
  assert.deepEqual(calls, [{ path: '/api/settings/agent-state-access', method: 'PUT', data: { allowed: true } }]);
  assert.equal(controller.statusTarget.textContent, 'Saved.');
  assert.equal(input.disabled, false);
});

test('a refused artifact override puts back the last saved choice and says why', async () => {
  const input = { type: 'select-one', value: 'inherit', disabled: false };
  const { calls, controller } = await setup({
    input, url: '/api/artifacts/a1', method: 'PATCH', field: 'agent_state_access',
    respond: () => Response.json({ error: 'agent_state_access is set by the artifact\'s owner' }, { status: 403 }),
  });
  input.value = 'allow';
  await controller.save();
  assert.deepEqual(calls, [{ path: '/api/artifacts/a1', method: 'PATCH', data: { agent_state_access: 'allow' } }]);
  assert.equal(input.value, 'inherit', 'the control never shows a choice that did not take');
  assert.match(controller.statusTarget.textContent, /^Not saved: agent_state_access is set by the artifact's owner/);
  assert.equal(input.disabled, false);
});
