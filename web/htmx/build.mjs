// Vendors htmx out of node_modules into internal/api/assets/htmx, which is
// go:embed-ed and served from the app origin — never a third-party CDN, same
// rule the Phosphor icons follow (technical_stack.md §9).
//
// Only the minified single-file build is copied: htmx is used here as a
// fragment-swapping helper (agent preview, av-6m3e), not as a module the other
// page scripts import, so the extensions and ESM entry points would ship bytes
// nothing loads.
import { copyFileSync, mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const src = path.join(here, "node_modules/htmx.org/dist");
const outDir = path.join(here, "../../internal/api/assets/htmx");

mkdirSync(outDir, { recursive: true });

copyFileSync(path.join(src, "htmx.min.js"), path.join(outDir, "htmx.min.js"));
copyFileSync(path.join(here, "node_modules/htmx.org/LICENSE"), path.join(outDir, "LICENSE"));

console.log("Vendored htmx ->", path.relative(process.cwd(), outDir));
