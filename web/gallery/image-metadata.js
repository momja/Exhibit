/* Image metadata stripping for window.exhibit.ai (Exh-v81w).
 *
 * Every image a tool sends to complete() is rewritten before it leaves the
 * device, keeping only what is needed to draw its pixels: no location,
 * camera, timestamps, comments, editing history, thumbnails or colour
 * profiles (an ICC profile names the device that made it). The server strips
 * again as a backstop for clients that bypass this page
 * (internal/inference/imagemeta.go). The two follow the same rules, and both
 * are held to the shared corpus in internal/inference/testdata/imagemeta:
 *
 *   - JPEG: walk the segments from SOI. Keep APP14 (Adobe, which decides the
 *     colour transform) and every non-APP segment (tables, frame and scan
 *     headers, restart intervals). Drop every other APPn, APP0 included (JFIF
 *     and JFXX can carry a thumbnail; APP1 is EXIF and XMP; APP2 is ICC and
 *     MPF), and COM. Scan data is copied verbatim up to the next marker, and
 *     walking resumes there, so a progressive file's later segments follow
 *     the same rules. A scan needs a frame header (SOFn) before it, and EOI
 *     needs a scan before it. Output ends at EOI: anything after it (a second
 *     picture, a motion photo's video, a vendor trailer) is dropped.
 *   - PNG: IHDR comes first and is 13 bytes. Keep IHDR, PLTE, IDAT, IEND,
 *     tRNS, gAMA, cHRM, sRGB, sBIT, pHYs, bKGD, cICP, mDCV and cLLI; drop
 *     every other chunk (eXIf, tEXt, zTXt, iTXt, tIME, iCCP and anything
 *     unknown). Kept chunks are copied verbatim, CRC included. At least one
 *     IDAT comes before IEND, IEND is empty, and output ends there. acTL,
 *     fcTL or fdAT make it an APNG, which is refused.
 *   - WebP: keep VP8, VP8L, VP8X and ALPH; drop every other chunk (EXIF, XMP,
 *     ICCP and anything unknown). VP8X's ICC (0x20), EXIF (0x08) and XMP
 *     (0x04) flags are cleared and the RIFF size is rewritten. A VP8 or VP8L
 *     image chunk is required. ANIM, ANMF or VP8X's animation flag (0x02)
 *     make it animated, which is refused. Output ends at the RIFF's end.
 *
 * Orientation goes with EXIF, so a phone photo that relied on it may reach
 * the model rotated, and colours fall back to sRGB without the profile.
 * Pixels are never decoded or re-encoded.
 *
 * A file that cannot be walked cleanly to its end throws ImageStructureError
 * with reason 'malformed'; an animated one, reason 'animated'. Either way it
 * is never sent. The walk stops at the first problem, so the two
 * implementations refuse the same file for the same reason.
 */

export class ImageStructureError extends Error {
  constructor(reason) {
    super(reason === 'animated' ? 'animated image' : 'malformed image');
    this.name = 'ImageStructureError';
    this.reason = reason;
  }
}

const MALFORMED = 'malformed';
const ANIMATED = 'animated';

/** Returns bytes (a Uint8Array) without metadata, or throws ImageStructureError. */
export function stripImageMetadata(bytes, mimeType) {
  const strip = { 'image/jpeg': stripJPEG, 'image/png': stripPNG, 'image/webp': stripWebP }[mimeType];
  const out = strip ? strip(bytes) : MALFORMED;
  if (typeof out === 'string') throw new ImageStructureError(out);
  return out;
}

function keepJPEGSegment(marker) {
  if (marker === 0xee) return true; // APP14 Adobe
  return !((marker >= 0xe0 && marker <= 0xef) || marker === 0xfe); // every other APPn, COM
}

// SOF0–SOF15. C4 (DHT), C8 (JPG) and CC (DAC) share the range and are not.
function isJPEGFrame(marker) {
  return marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc;
}

// Each strip returns the stripped bytes, or MALFORMED / ANIMATED.
function stripJPEG(raw) {
  const n = raw.length;
  if (n < 4 || raw[0] !== 0xff || raw[1] !== 0xd8) return MALFORMED;
  const out = new Writer(n);
  out.bytes([0xff, 0xd8]);
  let sawFrame = false;
  let sawScan = false;
  let i = 2;
  for (;;) {
    if (i >= n || raw[i] !== 0xff) return MALFORMED;
    while (i < n && raw[i] === 0xff) i++; // fill bytes before a marker
    if (i >= n) return MALFORMED;
    const marker = raw[i++];
    if (marker === 0xd9) { // EOI
      if (!sawScan) return MALFORMED;
      out.bytes([0xff, 0xd9]);
      return out.done();
    }
    if (marker === 0x00 || marker === 0xd8) return MALFORMED;
    if (marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) { // TEM, RSTn
      out.bytes([0xff, marker]);
      continue;
    }
    if (i + 2 > n) return MALFORMED;
    const length = (raw[i] << 8) | raw[i + 1];
    if (length < 2 || i + length > n) return MALFORMED;
    if (isJPEGFrame(marker)) sawFrame = true;
    if (marker === 0xda && !sawFrame) return MALFORMED; // a scan with no frame header
    if (keepJPEGSegment(marker)) {
      out.bytes([0xff, marker]);
      out.slice(raw, i, i + length);
    }
    i += length;
    if (marker !== 0xda) continue; // not SOS
    // Entropy-coded data runs to the next marker: FF 00 is a stuffed byte,
    // FF D0–D7 a restart, FF FF fill.
    sawScan = true;
    const start = i;
    for (;;) {
      if (i + 1 >= n) return MALFORMED;
      if (raw[i] !== 0xff) { i++; continue; }
      const next = raw[i + 1];
      if (next === 0x00 || (next >= 0xd0 && next <= 0xd7)) { i += 2; continue; }
      if (next === 0xff) { i++; continue; }
      break;
    }
    out.slice(raw, start, i);
  }
}

// The chunks needed to draw the pixels.
const PNG_KEPT = new Set([
  'IHDR', 'PLTE', 'IDAT', 'IEND',
  'tRNS', 'gAMA', 'cHRM', 'sRGB', 'sBIT', 'pHYs', 'bKGD',
  'cICP', 'mDCV', 'cLLI',
]);
// The APNG chunks.
const PNG_ANIMATION = new Set(['acTL', 'fcTL', 'fdAT']);
const PNG_SIGNATURE = '\x89PNG\r\n\x1a\n';

function stripPNG(raw) {
  if (!startsWith(raw, 0, raw.length, PNG_SIGNATURE)) return MALFORMED;
  const out = new Writer(raw.length);
  out.slice(raw, 0, 8);
  let sawData = false;
  for (let i = 8, first = true; ; first = false) {
    if (i + 8 > raw.length) return MALFORMED;
    const length = readU32(raw, i, false);
    const kind = ascii(raw, i + 4, i + 8);
    if (length > 0x7fffffff || raw.length - i < 12 + length) return MALFORMED;
    if (first !== (kind === 'IHDR') || (kind === 'IHDR' && length !== 13)) return MALFORMED;
    if (PNG_ANIMATION.has(kind)) return ANIMATED;
    const end = i + 12 + length;
    if (PNG_KEPT.has(kind)) out.slice(raw, i, end);
    if (kind === 'IDAT') sawData = true;
    if (kind === 'IEND') return length !== 0 || !sawData ? MALFORMED : out.done();
    i = end;
  }
}

// The chunks needed to draw the pixels.
const WEBP_KEPT = new Set(['VP8 ', 'VP8L', 'VP8X', 'ALPH']);
// The VP8X flags (byte 0 of its payload) that are cleared or refused.
const WEBP_FLAG_ICC = 0x20;
const WEBP_FLAG_EXIF = 0x08;
const WEBP_FLAG_XMP = 0x04;
const WEBP_FLAG_ANIMATION = 0x02;

function stripWebP(raw) {
  if (raw.length < 12 || ascii(raw, 0, 4) !== 'RIFF' || ascii(raw, 8, 12) !== 'WEBP') return MALFORMED;
  const riffSize = readU32(raw, 4, true);
  if (riffSize < 4 || riffSize > raw.length - 8) return MALFORMED;
  const end = 8 + riffSize;
  const out = new Writer(end);
  out.slice(raw, 0, 12);
  let sawImage = false;
  for (let i = 12; i < end;) {
    if (i + 8 > end) return MALFORMED;
    const kind = ascii(raw, i, i + 4);
    const size = readU32(raw, i + 4, true);
    const next = i + 8 + size + (size % 2); // an odd-sized chunk is padded
    if (next > end) return MALFORMED;
    if (kind === 'ANIM' || kind === 'ANMF') return ANIMATED;
    if (kind === 'VP8X') {
      if (size < 10) return MALFORMED;
      if (raw[i + 8] & WEBP_FLAG_ANIMATION) return ANIMATED;
    }
    if (kind === 'VP8 ' || kind === 'VP8L') sawImage = true;
    if (WEBP_KEPT.has(kind)) {
      const start = out.length;
      out.slice(raw, i, next);
      if (kind === 'VP8X') out.buf[start + 8] &= ~(WEBP_FLAG_ICC | WEBP_FLAG_EXIF | WEBP_FLAG_XMP);
    }
    i = next;
  }
  if (!sawImage) return MALFORMED;
  const result = out.done();
  const riff = result.length - 8;
  result[4] = riff & 0xff;
  result[5] = (riff >>> 8) & 0xff;
  result[6] = (riff >>> 16) & 0xff;
  result[7] = (riff >>> 24) & 0xff;
  return result;
}

// --- bytes ------------------------------------------------------------------

class Writer {
  constructor(capacity) { this.buf = new Uint8Array(capacity); this.length = 0; }
  bytes(list) { for (const b of list) this.buf[this.length++] = b; }
  slice(src, start, end) { this.buf.set(src.subarray(start, end), this.length); this.length += end - start; }
  done() { return this.buf.slice(0, this.length); }
}

function startsWith(b, start, end, text) {
  if (end - start < text.length) return false;
  for (let k = 0; k < text.length; k++) if (b[start + k] !== text.charCodeAt(k)) return false;
  return true;
}

function ascii(b, start, end) { return String.fromCharCode(...b.subarray(start, end)); }

function readU32(b, at, little) {
  return little
    ? (b[at] | (b[at + 1] << 8) | (b[at + 2] << 16) | (b[at + 3] << 24)) >>> 0
    : ((b[at] << 24) | (b[at + 1] << 16) | (b[at + 2] << 8) | b[at + 3]) >>> 0;
}
