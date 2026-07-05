// Vendors the Phosphor Icons "regular" weight (the `ph`/`ph-*` classes used
// throughout the gallery) out of node_modules into internal/api/assets/phosphor,
// which is go:embed-ed and served from the app origin — never a third-party CDN.
//
// We only embed woff2 (universal support in evergreen browsers), so the
// upstream @font-face's woff/ttf/svg fallback lines are stripped; keeping them
// would point at files we don't ship and could 404.
import { readFileSync, writeFileSync, copyFileSync, mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const src = path.join(here, "node_modules/@phosphor-icons/web/src/regular");
const licenseSrc = path.join(here, "node_modules/@phosphor-icons/web/LICENSE");
const outDir = path.join(here, "../../internal/api/assets/phosphor");

mkdirSync(outDir, { recursive: true });

copyFileSync(path.join(src, "Phosphor.woff2"), path.join(outDir, "Phosphor.woff2"));
copyFileSync(licenseSrc, path.join(outDir, "LICENSE"));

let css = readFileSync(path.join(src, "style.css"), "utf8");
const fontFaceSrc = /src:\s*\n(?:\s*url\([^)]*\)[^,;]*,?\s*\n?)+;/;
if (!fontFaceSrc.test(css)) {
  throw new Error("style.css @font-face src block not found — upstream format may have changed");
}
css = css.replace(fontFaceSrc, 'src: url("./Phosphor.woff2") format("woff2");');
writeFileSync(path.join(outDir, "regular.css"), css);

console.log("Vendored Phosphor Icons (regular) ->", path.relative(process.cwd(), outDir));
