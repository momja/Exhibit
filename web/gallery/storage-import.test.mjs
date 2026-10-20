import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/storage-import.js', import.meta.url));
const conflict = () => Response.json({ error: 'confirmation needed', confirmation_required: true }, { status: 409 });
const success = () => new Response(null, { status: 204 });

async function setup(kind, responses, answer = true) {
  const calls = [], prompts = [], events = [];
  const page = loadPageScript([], {
    ID: 'artifact-1', Event, CustomEvent,
    apiFetch: async (url, options) => {
      calls.push({ url, options });
      const response = responses.shift();
      if (response instanceof Error) throw response;
      return typeof response === 'function' ? response() : response;
    },
    confirm: message => { prompts.push(message); return answer; },
  });
  page.document.body = page.byId('body');
  for (const name of ['state-imported', 'assets-imported', 'assets-changed']) {
    page.document.body.addEventListener('exhibit:' + name, () => events.push(name));
  }
  await page.loadModule(script);
  const input = page.byId(kind + '-import-file');
  const file = new Blob([kind === 'state' ? '{"a":"b"}' : 'ZIP'], { type: kind === 'state' ? 'application/json' : 'application/zip' });
  const upload = () => { input.files = [file]; return input.dispatchEvent({ type: 'change' }); };
  return { ...page, calls, prompts, events, upload, input, file, button: page.byId(kind + '-import'), status: page.byId(kind + '-import-status') };
}

for (const kind of ['state', 'assets']) {
  test(kind + ': empty target imports without confirmation and refreshes', async () => {
    const t = await setup(kind, [success()]);
    await t.upload();
    assert.equal(t.prompts.length, 0);
    assert.equal(t.calls.length, 1);
    assert.equal(t.calls[0].options.body, t.file);
    assert.equal(t.calls[0].options.method, 'POST');
    assert.deepEqual(t.events, kind === 'state' ? ['state-imported'] : ['assets-imported', 'assets-changed']);
    assert.equal(t.status.textContent, 'Imported.');
    assert.equal(t.button.disabled, false);
    assert.equal(t.input.value, '');
  });
  test(kind + ': populated target requires explicit erasure confirmation', async () => {
    const t = await setup(kind, [conflict(), success()]);
    await t.upload();
    assert.equal(t.prompts.length, 1);
    assert.match(t.prompts[0], /erase all existing stored/);
    assert.equal(t.calls[1].url, '/api/artifacts/artifact-1/' + kind + '/import?replace=true');
    assert.equal(t.calls[1].options.body, t.file);
    assert.equal(t.status.textContent, 'Imported.');
  });
  test(kind + ': cancelled confirmation sends no replacement and permits retry', async () => {
    const t = await setup(kind, [conflict(), success()], false);
    await t.upload();
    assert.equal(t.calls.length, 1);
    assert.equal(t.events.length, 0);
    assert.equal(t.status.textContent, 'Import cancelled.');
    await t.upload();
    assert.equal(t.status.textContent, 'Imported.');
    assert.equal(t.calls.length, 2);
  });
  test(kind + ': validation and network errors preserve the panel and permit retry', async () => {
    const t = await setup(kind, [Response.json({ error: 'Invalid file' }, { status: 400 }), new Error('offline'), success()]);
    await t.upload();
    assert.match(t.status.textContent, /Invalid file/);
    assert.equal(t.prompts.length, 0);
    assert.equal(t.events.length, 0);
    await t.upload();
    assert.match(t.status.textContent, /offline/);
    await t.upload();
    assert.equal(t.status.textContent, 'Imported.');
    assert.equal(t.status.classList.contains('error'), false);
  });
  test(kind + ': no selected file and duplicate uploads are ignored', async () => {
    let finish;
    const t = await setup(kind, [() => new Promise(resolve => { finish = resolve; })]);
    t.input.files = [];
    await t.input.dispatchEvent({ type: 'change' });
    assert.equal(t.calls.length, 0);
    const pending = t.upload();
    await t.upload();
    assert.equal(t.calls.length, 1);
    assert.equal(t.button.disabled, true);
    finish(success());
    await pending;
    assert.equal(t.button.disabled, false);
  });
}
