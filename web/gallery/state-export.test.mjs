import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const assets = new URL('../../internal/api/assets/gallery/', import.meta.url);

async function fixture(fetch) {
  const blobs = [], downloads = [], timers = [], revoked = [], calls = [];
  const page = loadPageScript(fileURLToPath(new URL('state-api.js', assets)), {
    ID: 'artifact-1',
    apiFetch: async (...args) => { calls.push(args); return fetch(...args); },
    URL: {
      createObjectURL(blob) { blobs.push(blob); return 'blob:export'; },
      revokeObjectURL(url) { revoked.push(url); }
    },
    setTimeout: fn => { timers.push(fn); }
  });
  const createElement = page.document.createElement;
  page.document.createElement = tag => {
    const element = createElement(tag);
    if (tag === 'a') element.click = () => downloads.push({ href: element.href, name: element.download });
    return element;
  };
  await page.loadModule(fileURLToPath(new URL('state-export.js', assets)));
  return { ...page, blobs, downloads, timers, revoked, calls,
    click: () => page.byId('state-export').click() };
}

test('exports current saved strings losslessly, including arbitrary keys, without writing', async () => {
  const state = {
    '': 'empty key', ['__proto__']: 'literal', '../雪': 'line\nquote"',
    json: '{ "value": 3 }', number: '001', empty: ''
  };
  const t = await fixture(async () => Response.json(state));
  t.byId('state-rows').textContent = 'unsaved edits';
  t.byId('state-status').textContent = 'Unsaved changes';
  await t.click();
  assert.deepEqual(JSON.parse(await t.blobs[0].text()), state);
  assert.equal(t.blobs[0].type, 'application/json');
  assert.deepEqual(t.downloads, [{ href: 'blob:export', name: 'artifact-1-localStorage.json' }]);
  assert.equal(t.calls.length, 1);
  assert.equal(t.calls[0][0], '/api/artifacts/artifact-1/state');
  assert.equal(t.calls[0][1], undefined, 'the default GET never writes state');
  assert.equal(t.byId('state-rows').textContent, 'unsaved edits');
  assert.equal(t.byId('state-status').textContent, 'Unsaved changes');
  assert.deepEqual(t.revoked, [], 'the download can start before cleanup');
  t.timers[0]();
  assert.deepEqual(t.revoked, ['blob:export']);
});

test('empty saved state exports as an empty object', async () => {
  const t = await fixture(async () => Response.json({}));
  await t.click();
  assert.equal(await t.blobs[0].text(), '{}\n');
});

for (const [name, fetch, message] of [
  ['refused read', async () => new Response('', { status: 403 }), 'HTTP 403'],
  ['network failure', async () => { throw new Error('offline'); }, 'offline'],
  ['invalid JSON', async () => new Response('not JSON'), 'Could not export state']
]) {
  test(name + ' reports an error without downloading and allows retry', async () => {
    const t = await fixture(fetch);
    await t.click();
    assert.equal(t.downloads.length, 0);
    assert.equal(t.blobs.length, 0);
    assert.ok(t.byId('state-export-status').textContent.includes(message));
    assert.equal(t.byId('state-export').disabled, false);
    await t.click();
    assert.equal(t.calls.length, 2);
  });
}

test('duplicate clicks during a pending read produce one download', async () => {
  let resolve;
  const t = await fixture(() => new Promise(done => { resolve = done; }));
  const pending = t.click();
  assert.equal(t.byId('state-export').disabled, true);
  await t.click();
  assert.equal(t.calls.length, 1);
  resolve(Response.json({ key: 'saved' }));
  await pending;
  assert.equal(t.downloads.length, 1);
  assert.equal(t.byId('state-export').disabled, false);
});
