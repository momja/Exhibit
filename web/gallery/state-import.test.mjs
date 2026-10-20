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
