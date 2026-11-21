// Bakes a "boiling" hand-inked copy of a Phosphor icon (Exh-s8yu): three
// slightly different drawings of the same outline, the classic hand-drawn line
// boil. The frames sit side by side in one SVG, a filmstrip one cell (256
// units) apart, and the root's viewBox shows one cell. CSS slides the strip a
// whole cell at a time (web/gallery/agent.css, `.boil`).
//
// A strip moved by one animation, rather than three frames each toggled by its
// own, is what makes "exactly one frame shows" true by construction: one
// animation has one value at any instant. Three visibility animations hand
// over at a keyframe offset that cannot be exactly 1/3, and WebKit's timeline
// does land in the gap that leaves, drawing a blank frame.
//
// The frames are baked here, at build time, rather than generated in the
// browser or shipped as raster: an inline vector follows currentColor and dark
// mode and stays crisp at every pixel ratio, and baking keeps the noise code
// and path data out of the page entirely.
//
// Everything is seeded, so the output is a pure function of the source SVG:
// every build writes identical bytes (web/icons/boil.test.mjs holds it to that).
// All distances are in the icon's own 256-unit grid.
import { svgPathProperties } from "svg-path-properties";

const STEP = 2;            // distance between sample points along the outline
const DRIFT = 9;           // how far the hand wanders...
const DRIFT_WAVE = 90;     // ...and how far apart its bends are
const PRESSURE = 3;        // how much an edge swells or thins along its normal
const PRESSURE_WAVE = 45;
const TOLERANCE = 0.3;     // RDP: keep a point only if dropping it moves the line more
export const FRAME_SEEDS = [7, 24, 41];
const CELL = 256;          // one filmstrip cell: the source's viewBox

// The pinned upstream drawing the agent's indicator is baked from.
export const PENCIL_SIMPLE_SOURCE = new URL(
  "./node_modules/@phosphor-icons/core/assets/regular/pencil-simple.svg", import.meta.url);

// A seeded 2D Perlin noise field: the same (x, y) always gives the same value
// in about [-1, 1], and nearby points give nearby values. The permutation is a
// Fisher–Yates shuffle over a small LCG, so it depends on nothing the engine
// is free to vary (a random-comparator sort would).
function noise2D(seed) {
  let s = seed >>> 0;
  const rand = () => (s = (Math.imul(s, 1664525) + 1013904223) >>> 0) / 2 ** 32;
  const perm = [...Array(256).keys()];
  for (let i = perm.length - 1; i > 0; i--) {
    const j = Math.floor(rand() * (i + 1));
    [perm[i], perm[j]] = [perm[j], perm[i]];
  }
  const p = [...perm, ...perm];
  const grad = (h, x, y) => [x + y, -x + y, x - y, -x - y, x, -x, y, -y][h & 7];
  const fade = (t) => t * t * t * (t * (t * 6 - 15) + 10);
  const lerp = (a, b, t) => a + t * (b - a);
  return (x, y) => {
    const X = Math.floor(x) & 255, Y = Math.floor(y) & 255;
    x -= Math.floor(x); y -= Math.floor(y);
    const u = fade(x), v = fade(y), a = p[X] + Y, b = p[X + 1] + Y;
    return lerp(lerp(grad(p[a], x, y), grad(p[b], x - 1, y), u),
                lerp(grad(p[a + 1], x, y - 1), grad(p[b + 1], x - 1, y - 1), u), v);
  };
}

// Evenly spaced points along each contour (one list per subpath).
function sampleContours(d) {
  const subpaths = d.split(/(?=[Mm])/);
  if (subpaths.some((sub) => !sub.startsWith("M"))) {
    // A relative m would be resolved against the previous subpath, which
    // sampling each one alone cannot do.
    throw new Error("boil: every contour must start with an absolute M");
  }
  return subpaths.map((sub) => {
    const path = new svgPathProperties(sub);
    const len = path.getTotalLength();
    const n = Math.max(8, Math.round(len / STEP));
    return Array.from({ length: n }, (_, i) => {
      const { x, y } = path.getPointAtLength((i / n) * len);
      return [x, y];
    });
  });
}

// One frame: drift every point by a smooth field, then push it along its
// outline normal by a second, finer one. Holes wind opposite to outer
// contours, so the same push widens a stroke on one side and narrows it on the
// other — ink pressure rather than a uniform offset.
function inkFrame(contours, seed) {
  const nx = noise2D(seed), ny = noise2D(seed + 101), np = noise2D(seed + 202);
  const f = 1 / DRIFT_WAVE, pf = 1 / PRESSURE_WAVE;
  return contours.map((pts) => pts.map(([x, y], i) => {
    const dx = DRIFT * nx(x * f, y * f);
    const dy = DRIFT * ny(x * f + 40, y * f + 40);
    const a = pts[(i - 2 + pts.length) % pts.length], b = pts[(i + 2) % pts.length];
    const tx = b[0] - a[0], ty = b[1] - a[1], m = Math.hypot(tx, ty) || 1;
    const w = PRESSURE * np(x * pf, y * pf);
    return [x + dx - (ty / m) * w, y + dy + (tx / m) * w];
  }));
}

// Ramer–Douglas–Peucker over an open polyline.
function simplify(pts) {
  if (pts.length < 3) return pts;
  const a = pts[0], b = pts[pts.length - 1];
  const len = Math.hypot(b[0] - a[0], b[1] - a[1]) || 1;
  let worst = 0, at = 0;
  for (let i = 1; i < pts.length - 1; i++) {
    const dist = Math.abs((b[0] - a[0]) * (a[1] - pts[i][1]) - (a[0] - pts[i][0]) * (b[1] - a[1])) / len;
    if (dist > worst) { worst = dist; at = i; }
  }
  if (worst <= TOLERANCE) return [a, b];
  return [...simplify(pts.slice(0, at + 1)).slice(0, -1), ...simplify(pts.slice(at))];
}

// A closed loop starts and ends on the same point, which leaves RDP no chord
// to measure against. Split it at the point farthest from the start and
// simplify the halves; the result omits the repeated closing point.
function simplifyLoop(pts) {
  let k = 0, far = 0;
  pts.forEach(([x, y], i) => {
    const h = Math.hypot(x - pts[0][0], y - pts[0][1]);
    if (h > far) { far = h; k = i; }
  });
  const loop = [...simplify(pts.slice(0, k + 1)).slice(0, -1), ...simplify([...pts.slice(k), pts[0]])];
  return loop.slice(0, -1);
}

// Path data at one decimal, as relative l steps (shorter than absolute
// coordinates). Steps are differences of the rounded points, so rounding
// error never accumulates along the contour. offsetX places the frame in its
// filmstrip cell; being whole units, it moves only each contour's M.
function pathData(contours, offsetX) {
  const fmt = (tenths) => String(tenths / 10);
  return contours.map((pts) => {
    const q = simplifyLoop(pts).map(([x, y]) => [Math.round(x * 10) + offsetX * 10, Math.round(y * 10)]);
    const steps = q.slice(1).map((p, i) => fmt(p[0] - q[i][0]) + " " + fmt(p[1] - q[i][1]));
    return "M" + fmt(q[0][0]) + " " + fmt(q[0][1]) + "l" + steps.join(" ") + "z";
  }).join("");
}

// bakeBoil turns a Phosphor SVG (256-unit viewBox, filled outline paths) into
// the boiling SVG's markup: root class="boil", frames in <g class="boil-strip">.
// With no stylesheet the viewBox shows frame 1, a still pencil. Moving the
// strip is the stylesheet's business, so the SVG holds no <style> of its own —
// an inline SVG's <style> would apply to the whole page.
export function bakeBoil(sourceSVG) {
  if (!/viewBox="0 0 256 256"/.test(sourceSVG)) {
    throw new Error("boil: expected a Phosphor SVG with a 256-unit viewBox");
  }
  const d = [...sourceSVG.matchAll(/<path[^>]*\sd="([^"]+)"/g)].map((m) => m[1]).join("");
  if (!d) throw new Error("boil: source SVG has no path data");
  const contours = sampleContours(d);
  const paths = FRAME_SEEDS.map((seed, k) =>
    `<path class="f f${k + 1}" d="${pathData(inkFrame(contours, seed), k * CELL)}"/>`);
  return `<svg xmlns="http://www.w3.org/2000/svg" class="boil" viewBox="0 0 ${CELL} ${CELL}" fill="currentColor" aria-hidden="true">` +
    `<g class="boil-strip">` + paths.join("") + `</g></svg>\n`;
}
