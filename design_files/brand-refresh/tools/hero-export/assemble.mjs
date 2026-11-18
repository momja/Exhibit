// Assembles the composite and phone crop from the sky, clouds and hill masters.
//
// The delivered art was produced with Pillow (Image.alpha_composite and
// Image.resize with Lanczos). libvips differs from it by up to 2/255 when
// compositing and up to 70/255 when upscaling, so the two operations are
// reproduced here exactly, on 8-bit raw RGBA buffers, to keep the committed
// derivatives byte-for-byte what they were.
import sharp from 'sharp';

export const SKY_EXTENSION = 832; // sky pixels above the 1440px-tall composite
export const COMPOSITE = { width: 2560, height: 1440 };
export const PHONE_CROP = { left: 1540, top: 192, width: 960, height: 2080 };
export const PHONE_SIZE = { width: 1170, height: 2535 };

export async function readRGBA(path) {
  const { data, info } = await sharp(path).ensureAlpha().raw().toBuffer({ resolveWithObject: true });
  return { data, width: info.width, height: info.height };
}

// Pillow's AlphaComposite.c, source over, 7 fractional bits.
const PRECISION = 7;
const div255 = (a) => ((a >> 8) + a) >> 8;

export function alphaCompositeInPlace(dst, src, left, top) {
  const stride = dst.width * 4;
  for (let y = 0; y < src.height; y++) {
    for (let x = 0; x < src.width; x++) {
      const s = (y * src.width + x) * 4;
      const sa = src.data[s + 3];
      if (sa === 0) continue;
      const d = ((y + top) * dst.width + x + left) * 4;
      const da = dst.data[d + 3];
      const blend = da * (255 - sa);
      const outa255 = sa * 255 + blend;
      const coef1 = Math.floor((sa * 255 * 255 * (1 << PRECISION)) / outa255);
      const coef2 = 255 * (1 << PRECISION) - coef1;
      for (let c = 0; c < 3; c++) {
        const t = src.data[s + c] * coef1 + dst.data[d + c] * coef2;
        dst.data[d + c] = div255(t + (0x80 << PRECISION)) >> PRECISION;
      }
      dst.data[d + 3] = div255(outa255 + 0x80);
    }
  }
}

// Pillow's Resample.c: Lanczos, separable, horizontal pass first, 22-bit
// fixed-point coefficients, 8-bit intermediate.
const FIXED_BITS = 22;
const sinc = (x) => (x === 0 ? 1 : Math.sin(x * Math.PI) / (x * Math.PI));
const lanczos = (x) => (x > -3 && x < 3 ? sinc(x) * sinc(x / 3) : 0);

function coefficients(inSize, outSize) {
  const scale = inSize / outSize;
  const filterScale = Math.max(scale, 1);
  const support = 3 * filterScale;
  const rows = [];
  for (let out = 0; out < outSize; out++) {
    const center = (out + 0.5) * scale;
    const min = Math.max(Math.trunc(center - support + 0.5), 0);
    const max = Math.min(Math.trunc(center + support + 0.5), inSize);
    const weights = [];
    let total = 0;
    for (let x = min; x < max; x++) {
      const w = lanczos((x - center + 0.5) / filterScale);
      weights.push(w);
      total += w;
    }
    rows.push({
      min,
      weights: weights.map((w) => {
        const k = total === 0 ? w : w / total;
        return Math.trunc(k * (1 << FIXED_BITS) + (k < 0 ? -0.5 : 0.5));
      }),
    });
  }
  return rows;
}

const clip8 = (v) => {
  const n = v >> FIXED_BITS;
  return n < 0 ? 0 : n > 255 ? 255 : n;
};

export function resizeLanczos(image, width, height) {
  const channels = 4;
  const horizontal = coefficients(image.width, width);
  const vertical = coefficients(image.height, height);
  const middle = Buffer.alloc(width * image.height * channels);
  for (let y = 0; y < image.height; y++) {
    for (let x = 0; x < width; x++) {
      const { min, weights } = horizontal[x];
      for (let c = 0; c < channels; c++) {
        let sum = 1 << (FIXED_BITS - 1);
        for (let i = 0; i < weights.length; i++) sum += image.data[(y * image.width + min + i) * channels + c] * weights[i];
        middle[(y * width + x) * channels + c] = clip8(sum);
      }
    }
  }
  const out = Buffer.alloc(width * height * channels);
  for (let y = 0; y < height; y++) {
    const { min, weights } = vertical[y];
    for (let x = 0; x < width; x++) {
      for (let c = 0; c < channels; c++) {
        let sum = 1 << (FIXED_BITS - 1);
        for (let i = 0; i < weights.length; i++) sum += middle[((min + i) * width + x) * channels + c] * weights[i];
        out[(y * width + x) * channels + c] = clip8(sum);
      }
    }
  }
  return { data: out, width, height };
}

function crop(image, { left, top, width, height }) {
  const out = Buffer.alloc(width * height * 4);
  for (let y = 0; y < height; y++) {
    image.data.copy(out, y * width * 4, ((y + top) * image.width + left) * 4, ((y + top) * image.width + left + width) * 4);
  }
  return { data: out, width, height };
}

// masters: { sky, clouds, hill } as RGBA buffers from readRGBA.
export function assemble({ sky, clouds, hill }) {
  const composite = crop(sky, { left: 0, top: SKY_EXTENSION, ...COMPOSITE });
  alphaCompositeInPlace(composite, clouds, 0, 0);
  alphaCompositeInPlace(composite, hill, 0, 0);
  const extended = { data: Buffer.from(sky.data), width: sky.width, height: sky.height };
  alphaCompositeInPlace(extended, clouds, 0, SKY_EXTENSION);
  alphaCompositeInPlace(extended, hill, 0, SKY_EXTENSION);
  const phone = resizeLanczos(crop(extended, PHONE_CROP), PHONE_SIZE.width, PHONE_SIZE.height);
  return { composite, phone };
}
