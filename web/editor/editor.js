// Artifact source-editor island: CodeMirror 6 mounted over a plain <textarea>.
//
// The textarea remains the form's source of truth — every editor change is
// synced back into textarea.value, so page code that reads the field (save,
// validation) works unchanged and the API write path stays untouched. If this
// script fails to load, the textarea is still there and fully functional.
import { basicSetup } from 'codemirror';
import { Compartment } from '@codemirror/state';
import { EditorView, keymap, placeholder } from '@codemirror/view';
import { indentWithTab } from '@codemirror/commands';
import { html } from '@codemirror/lang-html';
import { oneDark } from '@codemirror/theme-one-dark';

// basicSetup bundles the standard editing surface: default keymap + history
// (@codemirror/commands), search panel + selection-match highlighting
// (@codemirror/search), line numbers, bracket matching, and autocompletion.
// html() nests @codemirror/lang-javascript and @codemirror/lang-css inside
// <script>/<style> blocks, so full artifact documents highlight correctly.
// The textarea's own placeholder attribute, if any, is carried into the editor
// (the upload form's empty box keeps its hint; the edit page has none).
//
// The theme follows the system color scheme, as the page around it does
// (av-xie4): One Dark when it is dark, CodeMirror's own light styling when it
// is not. It sits in a Compartment so a change of system setting swaps it in
// place, without rebuilding the editor and losing its buffer, history, or
// selection.
const darkScheme = window.matchMedia('(prefers-color-scheme: dark)');
const themeFor = (dark) => (dark ? oneDark : []);

function mount(textarea) {
  const theme = new Compartment();
  const extensions = [
    basicSetup,
    keymap.of([indentWithTab]),
    html(),
    theme.of(themeFor(darkScheme.matches)),
    EditorView.updateListener.of((update) => {
      if (update.docChanged) textarea.value = update.state.doc.toString();
    }),
  ];
  if (textarea.placeholder) extensions.push(placeholder(textarea.placeholder));
  const view = new EditorView({
    doc: textarea.value,
    extensions,
  });
  darkScheme.addEventListener('change', (event) => {
    view.dispatch({ effects: theme.reconfigure(themeFor(event.matches)) });
  });
  textarea.insertAdjacentElement('afterend', view.dom);
  textarea.style.display = 'none';
  return view;
}

window.ArtifactEditor = { mount };
