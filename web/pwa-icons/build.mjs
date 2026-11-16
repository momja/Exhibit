// Rasterizes the flat Exhibit mark (internal/api/logo.svg — the exact SVG
// internal/api/logo.go embeds and inlines as the favicon/header mark) into
// the PNG sizes the web app manifest and iOS home-screen tags need
// (av-emh4). One source, two consumers: the Go binary and this build step
// read the same file, so the vector artwork and the generated icons can
// never drift apart.
import sharp from "sharp";
import { readFileSync, mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const svgPath = path.join(here, "../../internal/api/logo.svg");
const outDir = path.join(here, "../../internal/api/assets/icons");

mkdirSync(outDir, { recursive: true });

const svg = readFileSync(svgPath);

// The flat mark's own viewBox, read from the artwork so the maskable variant's
// aspect ratio can never drift from it.
const [, , VIEWBOX_WIDTH, VIEWBOX_HEIGHT] = svg
  .toString()
  .match(/viewBox="([^"]+)"/)[1]
  .split(/\s+/)
  .map(Number);

const BACKGROUND = "#f1e9d6"; // paper: tokens.css --color-ground, and the manifest's background_color/theme_color

// Plain (non-maskable) icons: the flat mark centred on a square canvas at its
// native aspect ratio. The viewBox is tight to the frame, so the mark takes 80%
// of the canvas and leaves a margin of paper around it.
async function renderIcon(size, fileName) {
  const inner = Math.round(size * 0.8);
  const mark = await sharp(svg)
    .resize(inner, inner, { fit: "contain", background: { r: 0, g: 0, b: 0, alpha: 0 } })
    .png()
    .toBuffer();
  await sharp({ create: { width: size, height: size, channels: 4, background: BACKGROUND } })
    .composite([{ input: mark, gravity: "center" }])
    .png()
    .toFile(path.join(outDir, fileName));
}

// Maskable icon: Android's adaptive-icon mask can crop anything outside a
// centered circle whose diameter is `safeFraction` of the canvas
// (web.dev/maskable-icon), so the logo must be scaled down further than a
// plain "contain" fit until its bounding box sits fully inside that circle.
// For a box of width W and height H = aspect*W centered on the canvas, the
// box's corner distance from center is (W/2)*sqrt(1+aspect^2); solving that
// against the safe-zone radius gives the largest W that stays inside it.
async function renderMaskableIcon(size, fileName, safeFraction = 0.8) {
  const aspect = VIEWBOX_HEIGHT / VIEWBOX_WIDTH;
  const radius = (safeFraction * size) / 2;
  const contentWidth = Math.floor((2 * radius) / Math.sqrt(1 + aspect * aspect));
  const contentHeight = Math.floor(contentWidth * aspect);

  const logo = await sharp(svg).resize(contentWidth, contentHeight).png().toBuffer();

  await sharp({
    create: {
      width: size,
      height: size,
      channels: 4,
      background: BACKGROUND,
    },
  })
    .composite([{ input: logo, gravity: "center" }])
    .png()
    .toFile(path.join(outDir, fileName));
}

// The link-preview image every share falls back to (av-rclm): when the
// instance runs no snapshot browser, when a snapshot fails, and for every
// link that does not resolve to a live share. The same size as a widget
// snapshot (ImageWidth x ImageHeight in internal/ogsnap), so a preview card
// keeps its shape whichever image it gets; the ground is tokens.css's
// --color-ground (paper), which the logo's cream card reads against.
async function renderPreviewCard(width, height, fileName) {
  const logoHeight = Math.round(height * 0.6);
  const logo = await sharp(svg)
    .resize({ height: logoHeight })
    .png()
    .toBuffer();

  await sharp({
    create: { width, height, channels: 4, background: BACKGROUND },
  })
    .composite([{ input: logo, gravity: "center" }])
    .png()
    .toFile(path.join(outDir, fileName));
}

await renderIcon(180, "apple-touch-icon-180.png");
await renderIcon(192, "icon-192.png");
await renderIcon(512, "icon-512.png");
await renderMaskableIcon(512, "icon-512-maskable.png");
await renderPreviewCard(1056, 528, "og-default.png");

console.log("Rendered PWA icons ->", path.relative(process.cwd(), outDir));
