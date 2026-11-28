/* Image metadata stripping on the host page (Exh-v81w).
 *
 * image-metadata.js is pure bytes-in, bytes-out with no DOM, so these call it
 * directly, over the corpus internal/inference/imagemeta_corpus_test.go holds
 * the Go implementation to: the same input files, the same expected output
 * bytes, the same refusals. It loads the built, embedded copy the browser is
 * served, like the other page-script tests.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const { stripImageMetadata, ImageStructureError } =
  await import(pathToFileURL(path.join(here, '../../internal/api/assets/gallery/image-metadata.js')).href);

const corpus = path.join(here, '../../internal/inference/testdata/imagemeta');
const file = name => new Uint8Array(readFileSync(path.join(corpus, name)));
const cases = JSON.parse(readFileSync(path.join(corpus, 'cases.json'), 'utf8'));

// What no stripped image may contain, as in the Go corpus test.
const FORBIDDEN = [
  'GPS-MARKER-v81w', 'Exif\0\0', 'http://ns.adobe.com/xap', '<x:xmpmeta', 'ICC_PROFILE', 'JFIF\0', 'JFXX\0',
  'MPF\0', 'Photoshop 3.0', 'eXIf', 'tEXt', 'iTXt', 'zTXt', 'tIME', 'iCCP', 'ICCP', 'EXIF', 'XMP ',
];
const contains = (bytes, text) => Buffer.from(bytes).includes(Buffer.from(text, 'latin1'));

test('the corpus is there', () => assert.ok(cases.length >= 20, `${cases.length} cases`));

for (const c of cases) {
  test(`corpus: ${c.name}`, () => {
    const input = file(c.input);
    if (c.refused) {
      assert.throws(() => stripImageMetadata(input, c.mimeType), e => e instanceof ImageStructureError && e.reason === c.refused);
      return;
    }
    const got = stripImageMetadata(input, c.mimeType);
    assert.deepEqual(Buffer.from(got), Buffer.from(file(c.output)), 'the output differs from the Go implementation\'s');
    for (const text of FORBIDDEN) assert.equal(contains(got, text), false, `the output still carries ${JSON.stringify(text)}`);
    assert.deepEqual(stripImageMetadata(got, c.mimeType), got, 'stripping twice changed the file');
  });
}

test('the input is never mutated', () => {
  for (const c of cases) {
    const input = file(c.input);
    const before = Buffer.from(input);
    try { stripImageMetadata(input, c.mimeType); } catch { /* refused */ }
    assert.deepEqual(Buffer.from(input), before, c.name);
  }
});

test('WebP: the ICC, EXIF and XMP flags are cleared and the RIFF size rewritten', () => {
  const got = stripImageMetadata(file('webp-extended-metadata.in.webp'), 'image/webp');
  assert.equal(got[20], 0x10, 'only the alpha flag is left');
  assert.equal(Buffer.from(got).readUInt32LE(4), got.length - 8);
});

test('a type the strip does not know is refused', () => {
  assert.throws(() => stripImageMetadata(file('png-plain.in.png'), 'image/gif'), e => e.reason === 'malformed');
});
