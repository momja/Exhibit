/* The host page's check of a complete() payload (Exh-v81w).
 *
 * The cases are the ones internal/inference/imagemeta_corpus_test.go runs
 * through the server's prepareComplete: each payload must be refused here
 * with exactly the server's words, or come back with exactly the image bytes
 * the server forwards. It loads the built, embedded copy the browser is
 * served, like the other page-script tests.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const { prepareCompletePayload, MESSAGES } =
  await import(pathToFileURL(path.join(here, '../../internal/api/assets/gallery/complete-payload.js')).href);

const corpus = path.join(here, '../../internal/inference/testdata/imagemeta');
const file = name => readFileSync(path.join(corpus, name));
const cases = JSON.parse(readFileSync(path.join(corpus, 'payloads.json'), 'utf8'));

// The placeholders, expanded as the Go test does: $b64(file) is a corpus file
// in base64, $png(n) a well-formed PNG of exactly n bytes with zero CRCs.
function corpusPNG(n) {
  const out = Buffer.alloc(n);
  out.write('\x89PNG\r\n\x1a\n', 0, 'latin1');
  const chunk = (at, kind, size) => { out.writeUInt32BE(size, at); out.write(kind, at + 4, 'latin1'); return at + 12 + size; };
  let at = chunk(8, 'IHDR', 13);
  out.set([0, 0, 0, 1, 0, 0, 0, 1, 8, 0, 0, 0, 0], 8 + 8);
  at = chunk(at, 'IDAT', n - 8 - 25 - 12 - 12);
  chunk(at, 'IEND', 0);
  return out;
}
const expand = text => text.replace(/\$(b64|png)\(([^)]*)\)/g, (_, kind, arg) =>
  (kind === 'b64' ? file(arg) : corpusPNG(Number(arg))).toString('base64'));
const expandValue = value => JSON.parse(expand(JSON.stringify(value)));

const imageParts = payload => (payload.messages ?? []).flatMap(m => (Array.isArray(m.content) ? m.content : []))
  .filter(p => p.type === 'image');

for (const c of cases) {
  test(`corpus: ${c.name}`, () => {
    const payload = expandValue(c.payload);
    const before = JSON.stringify(payload);
    const got = prepareCompletePayload(payload);
    assert.equal(JSON.stringify(payload), before, 'the payload given is not changed');
    if (c.error) {
      assert.deepEqual(got, { ok: false, message: c.error });
      return;
    }
    assert.equal(got.ok, true, got.message);
    const want = expandValue(c.images ?? []);
    assert.deepEqual(imageParts(got.payload).map(p => ({ mimeType: p.mimeType, data: p.data })), want);
  });
}

test('every refusal the host gives is one the corpus holds the server to', () => {
  const held = new Set(cases.map(c => c.error).filter(Boolean));
  for (const [name, message] of Object.entries(MESSAGES)) {
    const texts = typeof message === 'function' ? ['image/png', 'image/jpeg', 'image/webp'].map(message) : [message];
    assert.ok(texts.some(t => held.has(t)), `no corpus case gives ${name}`);
  }
});

// --- what the corpus cannot say in JSON -----------------------------------------

const b64 = name => file(name).toString('base64');

test('everything but the images is sent as given', () => {
  const payload = {
    system: 'Read receipts.', maxTokens: 200, schema: { type: 'object' },
    messages: [
      { role: 'user', content: 'Earlier question' },
      { role: 'assistant', content: 'Earlier answer' },
      { role: 'user', content: [
        { type: 'text', text: 'What is this?' },
        { type: 'image', mimeType: 'image/jpeg', data: b64('jpeg-all-metadata.in.jpg').replace(/(.{76})/g, '$1\r\n') },
      ] },
    ],
  };
  const got = prepareCompletePayload(payload);
  assert.equal(got.ok, true);
  assert.deepEqual(got.payload, {
    ...payload,
    messages: [payload.messages[0], payload.messages[1], { role: 'user', content: [
      { type: 'text', text: 'What is this?' },
      { type: 'image', mimeType: 'image/jpeg', data: b64('jpeg-all-metadata.out.jpg') },
    ] }],
  });
});

test('the payload is read as JSON will carry it', () => {
  // undefined is dropped, Infinity becomes null, a Date its string, and a
  // lone surrogate U+FFFD, as the server will decode them.
  const got = prepareCompletePayload({
    messages: [{ role: 'user', content: [{ type: 'text', text: 'Hi \ud800' }], extra: undefined }],
    maxTokens: Infinity, system: undefined, schema: { description: new Date(0) },
  });
  assert.equal(got.ok, true);
  assert.deepEqual(got.payload, {
    messages: [{ role: 'user', content: [{ type: 'text', text: 'Hi �' }] }],
    maxTokens: null, schema: { description: '1970-01-01T00:00:00.000Z' },
  });
  // A value JSON cannot carry at all is refused before anything is sent.
  const cyclic = { messages: [] };
  cyclic.messages.push({ role: 'user', content: 'Hi', self: cyclic });
  assert.deepEqual(prepareCompletePayload(cyclic), { ok: false, message: MESSAGES.payloadShape });
  assert.deepEqual(prepareCompletePayload({ prompt: 'Hi', maxTokens: 10n }), { ok: false, message: MESSAGES.payloadShape });
  assert.deepEqual(prepareCompletePayload(undefined), { ok: false, message: MESSAGES.payloadShape });
});

test('mixed-case keys anywhere are refused rather than sent', () => {
  const image = { type: 'image', mimeType: 'image/jpeg', data: b64('jpeg-all-metadata.in.jpg') };
  for (const [payload, message] of [
    [{ Messages: [{ role: 'user', content: [image] }] }, MESSAGES.payloadShape],
    [{ messages: [{ role: 'user', Content: [image] }] }, MESSAGES.messageShape],
    [{ messages: [{ role: 'user', content: [{ ...image, data: undefined, Data: image.data }] }] }, MESSAGES.contentShape],
    [{ messages: [{ role: 'user', content: [{ type: 'image', MimeType: 'image/jpeg', data: image.data }] }] }, MESSAGES.contentShape],
    [{ messages: [{ role: 'user', content: [{ Type: 'image', mimeType: 'image/jpeg', data: image.data }] }] }, MESSAGES.contentShape],
  ]) {
    assert.deepEqual(prepareCompletePayload(payload), { ok: false, message });
  }
});
