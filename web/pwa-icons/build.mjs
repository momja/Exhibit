// Rasterizes the Exhibit logo (internal/api/logo.svg — the exact SVG
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

// The logo's own viewBox (internal/api/logo.svg), used to keep the maskable
// variant's aspect ratio in sync with the artwork without hand-copying it.
const VIEWBOX_WIDTH = 40.316795;
const VIEWBOX_HEIGHT = 49.983387;

const BACKGROUND = "#ffffff"; // matches the logo's own white card + the manifest's background_color

// Plain (non-maskable) icons: the logo letterboxed onto a square canvas at
// its native aspect ratio, same look as the existing favicon.
async function renderIcon(size, fileName) {
  await sharp(svg)
    .resize(size, size, { fit: "contain", background: BACKGROUND })
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

await renderIcon(180, "apple-touch-icon-180.png");
await renderIcon(192, "icon-192.png");
await renderIcon(512, "icon-512.png");
await renderMaskableIcon(512, "icon-512-maskable.png");

console.log("Rendered PWA icons ->", path.relative(process.cwd(), outDir));
