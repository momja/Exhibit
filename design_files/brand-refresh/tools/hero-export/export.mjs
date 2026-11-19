// Encodes the committed responsive hero derivatives (web/hero-art/images) from the
// lossless masters in design_files/brand-refresh/production.
//
//   npm run export   write the derivatives and manifest.json
//   npm run check    re-encode in memory; fail if any committed byte would change
//
// Only sky, clouds and hill are masters. The composite and the phone crop are
// assembled here (assemble.mjs). Byte equality needs the same Linux architecture
// and the sharp version pinned in package-lock.json: libvips output is not portable.
import sharp from 'sharp';
import { mkdirSync, readFileSync, readdirSync, writeFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { assemble, readRGBA } from './assemble.mjs';

const check = process.argv.includes('--check');
const masters = new URL('../../production/', import.meta.url);
const output = new URL('../../../../web/hero-art/images/', import.meta.url);

const widths = { composite: [960, 1600, 2560], sky: [960, 1600, 2560], clouds: [960, 1600, 2560], hill: [960, 1600, 2560], phone: [750, 1170] };
const formats = ['webp', 'avif'];

// Byte ceilings at each kind's largest width. "layers" is sky + clouds + hill,
// the set the descent animation downloads in place of the composite's first
// paint (the page needs both; they are not alternatives).
const BUDGET = {
  composite: { webp: 265_000, avif: 185_000 },
  phone: { webp: 200_000, avif: 135_000 },
  layers: { webp: 550_000, avif: 430_000 },
};

// Limit codec workers and only batch one kind of image at a time.
sharp.concurrency(1);
sharp.cache(false);

async function loadSources(mode) {
  const path = (kind) => fileURLToPath(new URL(`${mode}-${kind}.webp`, masters));
  const [sky, clouds, hill] = await Promise.all(['sky', 'clouds', 'hill'].map((kind) => readRGBA(path(kind))));
  // Decode explicitly: libwebp shrinks on load, which would resample differently
  // from the PNG masters these derivatives were first encoded from.
  const decoded = async (kind) => {
    const { data, info } = await sharp(path(kind)).raw().toBuffer({ resolveWithObject: true });
    return { input: data, raw: { width: info.width, height: info.height, channels: info.channels } };
  };
  const { composite, phone } = assemble({ sky, clouds, hill });
  const opaque = (image) => sharp(image.data, { raw: { width: image.width, height: image.height, channels: 4 } }).removeAlpha().raw().toBuffer({ resolveWithObject: true })
    .then(({ data, info }) => ({ input: data, raw: { width: info.width, height: info.height, channels: 3 } }));
  return {
    composite: await opaque(composite),
    phone: await opaque(phone),
    sky: await decoded('sky'),
    clouds: await decoded('clouds'),
    hill: await decoded('hill'),
  };
}

async function encode(source, width, format) {
  const pipeline = sharp(source.input, { raw: source.raw }).resize({ width, kernel: 'lanczos3' });
  // Limit the WebP fallback's cost on the two translucent layers.
  const webp = source.raw.channels === 4 ? { quality: 70, alphaQuality: 50 } : { quality: 78 };
  const { data, info } = await (format === 'avif'
    ? pipeline.avif({ quality: 48, effort: 4, chromaSubsampling: '4:4:4' })
    : pipeline.webp({ ...webp, effort: 6 })).toBuffer({ resolveWithObject: true });
  return { data, width: info.width, height: info.height };
}

const manifest = {};
const encoded = {};
const largest = {};
for (const mode of ['day', 'dusk']) {
  const sources = await loadSources(mode);
  for (const kind of Object.keys(widths)) {
    const jobs = widths[kind].flatMap((width) => formats.map((format) => ({ width, format })));
    const images = await Promise.all(jobs.map(async ({ width, format }) => ({
      width, format, image: await encode(sources[kind], width, format),
    })));
    for (const { width, format, image } of images) {
      const name = `${mode}-${kind}-${width}.${format}`;
      encoded[name] = image.data;
      manifest[name] = { width: image.width, height: image.height, bytes: image.data.length };
      if (width === widths[kind].at(-1)) largest[`${mode}/${kind}/${format}`] = image.data.length;
    }
    console.log(`Encoded ${mode} ${kind}`);
  }
}

const failures = [];
for (const mode of ['day', 'dusk']) {
  for (const format of formats) {
    const layers = ['sky', 'clouds', 'hill'].reduce((sum, kind) => sum + largest[`${mode}/${kind}/${format}`], 0);
    const sizes = { composite: largest[`${mode}/composite/${format}`], phone: largest[`${mode}/phone/${format}`], layers };
    for (const [kind, bytes] of Object.entries(sizes)) {
      if (bytes > BUDGET[kind][format]) failures.push(`${mode} ${kind} ${format}: ${bytes} bytes exceeds the ${BUDGET[kind][format]} budget`);
    }
  }
}

const manifestText = JSON.stringify(manifest, null, 2) + '\n';
if (check) {
  const expected = new Set([...Object.keys(manifest), 'manifest.json']);
  for (const extra of readdirSync(output)) if (!expected.has(extra)) failures.push(`${extra}: committed but not produced by the export`);
  for (const [name, data] of Object.entries(encoded)) {
    const file = new URL(name, output);
    if (!existsSync(file)) failures.push(`${name}: missing`);
    else if (Buffer.compare(readFileSync(file), data) !== 0) failures.push(`${name}: committed bytes differ from a fresh export`);
  }
  if (!existsSync(new URL('manifest.json', output)) || readFileSync(new URL('manifest.json', output), 'utf8') !== manifestText) failures.push('manifest.json: differs from a fresh export');
  if (failures.length) {
    console.error(failures.join('\n'));
    process.exit(1);
  }
  console.log(`Checked ${Object.keys(encoded).length} derivatives: byte-identical`);
} else {
  if (failures.length) {
    console.error(failures.join('\n'));
    process.exit(1);
  }
  mkdirSync(output, { recursive: true });
  for (const [name, data] of Object.entries(encoded)) writeFileSync(new URL(name, output), data);
  writeFileSync(new URL('manifest.json', output), manifestText);
  console.log(`Exported ${Object.keys(encoded).length} responsive images`);
}
