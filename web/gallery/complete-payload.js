/* The host page's check of a complete() payload (Exh-v81w).
 *
 * The artifact frame's complete() call is checked here before anything is
 * sent, by the server's own rules for its messages and images
 * (prepareComplete and parseImage in internal/inference/kinds.go), and every
 * image is stripped of its metadata (image-metadata.js). A payload either
 * comes back ready to send, with each image part carrying stripped bytes as
 * plain base64, or is refused here with the words the server would use. So
 * no image the server would refuse, and no metadata, ever leaves the device
 * through this page.
 *
 * The payload is read as the server will read it: through JSON, with lone
 * surrogates as U+FFFD, keys spelled exactly (the server refuses "Messages"
 * or a second "data"), whitespace and case as Go's decoder sees them, and
 * the image bounds counted on the bytes as given. What concerns only the
 * text (its total size, maxTokens' value, the schema) is left to the server:
 * nothing of an image rides on it. The shared corpus in
 * internal/inference/testdata/imagemeta holds both sides to the same words.
 */
import { stripImageMetadata, ImageStructureError } from './image-metadata.js';

// The bounds, as the server has them (internal/inference/kinds.go).
const MAX_MESSAGES = 40;
const MAX_PARTS = 16;
const MAX_IMAGES = 4;
const MAX_IMAGE_BYTES = 2 << 20;
const MAX_IMAGE_TOTAL_BYTES = 4 << 20;
const ACCEPTED = new Set(['image/png', 'image/jpeg', 'image/webp']);

// The server's refusals, word for word.
export const MESSAGES = {
  payloadShape: 'complete() takes { messages, system?, schema?, maxTokens? }.',
  messageShape: 'Each message is { role, content }.',
  promptAndMessages: 'Pass either prompt or messages, not both.',
  messageCount: `complete() needs between 1 and ${MAX_MESSAGES} messages.`,
  role: 'Each message\'s role must be "user" or "assistant".',
  lastUser: 'The last message must be from the user.',
  emptyText: "A message's content must be non-empty text.",
  contentShape: "A message's content must be text, or a list of { type: 'text', text } and { type: 'image', mimeType, data } parts.",
  partCount: `A message's content list needs between 1 and ${MAX_PARTS} parts.`,
  textPart: "A text part is { type: 'text', text } with non-empty text.",
  imageRole: 'Only user messages may carry images.',
  imagePart: "An image part is { type: 'image', mimeType, data } with base64 data.",
  partType: 'A content part\'s type must be "text" or "image".',
  imageCount: `complete() takes at most ${MAX_IMAGES} images.`,
  imageTotal: `complete() images are limited to ${MAX_IMAGE_TOTAL_BYTES >> 20} MB in total. Downscale them in a canvas before sending.`,
  dataURL: "An image's data must be base64, or a base64 data: URL.",
  dataURLType: "An image's mimeType does not match its data: URL.",
  imageType: 'Images must be PNG, JPEG or WebP (mimeType image/png, image/jpeg or image/webp).',
  imageSize: `Each image is limited to ${MAX_IMAGE_BYTES >> 20} MB. Downscale it in a canvas before sending.`,
  imageBase64: "An image's data must be non-empty base64.",
  imageAnimated: 'Animated images are not supported. Send a still image.',
  imageMislabel: mimeType => "An image's data is not the " + mimeType + ' its mimeType says.',
  imageMalformed: mimeType => "An image's data is not a well-formed " + mimeType + ' file.',
};

class Refusal extends Error {}
const refuse = message => { throw new Refusal(message); };

/**
 * Checks a complete() payload and strips its images. Returns
 * { ok: true, payload } with the payload to send, or { ok: false, message }
 * with the server's words for the first rule it breaks.
 */
export function prepareCompletePayload(payload) {
  try {
    return { ok: true, payload: prepare(asSent(payload)) };
  } catch (e) {
    if (e instanceof Refusal) return { ok: false, message: e.message };
    throw e;
  }
}

// The payload as the server will decode it.
function asSent(payload) {
  let json;
  try { json = JSON.stringify(payload); } catch { refuse(MESSAGES.payloadShape); }
  if (json === undefined) refuse(MESSAGES.payloadShape);
  return JSON.parse(json, (_, v) => (typeof v === 'string' ? wellFormed(v) : v));
}

function prepare(payload) {
  const top = fields(payload, ['messages', 'prompt', 'system', 'schema', 'maxTokens']) ?? refuse(MESSAGES.payloadShape);
  if (!nullOr(top.messages, Array.isArray) || !nullOr(top.prompt, isString) ||
      !nullOr(top.system, isString) || !nullOr(top.maxTokens, isGoInt)) refuse(MESSAGES.payloadShape);
  let messages = (top.messages ?? []).map(m => {
    const f = fields(m, ['role', 'content']);
    if (!f || !nullOr(f.role, isString)) refuse(MESSAGES.messageShape);
    return { role: f.role ?? '', content: f.content };
  });
  if (top.prompt != null) {
    if (messages.length > 0) refuse(MESSAGES.promptAndMessages);
    messages = [{ role: 'user', content: top.prompt }];
  }
  if (messages.length === 0 || messages.length > MAX_MESSAGES) refuse(MESSAGES.messageCount);
  const budget = { count: 0, bytes: 0 };
  const contents = messages.map((m, i) => {
    if (m.role !== 'user' && m.role !== 'assistant') refuse(MESSAGES.role);
    if (i === messages.length - 1 && m.role !== 'user') refuse(MESSAGES.lastUser);
    return content(m.role, m.content, budget);
  });
  if (top.prompt != null) return payload;
  return { ...payload, messages: payload.messages.map((m, i) => (contents[i] === m.content ? m : { ...m, content: contents[i] })) };
}

// One message's content as it will be sent: as given, or a parts list whose
// images carry their stripped bytes.
function content(role, value, budget) {
  if (value === null || typeof value === 'string') {
    if (isBlank(value ?? '')) refuse(MESSAGES.emptyText);
    return value;
  }
  if (!Array.isArray(value)) refuse(MESSAGES.contentShape);
  const wire = value.map((part, k) => {
    if (k === MAX_PARTS) refuse(MESSAGES.partCount);
    const f = fields(part, ['type', 'text', 'mimeType', 'data']);
    if (!f || !['type', 'text', 'mimeType', 'data'].every(name => nullOr(f[name], isString))) refuse(MESSAGES.contentShape);
    return f;
  });
  if (wire.length === 0) refuse(MESSAGES.partCount);
  return wire.map((w, k) => {
    const type = w.type ?? '';
    if (type === 'text') {
      if (w.text == null || isBlank(w.text) || w.data != null || (w.mimeType ?? '') !== '') refuse(MESSAGES.textPart);
      return value[k];
    }
    if (type !== 'image') refuse(MESSAGES.partType);
    if (role !== 'user') refuse(MESSAGES.imageRole);
    if (w.text != null || w.data == null) refuse(MESSAGES.imagePart);
    if (++budget.count > MAX_IMAGES) refuse(MESSAGES.imageCount);
    const image = parseImage(w.mimeType ?? '', w.data);
    if ((budget.bytes += image.size) > MAX_IMAGE_TOTAL_BYTES) refuse(MESSAGES.imageTotal);
    return { ...value[k], mimeType: image.mimeType, data: image.data };
  });
}

// parseImage in kinds.go: the same checks in the same order, then the strip.
function parseImage(givenType, data) {
  let mimeType = asciiLower(goTrim(givenType));
  if (data.startsWith('data:')) {
    const comma = data.indexOf(',');
    const header = asciiLower(comma < 0 ? data.slice(5) : data.slice(5, comma));
    if (comma < 0 || !header.endsWith(';base64')) refuse(MESSAGES.dataURL);
    const urlType = header.slice(0, -';base64'.length);
    if (mimeType === '') mimeType = urlType;
    else if (urlType !== mimeType) refuse(MESSAGES.dataURLType);
    data = data.slice(comma + 1);
  }
  if (!ACCEPTED.has(mimeType)) refuse(MESSAGES.imageType);
  // The decoder skips line breaks, so they do not count toward the size.
  data = data.replace(/[\r\n]/g, '');
  // Go's base64.DecodedLen, over the string's length in UTF-8 bytes.
  if (Math.floor(utf8Length(data) / 4) * 3 > MAX_IMAGE_BYTES + 2) refuse(MESSAGES.imageSize);
  if (data === '' || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(data)) refuse(MESSAGES.imageBase64);
  const raw = fromBase64(data);
  if (raw.length > MAX_IMAGE_BYTES) refuse(MESSAGES.imageSize);
  if (sniff(raw) !== mimeType) refuse(MESSAGES.imageMislabel(mimeType));
  let stripped;
  try {
    stripped = stripImageMetadata(raw, mimeType);
  } catch (e) {
    if (!(e instanceof ImageStructureError)) throw e;
    refuse(e.reason === 'animated' ? MESSAGES.imageAnimated : MESSAGES.imageMalformed(mimeType));
  }
  return { mimeType, data: toBase64(stripped), size: raw.length };
}

// The three signatures http.DetectContentType knows these types by. No
// signature it checks earlier can start with any of their first bytes.
function sniff(b) {
  const at = (start, text) => b.length >= start + text.length && [...text].every((c, k) => b[start + k] === c.charCodeAt(0));
  if (at(0, '\x89PNG\r\n\x1a\n')) return 'image/png';
  if (at(0, '\xff\xd8\xff')) return 'image/jpeg';
  if (at(0, 'RIFF') && at(8, 'WEBPVP')) return 'image/webp';
  return '';
}

// --- reading JSON as Go does ----------------------------------------------

// An object's fields when every key is among names, else null. JSON.parse
// already keeps only the last of two keys spelled the same; two spelled
// differently are refused here, as on the server, because only one of them
// can be among names.
function fields(value, names) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return null;
  return Object.keys(value).every(k => names.includes(k)) ? value : null;
}

const isString = v => typeof v === 'string';
const nullOr = (v, check) => v === undefined || v === null || check(v);
// What Go decodes into an int: an integer literal within 64 bits.
const isGoInt = v => Number.isInteger(v) && v >= -(2 ** 63) && v < 2 ** 63;

// strings.TrimSpace's and unicode.IsSpace's whitespace, which is not JS's:
// Go counts U+0085 and not U+FEFF.
const GO_SPACE = '\\t\\n\\v\\f\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000';
const goTrim = s => s.replace(new RegExp(`^[${GO_SPACE}]+|[${GO_SPACE}]+$`, 'g'), '');
const isBlank = s => goTrim(s) === '';
const asciiLower = s => s.replace(/[A-Z]/g, c => c.toLowerCase());

function wellFormed(s) {
  return typeof s.toWellFormed === 'function'
    ? s.toWellFormed()
    : s.replace(/[\ud800-\udbff](?![\udc00-\udfff])|(?<![\ud800-\udbff])[\udc00-\udfff]/g, '�');
}

function utf8Length(s) {
  if (!/[^\x00-\x7f]/.test(s)) return s.length;
  let n = 0;
  for (const c of s) {
    const cp = c.codePointAt(0);
    n += cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4;
  }
  return n;
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
