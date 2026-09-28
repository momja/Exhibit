// Vendors the Phosphor Icons "regular" weight (the `ph`/`ph-*` classes used
// throughout the gallery) out of node_modules into internal/api/assets/phosphor,
// which is go:embed-ed and served from the app origin — never a third-party CDN.
//
// We only embed woff2 (universal support in evergreen browsers), so the
// upstream @font-face's woff/ttf/svg fallback lines are stripped; keeping them
// would point at files we don't ship and could 404.
//
// av-diue: two load-time trims on top of the vendoring.
//  1. font-display: block -> swap. Upstream blocks icon rendering for up to 3s
//     while the font downloads; swap paints fallback text immediately.
//  2. Subset to the icon classes listed in icons.txt (~50 of 1530). The
//     stylesheet shrinks from ~78KB to ~5KB and the woff2 from ~147KB to
//     ~6KB. The set is explicit on purpose: adding an icon means adding one
//     line, and a name that matches no upstream rule fails this build loudly
//     (see TestIconRegistryCoversUsage for the other direction). There is no
//     dynamic `ph-${name}` construction in the codebase — keep it that way.
// Subsetting needs python3 + fonttools/brotli. The Dockerfile's asset stage
// installs them (python3-fonttools, python3-brotli); a local checkout without
// them falls back to the full font with the swap rewrite still applied, with
// a warning — shipping a working page beats failing the whole asset build.
import { readFileSync, writeFileSync, copyFileSync, mkdirSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const src = path.join(here, "node_modules/@phosphor-icons/web/src/regular");
const licenseSrc = path.join(here, "node_modules/@phosphor-icons/web/LICENSE");
const outDir = path.join(here, "../../internal/api/assets/phosphor");

mkdirSync(outDir, { recursive: true });

copyFileSync(licenseSrc, path.join(outDir, "LICENSE"));

let css = readFileSync(path.join(src, "style.css"), "utf8");
const fontFaceSrc = /src:\s*\n(?:\s*url\([^)]*\)[^,;]*,?\s*\n?)+;/;
if (!fontFaceSrc.test(css)) {
  throw new Error("style.css @font-face src block not found — upstream format may have changed");
}
css = css.replace(fontFaceSrc, 'src: url("./Phosphor.woff2") format("woff2");');
css = css.replace("font-display: block;", "font-display: swap;");

// The explicit registry: one `ph-*` class per line, `#` comments and blanks
// ignored. Unknown names fail below, so a typo'd addition breaks the build
// instead of shipping tofu.
const used = new Set(
  readFileSync(path.join(here, "icons.txt"), "utf8")
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("#"))
    .map((l) => {
      const m = l.match(/^ph-([a-z0-9-]+)$/);
      if (!m) throw new Error(`icons.txt: bad line ${JSON.stringify(l)} — want ph-<name>`);
      return m[1];
    })
);

// Drop `.ph.ph-<name>:before` rules none of whose selectors is used, keeping
// the rule (and its codepoint) when any alias of it is used. Everything else
// — @font-face, the .ph base, size helpers — passes through untouched.
const codepoints = new Set();
const matched = new Set();
let kept = 0, dropped = 0;
css = css.replace(/\.ph\.ph-[a-z0-9-,.\s]+:before\s*\{[^}]*\}/g, (rule) => {
  const names = [...rule.matchAll(/\.ph\.ph-([a-z0-9-]+):before/g)].map((m) => m[1]);
  if (!names.length || !names.some((n) => used.has(n))) { dropped++; return ""; }
  names.forEach((n) => matched.add(n));
  const cp = rule.match(/content:\s*"\\([0-9a-fA-F]+)"/);
  if (cp) codepoints.add("U+" + cp[1].toUpperCase());
  kept++;
  return rule;
});
for (const name of used) {
  if (!matched.has(name)) {
    throw new Error(`icons.txt: .ph-${name} matches no upstream rule — typo or removed icon?`);
  }
}

const subsetPath = path.join(outDir, "Phosphor.woff2");
let subset = false;
try {
  if (codepoints.size === 0) throw new Error("no used codepoints found — refusing to emit an empty font");
  execFileSync("python3", [
    "-m", "fontTools.subset",
    path.join(src, "Phosphor.woff2"),
    `--unicodes=${[...codepoints].join(",")}`,
    "--flavor=woff2",
    `--output-file=${subsetPath}`,
  ], { stdio: "pipe" });
  subset = true;
} catch (err) {
  console.warn("phosphor subset unavailable (" + (err.message || err).split("\n")[0] + ") — shipping full font");
  copyFileSync(path.join(src, "Phosphor.woff2"), subsetPath);
}

writeFileSync(path.join(outDir, "regular.css"), css);

console.log(`Vendored Phosphor Icons (regular) -> ${path.relative(process.cwd(), outDir)} ` +
  `(${matched.size}/${used.size} registry classes matched, ${kept} rules kept, ${dropped} dropped, font ${subset ? "subset" : "FULL"})`);
