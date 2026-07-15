// Artifact source-editor island: CodeMirror 6 mounted over a plain <textarea>.
//
// The textarea remains the form's source of truth — every editor change is
// synced back into textarea.value, so page code that reads the field (save,
// validation) works unchanged and the API write path stays untouched. If this
// script fails to load, the textarea is still there and fully functional.
import { basicSetup } from 'codemirror';
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
function mount(textarea) {
  const extensions = [
    basicSetup,
    keymap.of([indentWithTab]),
    html(),
    oneDark,
    EditorView.updateListener.of((update) => {
      if (update.docChanged) textarea.value = update.state.doc.toString();
    }),
  ];
  if (textarea.placeholder) extensions.push(placeholder(textarea.placeholder));
  const view = new EditorView({
    doc: textarea.value,
    extensions,
  });
  textarea.insertAdjacentElement('afterend', view.dom);
  textarea.style.display = 'none';
  return view;
}

window.ArtifactEditor = { mount };
