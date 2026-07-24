// Copies the gallery page assets (index/detail/edit .css and .js) verbatim
// into internal/api/assets/gallery, which is go:embed-ed and served from the
// app origin under /assets/gallery/. No bundling or minification: the files
// are plain hand-written CSS/JS with no imports, and copying verbatim keeps
// the served bytes greppable by the Go tests that assert on rule/function
// presence.
//
// Each .js file is syntax-checked (node --check) before copying, so a slip in
// these hand-maintained scripts fails the asset build instead of surfacing as
// a broken page at runtime.
import { copyFileSync, mkdirSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.join(here, "../../internal/api/assets/gallery");

mkdirSync(outDir, { recursive: true });

const files = [
  // tokens.css/components.css/components.js are the shared layer (av-xgik,
  // av-41se) and must be linked before the page sheets/scripts that build on
  // them.
  "tokens.css", "components.css", "components.js",
  "index.css", "index.js",
  "detail.css", "detail.js",
  "edit.css", "edit.js",
  "notfound.css", "notfound.js",
  "agent.css", "agent.js",
];

for (const f of files) {
  const src = path.join(here, f);
  if (f.endsWith(".js")) {
    execFileSync(process.execPath, ["--check", src], { stdio: "inherit" });
  }
  copyFileSync(src, path.join(outDir, f));
}

console.log("Copied gallery page assets ->", path.relative(process.cwd(), outDir));
