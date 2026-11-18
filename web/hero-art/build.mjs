import { copyFileSync, mkdirSync, readFileSync, rmSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const source = new URL('./images/', import.meta.url);
const target = new URL('../../internal/api/assets/hero-art/', import.meta.url);
const manifest = JSON.parse(readFileSync(new URL('manifest.json', source), 'utf8'));
const names = Object.keys(manifest);
if (names.length === 0) throw new Error('Hero derivatives are missing; run npm run export in design_files/brand-refresh/tools/hero-export');

// A stale file from an earlier build must not be embedded and served.
rmSync(target, { recursive: true, force: true });
mkdirSync(target, { recursive: true });
const mismatched = [];
for (const name of names) {
  copyFileSync(new URL(name, source), new URL(name, target));
  const bytes = statSync(new URL(name, target)).size;
  if (bytes !== manifest[name].bytes) mismatched.push(`${name}: ${bytes} bytes, manifest says ${manifest[name].bytes}`);
}
if (mismatched.length > 0) {
  console.error(`Hero derivatives differ from images/manifest.json:\n${mismatched.join('\n')}`);
  process.exit(1);
}
console.log(`Copied ${names.length} hero assets to ${fileURLToPath(target)}`);
