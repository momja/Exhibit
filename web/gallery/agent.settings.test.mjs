import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const asset = name => fileURLToPath(new URL('../../internal/api/assets/gallery/' + name, import.meta.url));
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };

async function settingsPage(saved, byok = true) {
  const calls = [];
  const page = loadPageScript([asset('state-api.js'), asset('network-prompt.js')], {
    TOKEN: '', READ_ONLY: false, BYOK: byok, artifact: null,
    apiFetch: async (path, options = {}) => {
      calls.push({path, ...options});
      assert.equal(path, '/api/agent/key');
      if (options.method === 'PUT') {
        saved = {...saved, ...JSON.parse(options.body), configured: true};
      }
      return {ok: true, json: async () => saved};
    },
    apiEventSource: () => ({close() {}}),
    matchMedia: () => ({matches: false, addEventListener() {}}),
    CustomEvent: class { constructor(type, options) { this.type = type; Object.assign(this, options); } },
  }, {'key-modal': {hidden: true}, 'net-modal': {hidden: true}});
  await page.loadModule(asset('agent.js'));
  await flush();
  return {...page, calls};
}

test('API key settings load, save, and clear a thinking preference without re-entering the key', async () => {
  const page = await settingsPage({configured: true, provider: 'anthropic', model: 'model', thinking_level: 'high'});
  assert.equal(page.byId('key-thinking').value, 'high');
  page.window.openKeyModal();
  page.byId('key-thinking').value = 'off';
  await page.window.saveKey();
  let saved = JSON.parse(page.calls.find(call => call.method === 'PUT').body);
  assert.equal(saved.thinking_level, 'off');
  assert.equal(saved.api_key, '', 'the existing masked key is kept');
  assert.equal(page.byId('key-thinking').value, 'off');

  page.window.openKeyModal();
  page.byId('key-thinking').value = 'max';
  page.window.closeKeyModal();
  page.window.openKeyModal();
  assert.equal(page.byId('key-thinking').value, 'off', 'Cancel discards the unsaved selection');

  page.byId('key-thinking').value = '';
  await page.window.saveKey();
  saved = JSON.parse(page.calls.filter(call => call.method === 'PUT').at(-1).body);
  assert.equal(saved.thinking_level, '', 'server default is an explicit clear');
  assert.equal(page.byId('key-thinking').value, '');
});

test('a newly configured key can include a thinking preference', async () => {
  const page = await settingsPage({configured: false});
  assert.equal(page.byId('key-thinking').value, '');
  page.byId('key-provider').value = 'anthropic';
  page.byId('key-secret').value = 'new-key';
  page.byId('key-thinking').value = 'low';
  await page.window.saveKey();
  assert.equal(JSON.parse(page.calls.find(call => call.method === 'PUT').body).thinking_level, 'low');
});

test('hosted mode makes no requests for personal agent settings', async () => {
  const page = await settingsPage({configured: true, thinking_level: 'max'}, false);
  page.window.openKeyModal();
  assert.equal(page.byId('key-modal').hidden, true);
  assert.deepEqual(page.calls, []);
});
