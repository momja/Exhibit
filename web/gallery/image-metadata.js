/* Image metadata stripping for window.exhibit.ai (Exh-v81w).
 *
 * Every image a tool sends to complete() is rewritten without its metadata
 * before it leaves the device: location, camera, timestamps, comments,
 * editing history. The server strips again as a backstop for clients that
 * bypass this page (internal/inference/imagemeta.go). The two follow the
 * same rules:
 *
 *   - JPEG: walk the segments from SOI. Keep APP0 (JFIF), APP2 only when it
 *     is an ICC_PROFILE, APP14 (Adobe, which decides the colour transform)
 *     and every non-APP segment (tables, frame and scan headers, restart
 *     intervals). Drop every other APPn (APP1 is EXIF and XMP; APP2 also
 *     carries MPF) and COM. Scan data is copied verbatim up to the next
 *     marker, and walking resumes there, so a progressive file's later
 *     segments follow the same rules. Output ends at EOI: anything after it
 *     (a second picture, a motion photo's video, a vendor trailer) is
 *     dropped.
 *   - PNG: keep IHDR, PLTE, IDAT, IEND, tRNS, gAMA, cHRM, sRGB, iCCP, sBIT,
 *     pHYs, bKGD, and acTL/fcTL/fdAT for APNG; drop every other chunk
 *     (eXIf, tEXt, zTXt, iTXt, tIME and anything unknown). Kept chunks are
 *     copied verbatim, CRC included. Output ends at IEND.
 *   - WebP: keep VP8, VP8L, VP8X, ALPH, ANIM, ANMF and ICCP; drop every
 *     other chunk (EXIF, XMP and anything unknown). VP8X's EXIF (0x08) and
 *     XMP (0x04) flags are cleared, and the RIFF size is rewritten. Output
 *     ends at the RIFF's end.
 *
 * Orientation goes with EXIF, so a phone photo that relied on it may reach
 * the model rotated. Pixels are never decoded or re-encoded.
 *
 * A file that cannot be walked cleanly to its end marker throws
 * MalformedImageError; it is never sent as it came.
 */

export class MalformedImageError extends Error {
  constructor(mimeType) {
    super("An image's data is not a well-formed " + mimeType + ' file.');
    this.name = 'MalformedImageError';
  }
}

// What complete() accepts, and the per-image bound, as the server has them
// (internal/inference/kinds.go). An image outside them is left for the
// server to refuse in its own words.
const ACCEPTED = new Set(['image/png', 'image/jpeg', 'image/webp']);
const MAX_IMAGE_BYTES = 2 << 20;

/** Returns bytes (a Uint8Array) without metadata, or throws MalformedImageError. */
export function stripImageMetadata(bytes, mimeType) {
  const strip = { 'image/jpeg': stripJPEG, 'image/png': stripPNG, 'image/webp': stripWebP }[mimeType];
  const out = strip && strip(bytes);
  if (!out) throw new MalformedImageError(mimeType);
  return out;
}

function keepJPEGSegment(marker, bytes, start, end) {
  if (marker === 0xfe) return false; // COM
  if (marker === 0xe0 || marker === 0xee) return true; // APP0 JFIF, APP14 Adobe
  if (marker === 0xe2) return startsWith(bytes, start, end, 'ICC_PROFILE\0'); // not MPF or FlashPix
  return !(marker >= 0xe1 && marker <= 0xef); // every other APPn
}

// Each strip returns null for a file it cannot walk.
function stripJPEG(raw) {
  const n = raw.length;
  if (n < 4 || raw[0] !== 0xff || raw[1] !== 0xd8) return null;
  const out = new Writer(n);
  out.bytes([0xff, 0xd8]);
  let sawScan = false;
  let i = 2;
  for (;;) {
    if (i >= n || raw[i] !== 0xff) return null;
    while (i < n && raw[i] === 0xff) i++; // fill bytes before a marker
    if (i >= n) return null;
    const marker = raw[i++];
    if (marker === 0xd9) { // EOI
      if (!sawScan) return null;
      out.bytes([0xff, 0xd9]);
      return out.done();
    }
    if (marker === 0x00 || marker === 0xd8) return null;
    if (marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) { // TEM, RSTn
      out.bytes([0xff, marker]);
      continue;
    }
    if (i + 2 > n) return null;
    const length = (raw[i] << 8) | raw[i + 1];
    if (length < 2 || i + length > n) return null;
    if (keepJPEGSegment(marker, raw, i + 2, i + length)) {
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
      if (i + 1 >= n) return null;
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
const PNG_KEPT = new Set(['IHDR', 'PLTE', 'IDAT', 'IEND', 'tRNS', 'gAMA', 'cHRM', 'sRGB', 'iCCP', 'sBIT', 'pHYs', 'bKGD', 'acTL', 'fcTL', 'fdAT']);
const PNG_SIGNATURE = '\x89PNG\r\n\x1a\n';

function stripPNG(raw) {
  if (!startsWith(raw, 0, raw.length, PNG_SIGNATURE)) return null;
  const out = new Writer(raw.length);
  out.slice(raw, 0, 8);
  for (let i = 8, first = true; ; first = false) {
    if (i + 8 > raw.length) return null;
    const length = readU32(raw, i, false);
    const kind = ascii(raw, i + 4, i + 8);
    if (length > 0x7fffffff || raw.length - i < 12 + length) return null;
    if (first !== (kind === 'IHDR')) return null;
    const end = i + 12 + length;
    if (PNG_KEPT.has(kind)) out.slice(raw, i, end);
    if (kind === 'IEND') return out.done();
    i = end;
  }
}

// The chunks needed to draw the pixels.
const WEBP_KEPT = new Set(['VP8 ', 'VP8L', 'VP8X', 'ALPH', 'ANIM', 'ANMF', 'ICCP']);

function stripWebP(raw) {
  if (raw.length < 12 || ascii(raw, 0, 4) !== 'RIFF' || ascii(raw, 8, 12) !== 'WEBP') return null;
  const riffSize = readU32(raw, 4, true);
  if (riffSize < 4 || riffSize > raw.length - 8) return null;
  const end = 8 + riffSize;
  const out = new Writer(end);
  out.slice(raw, 0, 12);
  for (let i = 12; i < end;) {
    if (i + 8 > end) return null;
    const kind = ascii(raw, i, i + 4);
    const size = readU32(raw, i + 4, true);
    const next = i + 8 + size + (size & 1);
    if (next > end) return null;
    if (WEBP_KEPT.has(kind)) {
      const start = out.length;
      out.slice(raw, i, next);
      if (kind === 'VP8X') {
        if (size < 10) return null;
        out.buf[start + 8] &= ~(0x08 | 0x04); // EXIF, XMP
      }
    }
    i = next;
  }
  const result = out.done();
  const riff = result.length - 8;
  result[4] = riff & 0xff;
  result[5] = (riff >>> 8) & 0xff;
  result[6] = (riff >>> 16) & 0xff;
  result[7] = (riff >>> 24) & 0xff;
  return result;
}

// --- complete() payloads ----------------------------------------------------

/**
 * Returns a copy of a complete() payload whose image parts carry stripped
 * bytes as plain base64 (a data: URL's type becomes mimeType when none was
 * given). Nothing else changes, and a part this does not recognize as an
 * accepted image (another type, data that is not strict base64, a type the
 * bytes are not, an image over the bound) is left as it was, for the server
 * to refuse in its own words. Throws MalformedImageError for an accepted
 * image whose structure is broken.
 */
export function stripPayloadImages(payload) {
  if (!payload || typeof payload !== 'object' || !Array.isArray(payload.messages)) return payload;
  let changed = false;
  const messages = payload.messages.map(m => {
    if (!m || typeof m !== 'object' || !Array.isArray(m.content)) return m;
    let partChanged = false;
    const content = m.content.map(part => {
      const next = strippedPart(part);
      if (next !== part) partChanged = true;
      return next;
    });
    if (!partChanged) return m;
    changed = true;
    return { ...m, content };
  });
  return changed ? { ...payload, messages } : payload;
}

function strippedPart(part) {
  if (!part || typeof part !== 'object' || part.type !== 'image' || typeof part.data !== 'string') return part;
  // JSON null reads as no mimeType on the server, as does a missing one.
  if (part.mimeType != null && typeof part.mimeType !== 'string') return part;
  const given = (part.mimeType ?? '').trim().toLowerCase();
  let mimeType = given;
  let data = part.data;
  if (data.startsWith('data:')) {
    const comma = data.indexOf(',');
    const header = data.slice(5, comma).toLowerCase();
    if (comma < 0 || !header.endsWith(';base64')) return part;
    const urlType = header.slice(0, -';base64'.length);
    if (mimeType === '') mimeType = urlType;
    else if (urlType !== mimeType) return part;
    data = data.slice(comma + 1);
  }
  if (!ACCEPTED.has(mimeType)) return part;
  data = data.replace(/[\r\n]/g, '');
  if (data === '' || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(data)) return part;
  if ((data.length / 4) * 3 - (data.endsWith('==') ? 2 : data.endsWith('=') ? 1 : 0) > MAX_IMAGE_BYTES) return part;
  const bytes = fromBase64(data);
  if (sniff(bytes) !== mimeType) return part;
  const stripped = stripImageMetadata(bytes, mimeType);
  // The data is no longer a data: URL, so a type it carried becomes mimeType.
  return { ...part, mimeType: given === '' ? mimeType : part.mimeType, data: toBase64(stripped) };
}

// The three signatures http.DetectContentType knows these types by.
function sniff(b) {
  if (startsWith(b, 0, b.length, PNG_SIGNATURE)) return 'image/png';
  if (b.length >= 3 && b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return 'image/jpeg';
  if (startsWith(b, 0, b.length, 'RIFF') && startsWith(b, 8, b.length, 'WEBPVP')) return 'image/webp';
  return '';
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

function fromBase64(data) {
  const binary = atob(data);
  const out = new Uint8Array(binary.length);
  for (let k = 0; k < binary.length; k++) out[k] = binary.charCodeAt(k);
  return out;
}

function toBase64(bytes) {
  let binary = '';
  for (let k = 0; k < bytes.length; k += 0x8000) binary += String.fromCharCode(...bytes.subarray(k, k + 0x8000));
  return btoa(binary);
}
