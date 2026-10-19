import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/assets-export.js', import.meta.url));

async function fixture(fetch) {
  const blobs = [], downloads = [], calls = [];
  const page = loadPageScript([], {
    ID: 'artifact 1',
    apiFetch: async (...args) => { calls.push(args); return fetch(...args); },
    URL: { createObjectURL(blob) { blobs.push(blob); return 'blob:assets'; }, revokeObjectURL() {} },
    setTimeout() {}
  });
  const createElement = page.document.createElement;
  page.document.createElement = tag => {
    const element = createElement(tag);
    if (tag === 'a') element.click = () => downloads.push({ href: element.href, name: element.download });
    return element;
  };
  await page.loadModule(script);
  return { ...page, blobs, downloads, calls, click: () => page.byId('assets-export').click() };
}

test('downloads the current ZIP bytes through the shared credential client', async () => {
  const bytes = new Uint8Array([80, 75, 0, 255, 128]);
  const t = await fixture(async () => new Response(bytes, { headers: { 'Content-Type': 'application/zip' } }));
  await t.click();
  assert.equal(t.calls[0][0], '/api/artifacts/artifact%201/assets/export');
  assert.equal(t.calls[0][1], undefined, 'export is a GET');
  assert.equal(t.blobs[0].type, 'application/zip');
  assert.deepEqual(new Uint8Array(await t.blobs[0].arrayBuffer()), bytes);
  assert.deepEqual(t.downloads, [{ href: 'blob:assets', name: 'artifact 1-assets.zip' }]);
  assert.equal(t.byId('assets-export').disabled, false);
  await t.click();
  assert.equal(t.calls.length, 2, 'each export fetches current assets');
});

for (const [name, fetch, message] of [
  ['unreadable asset', async () => new Response('missing blob', { status: 500 }), 'HTTP 500'],
  ['forbidden read', async () => new Response('', { status: 403 }), 'HTTP 403'],
  ['network failure', async () => { throw new Error('offline'); }, 'offline'],
  ['interrupted download', async () => ({ ok: true, blob: async () => { throw new Error('interrupted'); } }), 'interrupted']
]) {
  test(name + ' reports an error without saving an incomplete file', async () => {
    const t = await fixture(fetch);
    await t.click();
    assert.equal(t.downloads.length, 0);
    assert.equal(t.blobs.length, 0);
    assert.ok(t.byId('assets-export-status').textContent.includes(message));
    assert.equal(t.byId('assets-export').disabled, false);
    await t.click();
    assert.equal(t.calls.length, 2, 'failed downloads can be retried');
  });
}

test('duplicate clicks during export send one request', async () => {
  let resolve;
  const t = await fixture(() => new Promise(done => { resolve = done; }));
  const pending = t.click();
  assert.equal(t.byId('assets-export').disabled, true);
  await t.click();
  assert.equal(t.calls.length, 1);
  resolve(new Response('zip bytes'));
  await pending;
  assert.equal(t.downloads.length, 1);
  assert.equal(t.byId('assets-export').disabled, false);
});
