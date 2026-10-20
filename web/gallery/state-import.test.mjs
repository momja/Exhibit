import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const assets = new URL('../../internal/api/assets/gallery/', import.meta.url);
const settle = () => new Promise(resolve => setTimeout(resolve, 0));

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

async function inspector(apiFetch) {
  const page = loadPageScript(fileURLToPath(new URL('state-api.js', assets)), {
    ID: 'artifact-1', TITLE: 'Demo', Event, CustomEvent, confirm: () => true, apiFetch,
  });
  const create = page.document.createElement;
  page.document.createElement = tag => {
    const element = create(tag);
    element.append = (...children) => element.children.push(...children);
    return element;
  };
  await page.loadModule(fileURLToPath(new URL('state.js', assets)));
  await page.loadModule(fileURLToPath(new URL('storage-import.js', assets)));
  page.byId('state-panel').open = true;
  await page.byId('state-panel').dispatchEvent({ type: 'toggle' });
  await settle();
  page.add = async key => {
    page.byId('state-add-key').value = key;
    page.byId('state-add-type').value = 'string';
    await page.byId('state-add-btn').click();
  };
  page.upload = () => {
    page.byId('state-import-file').files = [new Blob(['{"imported":"new"}'])];
    return page.byId('state-import-file').dispatchEvent({ type: 'change' });
  };
  return page;
}

test('a pending multi-key save finishes before a state import can start', async () => {
  let data = {}, writes = 0;
  const firstWrite = deferred(), calls = [];
  const page = await inspector(async (url, options = {}) => {
    calls.push(options.method);
    if (url.endsWith('/import')) {
      data = JSON.parse(await options.body.text());
      return new Response(null, { status: 204 });
    }
    if (options.method === 'GET') return Response.json(data);
    const { key, value } = JSON.parse(options.body);
    data[key] = value;
    if (++writes === 1) return firstWrite.promise;
    return new Response(null, { status: 204 });
  });
  await page.add('a');
  await page.add('b');
  const saving = page.byId('state-save').click();
  assert.equal(page.byId('state-controls').disabled, true);
  await page.upload();
  assert.equal(calls.includes('POST'), false, 'an upload cannot overlap the captured save loop');
  firstWrite.resolve(new Response(null, { status: 204 }));
  await saving;
  assert.equal(page.byId('state-controls').disabled, false);
  await page.upload();
  assert.deepEqual(data, { imported: 'new' });
  assert.deepEqual(calls, ['GET', 'PUT', 'PUT', 'POST', 'GET']);
});

test('state import blocks saves and erasure until its inspector refresh completes', async () => {
  const imported = deferred(), refresh = deferred(), calls = [];
  let reads = 0;
  const page = await inspector(async (url, options = {}) => {
    calls.push(options.method);
    if (options.method === 'POST') return imported.promise;
    if (options.method === 'GET') return ++reads === 1 ? Response.json({}) : refresh.promise;
    throw new Error('old state must not be written');
  });
  await page.add('old');
  const uploading = page.upload();
  await page.byId('state-save').click();
  await page.byId('state-erase').click();
  imported.resolve(new Response(null, { status: 204 }));
  await settle();
  assert.equal(page.byId('state-controls').disabled, true, 'the gate includes the refresh GET');
  await page.byId('state-save').click();
  refresh.resolve(Response.json({ imported: 'new' }));
  await uploading;
  assert.equal(page.byId('state-controls').disabled, false);
  assert.deepEqual(calls, ['GET', 'POST', 'GET']);
  assert.deepEqual(page.byId('state-rows').children.map(row => row.dataset.key), ['imported']);
});

test('a failed post-import refresh cannot leave old drafts available for saving', async () => {
  let reads = 0;
  const calls = [];
  const page = await inspector(async (url, options = {}) => {
    calls.push(options.method);
    if (options.method === 'POST') return new Response(null, { status: 204 });
    if (options.method === 'GET' && ++reads === 1) return Response.json({});
    throw new Error('refresh offline');
  });
  await page.add('old-draft');
  await page.upload();
  assert.match(page.byId('state-status').textContent, /refresh offline/);
  assert.equal(page.byId('state-controls').disabled, false);
  assert.deepEqual(page.byId('state-rows').children, []);
  await page.byId('state-save').click();
  assert.deepEqual(calls, ['GET', 'POST', 'GET']);
});

for (const olderRead of ['response', 'JSON body', 'network error', 'HTTP error']) {
  test('import refresh ignores an obsolete ' + olderRead, async () => {
    const old = deferred();
    const imported = deferred();
    let calls = 0;
    const page = loadPageScript(fileURLToPath(new URL('state-api.js', assets)), {
      ID: 'artifact-1',
      apiFetch: () => (++calls === 1 ? old.promise : imported.promise),
    });
    const createElement = page.document.createElement;
    page.document.createElement = tag => {
      const element = createElement(tag);
      element.append = (...children) => element.children.push(...children);
      return element;
    };
    await page.loadModule(fileURLToPath(new URL('state.js', assets)));
    page.byId('state-panel').open = true;
    await page.byId('state-panel').dispatchEvent({ type: 'toggle' });
    let finishOld;
    if (olderRead === 'JSON body') {
      const body = deferred();
      old.resolve({ ok: true, json: () => body.promise });
      finishOld = () => body.resolve({ obsolete: 'old' });
      await settle();
    } else {
      finishOld = () => {
        if (olderRead === 'network error') old.reject(new Error('old failure'));
        else old.resolve(olderRead === 'HTTP error' ? new Response(null, { status: 500 }) : Response.json({ obsolete: 'old' }));
      };
    }
    const refresh = page.document.body.dispatchEvent({ type: 'exhibit:state-imported' });
    imported.resolve(Response.json({ imported: 'new' }));
    await refresh;
    const keys = () => page.byId('state-rows').children.map(row => row.dataset.key);
    assert.deepEqual(keys(), ['imported']);
    finishOld();
    await settle();
    assert.deepEqual(keys(), ['imported']);
    assert.equal(page.byId('state-status').textContent, '');
    assert.equal(page.byId('state-summary-text').textContent, 'Stored state · 1 key');
    assert.equal(calls, 2);
  });
}
