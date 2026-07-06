// ESLint flat config for the CodeMirror source-editor island.
//
// editor.js is a vanilla-JS ES module that runs in the browser and is bundled
// by esbuild (see package.json). The language options here mirror that build:
// es2020 target, ES-module syntax, browser globals — so lint sees the file the
// same way esbuild does. Just the plain eslint recommended rules; this isn't
// React or TypeScript, so no framework plugin is needed.
const js = require('@eslint/js');
const globals = require('globals');

module.exports = [
  js.configs.recommended,
  {
    files: ['editor.js'],
    languageOptions: {
      ecmaVersion: 2020,
      sourceType: 'module',
      globals: {
        ...globals.browser,
      },
    },
  },
  {
    // This config file itself is CommonJS run by Node, not part of the bundle.
    files: ['eslint.config.js'],
    languageOptions: {
      sourceType: 'commonjs',
      globals: {
        ...globals.node,
      },
    },
  },
];
