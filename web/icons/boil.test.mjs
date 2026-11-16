/* Exh-s8yu — the baked, boiling pencil behind the agent's working indicator.
 *
 * The bake is a build step, so what matters is that it is one: the same pinned
 * source gives the same bytes on every run, in any process, and the embedded
 * asset is that output rather than something left over from an older build.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { gzipSync } from "node:zlib";
import { fileURLToPath } from "node:url";
import { bakeBoil, PENCIL_SIMPLE_SOURCE } from "./boil.mjs";

const source = readFileSync(PENCIL_SIMPLE_SOURCE, "utf8");
const embedded = fileURLToPath(new URL("../../internal/api/assets/phosphor/pencil-simple-boil.svg", import.meta.url));

test("baking the pinned pencil twice gives identical bytes", () => {
  assert.equal(bakeBoil(source), bakeBoil(source));
});

test("a separate process bakes the same bytes", () => {
  // A fresh process has no state left over from an earlier bake in this one.
  const out = execFileSync(process.execPath, ["--input-type=module", "-e", `
    import { readFileSync } from "node:fs";
    import { bakeBoil, PENCIL_SIMPLE_SOURCE } from ${JSON.stringify(new URL("./boil.mjs", import.meta.url).href)};
    process.stdout.write(bakeBoil(readFileSync(PENCIL_SIMPLE_SOURCE, "utf8")));
  `], { encoding: "utf8" });
  assert.equal(out, bakeBoil(source));
});

test("the embedded asset is what the build bakes today", () => {
  let built;
  try { built = readFileSync(embedded, "utf8"); } catch {
    assert.fail(`${embedded} is missing: run make assets`);
  }
  assert.equal(built, bakeBoil(source), "stale embedded asset: run make assets");
});

test("the SVG is a filmstrip of three distinct frames and nothing else", () => {
  const svg = bakeBoil(source);
  assert.match(svg, /^<svg [^>]*class="boil"[^>]*><g class="boil-strip"><path /);
  assert.match(svg, /viewBox="0 0 256 256"/);
  assert.match(svg, /fill="currentColor"/);
  assert.match(svg, /aria-hidden="true"/);
  const frames = [...svg.matchAll(/<path class="f (f\d)" d="([^"]+)"\/>/g)];
  assert.deepEqual(frames.map((m) => m[1]), ["f1", "f2", "f3"]);
  assert.equal(new Set(frames.map((m) => m[2])).size, 3, "every frame is its own drawing");
  frames.forEach(([, name, d], k) => {
    // The pencil has three contours: the outline and its two holes.
    assert.equal(d.match(/M/g).length, 3);
    assert.match(d, /^(M-?\d+(\.\d)? -?\d+(\.\d)?l[-\d. ]+z)+$/, "relative path data at one decimal");
    // Each frame stays inside its own 256-unit cell, so the viewBox that
    // shows one cell never shows a sliver of a neighbour.
    const [minX, maxX, minY, maxY] = extent(d);
    assert.ok(minX > k * 256 && maxX < (k + 1) * 256, `${name} x ${minX}..${maxX} is outside cell ${k}`);
    assert.ok(minY > 0 && maxY < 256, `${name} y ${minY}..${maxY}`);
  });
  // Inline in the page, a <style> would apply to the whole document; which
  // frame shows is agent.css's business.
  assert.doesNotMatch(svg, /<style|<script|<image/);
});

// The absolute extent of relative path data: M x y, then l steps, per contour.
function extent(d) {
  const xs = [], ys = [];
  for (const [, start, steps] of d.matchAll(/M([^l]+)l([^z]+)z/g)) {
    let [x, y] = start.trim().split(" ").map(Number);
    xs.push(x); ys.push(y);
    const n = steps.trim().split(" ").map(Number);
    for (let i = 0; i < n.length; i += 2) { x += n[i]; y += n[i + 1]; xs.push(x); ys.push(y); }
  }
  return [Math.min(...xs), Math.max(...xs), Math.min(...ys), Math.max(...ys)];
}

test("the asset stays within its 2 KB gzipped budget", () => {
  const gz = gzipSync(bakeBoil(source), { level: 9 }).length;
  assert.ok(gz <= 2048, `${gz} bytes gzipped`);
});

test("a source that is not a 256-unit Phosphor drawing is refused", () => {
  assert.throws(() => bakeBoil('<svg viewBox="0 0 24 24"><path d="M0 0h24v24z"/></svg>'), /256-unit/);
  assert.throws(() => bakeBoil('<svg viewBox="0 0 256 256"></svg>'), /no path data/);
});
