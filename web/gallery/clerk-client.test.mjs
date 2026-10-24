import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = (await readFile(new URL('./clerk-client.js', import.meta.url), 'utf8'))
  .replace("'./clerk-appearance.js'", JSON.stringify(new URL('./clerk-appearance.js', import.meta.url).href));

for (const stage of ['script download', 'SDK initialization']) {
  test(`Clerk reports a timeout during ${stage}`, async t => {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    const scripts = [];
    const oldDocument = globalThis.document;
    const oldWindow = globalThis.window;
    globalThis.document = {
      querySelector: selector => ({ content: selector.includes('publishable') ? 'pk_test_example' : 'https://example.clerk.accounts.dev' }),
      createElement: () => ({ dataset: {} }),
      head: { append: script => scripts.push(script) },
    };
    globalThis.window = { Clerk: { load: () => new Promise(() => {}) } };
    t.after(() => { globalThis.document = oldDocument; globalThis.window = oldWindow; });
    const { clerkClient } = await import(`data:text/javascript,${encodeURIComponent(source)}#${encodeURIComponent(stage)}`);
    const pending = clerkClient();
    const rejected = assert.rejects(pending, /account service did not respond/);
    if (stage === 'SDK initialization') {
      scripts.forEach(script => script.onload());
      await Promise.resolve(); await Promise.resolve();
    }
    t.mock.timers.tick(15000);
    await rejected;
  });
}
