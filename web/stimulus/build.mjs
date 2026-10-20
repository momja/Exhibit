import { copyFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const here = path.dirname(fileURLToPath(import.meta.url));
const source = path.join(here, 'node_modules/@hotwired/stimulus');
const destination = path.join(here, '../../internal/api/assets/stimulus');

mkdirSync(destination, { recursive: true });
copyFileSync(path.join(source, 'dist/stimulus.js'), path.join(destination, 'stimulus.js'));
copyFileSync(path.join(source, 'LICENSE.md'), path.join(destination, 'LICENSE.md'));

console.log('Vendored Stimulus ES module and license');
