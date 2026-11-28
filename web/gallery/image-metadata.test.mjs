/* Image metadata stripping on the host page (Exh-v81w).
 *
 * image-metadata.js is pure bytes-in, bytes-out with no DOM, so these call it
 * directly. The cases mirror internal/inference/imagemeta_test.go, whose rules
 * it shares. It loads the built, embedded copy the browser is served, like
 * the other page-script tests.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const { stripImageMetadata, stripPayloadImages, MalformedImageError } =
  await import(pathToFileURL(path.join(here, '../../internal/api/assets/gallery/image-metadata.js')).href);

const MARKER = 'GPS-MARKER-v81w';
const enc = new TextEncoder();
const bytes = (...parts) => {
  const list = parts.map(p => (typeof p === 'string' ? Uint8Array.from(p, c => c.charCodeAt(0)) : Uint8Array.from(p)));
  const out = new Uint8Array(list.reduce((n, p) => n + p.length, 0));
  let at = 0;
  for (const p of list) { out.set(p, at); at += p.length; }
  return out;
};
const hasMarker = b => Buffer.from(b).includes(enc.encode(MARKER));
const b64 = b => Buffer.from(b).toString('base64');

// --- JPEG -------------------------------------------------------------------

const seg = (marker, payload) => bytes([0xff, marker, (payload.length + 2) >> 8, (payload.length + 2) & 0xff], payload);
const ICC = 'ICC_PROFILE\0\x01\x01fake-profile-bytes';
// A structurally complete JPEG: tables, frame and scan headers, then scan data
// with a stuffed byte and a restart. The strip never decodes pixels, so the
// table contents need not be real.
const jpegBody = bytes(
  seg(0xdb, '\0' + 'q'.repeat(64)), // DQT
  seg(0xc0, '\x08\0\x01\0\x01\x01\x01\x11\0'), // SOF0
  seg(0xc4, '\0' + '\x01'.repeat(16) + 'h'), // DHT
  seg(0xda, '\x01\x01\0\0\x3f\0'), // SOS
  [0x12, 0xff, 0x00, 0x34, 0xff, 0xd0, 0x56],
  [0xff, 0xd9], // EOI
);
const plainJPEG = bytes([0xff, 0xd8], seg(0xe0, 'JFIF\0\x01\x02\0\0\x01\0\x01\0\0'), jpegBody);
const metadataJPEG = bytes(
  [0xff, 0xd8],
  seg(0xe0, 'JFIF\0\x01\x02\0\0\x01\0\x01\0\0'),
  seg(0xe1, 'Exif\0\0MM\0*GPSLatitude ' + MARKER),
  seg(0xe1, 'http://ns.adobe.com/xap/1.0/\0<x:xmpmeta>' + MARKER + '</x:xmpmeta>'),
  seg(0xe2, 'MPF\0' + MARKER),
  seg(0xe2, ICC),
  seg(0xed, 'Photoshop 3.0\0' + MARKER),
  seg(0xee, 'Adobe\0d\0\0\0\0\x01'),
  seg(0xfe, 'camera comment ' + MARKER),
  jpegBody,
  'motion-photo-trailer ' + MARKER,
);
const strippedJPEG = bytes(
  [0xff, 0xd8],
  seg(0xe0, 'JFIF\0\x01\x02\0\0\x01\0\x01\0\0'),
  seg(0xe2, ICC),
  seg(0xee, 'Adobe\0d\0\0\0\0\x01'),
  jpegBody,
);

// --- PNG --------------------------------------------------------------------

const chunk = (kind, data) => bytes(
  [(data.length >>> 24) & 0xff, (data.length >>> 16) & 0xff, (data.length >>> 8) & 0xff, data.length & 0xff],
  kind, data, [0xde, 0xad, 0xbe, 0xef], // the CRC is copied, never recomputed
);
const SIG = '\x89PNG\r\n\x1a\n';
const ihdr = chunk('IHDR', '\0\0\0\x01\0\0\0\x01\x08\0\0\0\0');
const kept = bytes(chunk('gAMA', '\0\0\xb1\x8f'), chunk('pHYs', '\0\0\x0b\x13\0\0\x0b\x13\x01'));
const idat = bytes(chunk('IDAT', 'x\x9cc\0\x01\0\0\x05\0\x01'), chunk('IEND', ''));
const metadataPNG = bytes(
  SIG, ihdr,
  chunk('eXIf', 'MM\0*' + MARKER),
  chunk('tEXt', 'Comment\0' + MARKER),
  chunk('iTXt', 'XML:com.adobe.xmp\0\0\0\0\0' + MARKER),
  chunk('zTXt', 'Raw profile type exif\0\0x\x9c' + MARKER),
  chunk('tIME', '\x07\xea\x0b\x1c\x0c\0\0'),
  chunk('prVt', MARKER),
  kept, idat,
  'trailer ' + MARKER,
);
const strippedPNG = bytes(SIG, ihdr, kept, idat);

// --- WebP -------------------------------------------------------------------

const u32le = n => [n & 0xff, (n >>> 8) & 0xff, (n >>> 16) & 0xff, (n >>> 24) & 0xff];
const wchunk = (kind, data) => bytes(kind, u32le(data.length), data, data.length % 2 ? [0] : []);
const riff = (...chunks) => {
  const body = bytes(...chunks);
  return bytes('RIFF', u32le(4 + body.length), 'WEBP', body);
};
const vp8x = flags => wchunk('VP8X', bytes([flags], '\0'.repeat(9)));
const icc = wchunk('ICCP', 'fake-profile');
const vp8 = wchunk('VP8 ', '\0'.repeat(14));
const metadataWebP = bytes(
  riff(vp8x(0x20 | 0x08 | 0x04), icc, vp8,
    wchunk('EXIF', 'MM\0*' + MARKER + '!'), // odd length, padded
    wchunk('XMP ', '<x:xmpmeta>' + MARKER + '</x:xmpmeta>'),
    wchunk('ZZZZ', MARKER)),
  'trailer ' + MARKER,
);
const strippedWebP = riff(vp8x(0x20), icc, vp8);

// --- stripImageMetadata -------------------------------------------------------

for (const [mimeType, input, want] of [
  ['image/jpeg', metadataJPEG, strippedJPEG],
  ['image/png', metadataPNG, strippedPNG],
  ['image/webp', metadataWebP, strippedWebP],
]) {
  test(`${mimeType}: metadata goes, everything needed to draw the pixels stays`, () => {
    const got = stripImageMetadata(input, mimeType);
    assert.equal(hasMarker(got), false, 'metadata survived the strip');
    assert.deepEqual(got, want);
    assert.deepEqual(stripImageMetadata(got, mimeType), got, 'a file with nothing to strip comes back as it was');
  });
}

test('a file with no metadata is unchanged', () => {
  assert.deepEqual(stripImageMetadata(plainJPEG, 'image/jpeg'), plainJPEG);
});

test('WebP: the EXIF and XMP flags are cleared and the RIFF size rewritten', () => {
  const got = stripImageMetadata(metadataWebP, 'image/webp');
  assert.equal(got[20], 0x20);
  assert.equal(Buffer.from(got).readUInt32LE(4), got.length - 8);
});

test('JPEG: a later scan is walked too, with the same rules', () => {
  const eoi = plainJPEG.length - 2;
  const scanData = [0x12, 0xff, 0x00, 0x34, 0xff, 0xd0, 0x56, 0xff, 0xff];
  const input = bytes(plainJPEG.subarray(0, eoi), seg(0xfe, MARKER), seg(0xda, '\x01\x01\0\0\x3f\0'), scanData, [0xff, 0xd9]);
  const got = stripImageMetadata(input, 'image/jpeg');
  assert.equal(hasMarker(got), false);
  assert.deepEqual(got.subarray(got.length - 11), Uint8Array.from([...scanData, 0xff, 0xd9]));
});

const setU32 = (b, at, n, little) => {
  const out = Uint8Array.from(b);
  const v = new DataView(out.buffer);
  v.setUint32(at, n, little);
  return out;
};
for (const [name, mimeType, raw] of [
  ['jpeg truncated in a segment', 'image/jpeg', metadataJPEG.subarray(0, 40)],
  ['jpeg truncated in the scan', 'image/jpeg', plainJPEG.subarray(0, plainJPEG.length - 4)],
  ['jpeg without EOI', 'image/jpeg', plainJPEG.subarray(0, plainJPEG.length - 2)],
  ['jpeg length past the end', 'image/jpeg', bytes([0xff, 0xd8, 0xff, 0xe1, 0x7f, 0xff], MARKER)],
  ['jpeg EOI before any scan', 'image/jpeg', bytes([0xff, 0xd8, 0xff, 0xfe, 0x00, 0x03, 0x78, 0xff, 0xd9])],
  ['jpeg junk between segments', 'image/jpeg', bytes([0xff, 0xd8], seg(0xfe, 'x'), [0x00], jpegBody)],
  ['png truncated', 'image/png', metadataPNG.subarray(0, 60)],
  ['png without IEND', 'image/png', strippedPNG.subarray(0, strippedPNG.length - 12)],
  ['png not starting with IHDR', 'image/png', bytes(SIG, chunk('tEXt', MARKER))],
  ['png length past the end', 'image/png', setU32(strippedPNG, 8 + 25, 1 << 30, false)],
  ['webp RIFF size past the end', 'image/webp', setU32(metadataWebP, 4, metadataWebP.length, true)],
  ['webp chunk past the end', 'image/webp', setU32(riff(vp8), 16, 1000, true)],
  ['webp short VP8X', 'image/webp', riff(wchunk('VP8X', '\x08'), vp8)],
]) {
  test(`malformed: ${name} is refused, not sent as it came`, () => {
    assert.throws(() => stripImageMetadata(raw, mimeType), e =>
      e instanceof MalformedImageError && e.message === `An image's data is not a well-formed ${mimeType} file.`);
  });
}

// --- stripPayloadImages ---------------------------------------------------------

const imageMessage = (...content) => ({ messages: [{ role: 'user', content }] });

test('every image part in a complete() payload is sent stripped, as plain base64', () => {
  const payload = {
    system: 'Read receipts.',
    maxTokens: 200,
    messages: [
      { role: 'user', content: 'Earlier question' },
      { role: 'assistant', content: 'Earlier answer' },
      { role: 'user', content: [
        { type: 'text', text: 'What is this?' },
        { type: 'image', mimeType: 'image/jpeg', data: b64(metadataJPEG) },
        // A data: URL with no mimeType takes the URL's type.
        { type: 'image', data: 'data:image/png;base64,' + b64(metadataPNG) },
        { type: 'image', mimeType: 'IMAGE/WEBP', data: 'data:image/webp;base64,' + b64(metadataWebP).replace(/(.{76})/g, '$1\r\n') },
      ] },
    ],
  };
  const before = JSON.stringify(payload);
  const got = stripPayloadImages(payload);
  assert.equal(JSON.stringify(payload), before, 'the input is not mutated');
  assert.equal(got.system, 'Read receipts.');
  assert.equal(got.maxTokens, 200);
  assert.equal(got.messages[0], payload.messages[0]);
  assert.equal(got.messages[1], payload.messages[1]);
  const parts = got.messages[2].content;
  assert.deepEqual(parts[0], { type: 'text', text: 'What is this?' });
  assert.deepEqual(parts[1], { type: 'image', mimeType: 'image/jpeg', data: b64(strippedJPEG) });
  assert.deepEqual(parts[2], { type: 'image', mimeType: 'image/png', data: b64(strippedPNG) });
  assert.deepEqual(parts[3], { type: 'image', mimeType: 'IMAGE/WEBP', data: b64(strippedWebP) });
});

test('what the server refuses is left for it to refuse', () => {
  const gif = 'R0lGODlhAQABAAAAACw=';
  const over = new Uint8Array((2 << 20) + 1);
  over.set(strippedPNG);
  for (const part of [
    { type: 'image', mimeType: 'image/gif', data: gif },
    { type: 'image', mimeType: 'image/png', data: '!!not base64!!' },
    { type: 'image', mimeType: 'image/png', data: b64(metadataPNG).slice(0, -1) }, // no padding
    { type: 'image', mimeType: 'image/png', data: '' },
    { type: 'image', mimeType: 'image/jpeg', data: b64(metadataPNG) }, // mislabelled
    { type: 'image', mimeType: 'image/jpeg', data: 'data:image/png;base64,' + b64(metadataPNG) },
    { type: 'image', mimeType: 'image/png', data: 'data:image/png,' + b64(metadataPNG) },
    { type: 'image', data: b64(metadataPNG) }, // no type at all
    { type: 'image', mimeType: 7, data: b64(metadataPNG) },
    { type: 'image', mimeType: 'image/png', data: b64(over) }, // over 2 MB
    { type: 'image', mimeType: 'image/png' },
    { type: 'audio', mimeType: 'image/png', data: b64(metadataPNG) },
  ]) {
    const payload = imageMessage(part);
    assert.equal(stripPayloadImages(payload), payload, JSON.stringify(part).slice(0, 80));
  }
  for (const payload of [null, 'text', { prompt: 'Hi' }, { messages: 'x' }, { messages: [null, { role: 'user', content: 'Hi' }] }]) {
    assert.equal(stripPayloadImages(payload), payload);
  }
});

test('an image part keeps its other fields, for the server to judge', () => {
  const got = stripPayloadImages(imageMessage({ type: 'image', mimeType: 'image/png', data: b64(metadataPNG), url: 'x' }));
  assert.deepEqual(got.messages[0].content[0], { type: 'image', mimeType: 'image/png', data: b64(strippedPNG), url: 'x' });
});

test('a JSON null mimeType reads as none, as on the server', () => {
  const got = stripPayloadImages(imageMessage({ type: 'image', mimeType: null, data: 'data:image/png;base64,' + b64(metadataPNG) }));
  assert.deepEqual(got.messages[0].content[0], { type: 'image', mimeType: 'image/png', data: b64(strippedPNG) });
});

test('a broken file of an accepted type throws rather than being sent', () => {
  assert.throws(() => stripPayloadImages(imageMessage({ type: 'image', mimeType: 'image/jpeg', data: b64(plainJPEG.subarray(0, 30)) })),
    MalformedImageError);
});
