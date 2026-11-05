// Artifact source-editor island: CodeMirror 6 mounted over a plain <textarea>.
//
// The textarea remains the form's source of truth — every editor change is
// synced back into textarea.value, so page code that reads the field (save,
// validation) works unchanged and the API write path stays untouched. If this
// script fails to load, the textarea is still there and fully functional.
import { basicSetup } from 'codemirror';
import { Compartment, EditorState, Prec } from '@codemirror/state';
import { EditorView, keymap, placeholder, tooltips } from '@codemirror/view';
import { defaultHighlightStyle, HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags } from '@lezer/highlight';
import { indentWithTab } from '@codemirror/commands';
import { searchPanelOpen } from '@codemirror/search';
import { html } from '@codemirror/lang-html';
import { color as oneDarkColors, oneDarkHighlightStyle, oneDarkTheme } from '@codemirror/theme-one-dark';

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
// Keep syntax highlighting, while app tokens own the editor's surfaces and
// subdued text. Both palettes cover base, active-line and selected text.
const appEditorTheme = EditorView.theme({
  '&': { backgroundColor: 'var(--color-field)', color: 'var(--color-ink)' },
  '.cm-content': { caretColor: 'var(--color-accent)' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--color-accent)' },
  '.cm-gutters': { backgroundColor: 'var(--color-field)', color: 'var(--color-code-muted)', border: 'none' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--color-code-active)' },
  '.cm-placeholder': { color: 'var(--color-code-muted)', fontStyle: 'normal', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' },
  '.cm-selectionMatch, .cm-searchMatch': { backgroundColor: 'var(--color-code-match)', outline: 'none' },
  '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'var(--color-code-selection)', outline: '1px solid var(--color-accent)' },
  '&.cm-focused .cm-matchingBracket, .cm-matchingBracket': { backgroundColor: 'var(--color-code-match)', outline: '1px solid var(--color-accent-tint-border)' },
  '&.cm-focused .cm-nonmatchingBracket, .cm-nonmatchingBracket': { backgroundColor: 'var(--color-danger-tint)' },
  '.cm-specialChar': { color: 'var(--color-danger-ink)' },
  '.cm-foldPlaceholder': { backgroundColor: 'var(--color-subtle)', color: 'var(--color-code-muted)', border: '1px solid var(--color-border)', borderRadius: 'var(--radius-sm)' },
  '.cm-panels': { backgroundColor: 'var(--color-panel)', color: 'var(--color-ink)', flexShrink: '0' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--color-border)' },
  '.cm-panels.cm-panels-bottom': { borderTop: '1px solid var(--color-border)' },
  '.cm-panel': { fontFamily: 'system-ui, sans-serif', fontSize: '.875rem', lineHeight: '1.4', padding: '10px' },
  '.cm-panel:not(.cm-search)': { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '8px' },
  '.cm-panel input, .cm-panel button': { fontFamily: 'inherit' },
  '.cm-panel .cm-textfield, .cm-panel .cm-button, .cm-panel button[name=close], .cm-panel .cm-dialog-close': {
    boxSizing: 'border-box', minHeight: 'var(--control-height-sm)', fontSize: 'var(--field-font-size)', lineHeight: '1.4',
    borderRadius: 'var(--radius-md)', padding: '4px 8px', color: 'var(--color-ink)',
    backgroundColor: 'var(--color-field)', backgroundImage: 'none', border: '1px solid var(--color-field-border)',
  },
  '.cm-panel .cm-textfield': { minWidth: '0', width: '12rem', maxWidth: '100%' },
  '.cm-panel .cm-button, .cm-panel button[name=close], .cm-panel .cm-dialog-close': { cursor: 'pointer', whiteSpace: 'nowrap', fontSize: '.875rem', backgroundColor: 'var(--color-panel)', borderColor: 'var(--color-border)' },
  '.cm-panel .cm-button:hover, .cm-panel button[name=close]:hover, .cm-panel .cm-dialog-close:hover': { backgroundColor: 'var(--color-accent-tint)' },
  '.cm-panel.cm-dialog': { padding: '10px' },
  '.cm-dialog form': { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '6px', flex: '1 1 0', minWidth: '0' },
  '.cm-panel.cm-dialog label': { display: 'inline-flex', alignItems: 'center', gap: '6px', fontSize: 'inherit', flexWrap: 'wrap' },
  '.cm-dialog .cm-textfield': { width: '4.5rem' },
  '.cm-panel .cm-dialog-close': { position: 'static', width: 'var(--control-height-sm)', fontSize: '20px', padding: '0', border: '1px solid var(--color-border)', backgroundColor: 'var(--color-panel)' },
  '.cm-panel input:focus-visible, .cm-panel button:focus-visible': { outline: '2px solid var(--color-accent)', outlineOffset: '2px' },
  '.cm-panel.cm-search': { display: 'grid', gridTemplateColumns: 'repeat(3, max-content) minmax(0, 1fr) repeat(4, max-content)', gap: '6px', alignItems: 'center', padding: '10px' },
  '.cm-panel.cm-search input, .cm-panel.cm-search button, .cm-panel.cm-search label': { margin: '0' },
  '.cm-search br': { display: 'none' },
  '.cm-search .cm-textfield': { width: '100%' },
  '.cm-search input[name=search]': { gridRow: '1', gridColumn: '1 / 5' },
  '.cm-search button[name=next]': { gridRow: '1', gridColumn: '5', backgroundColor: 'var(--color-accent)', color: 'var(--color-on-accent)', borderColor: 'var(--color-accent)' },
  '.cm-panel.cm-search button[name=next]:hover': { backgroundColor: 'var(--brand-blue-hover)', color: 'var(--color-on-accent)', borderColor: 'var(--brand-blue-hover)' },
  '.cm-search button[name=prev]': { gridRow: '1', gridColumn: '6' },
  '.cm-search button[name=select]': { gridRow: '1', gridColumn: '7' },
  '.cm-panel.cm-search button[name=close]': { position: 'static', gridRow: '1', gridColumn: '8', width: 'var(--control-height-sm)', padding: '0', fontSize: '20px', border: '1px solid var(--color-border)', backgroundColor: 'var(--color-panel)' },
  '.cm-panel.cm-search label': { gridRow: '2', display: 'flex', alignItems: 'center', gap: '6px', minHeight: 'var(--control-height-sm)', fontSize: 'inherit', whiteSpace: 'normal' },
  '.cm-search input[type=checkbox]': { flexShrink: '0', width: '18px', height: '18px', minHeight: '18px', padding: '0', accentColor: 'var(--color-accent)' },
  '.cm-search input[name=replace]': { gridRow: '3', gridColumn: '1 / 5' },
  '.cm-search button[name=replace]': { gridRow: '3', gridColumn: '5' },
  '.cm-search button[name=replaceAll]': { gridRow: '3', gridColumn: '6 / 8' },
  '&.cm-editor[data-search-open=true]': { height: 'auto', minHeight: '400px', maxHeight: 'none' },
  '&.cm-editor[data-search-open=true] > .cm-scroller': { height: '240px', maxHeight: '320px', flexGrow: '1' },
  '.cm-scroller': { minHeight: '120px' },
  '.cm-tooltip': { backgroundColor: 'var(--color-panel)', color: 'var(--color-ink)', border: '1px solid var(--color-border)', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-card-hover)', fontFamily: 'system-ui, sans-serif', fontSize: '.875rem', maxWidth: 'calc(100vw - 32px)' },
  '.cm-tooltip .cm-tooltip-arrow:before': { borderTopColor: 'var(--color-border)', borderBottomColor: 'var(--color-border)' },
  '.cm-tooltip .cm-tooltip-arrow:after': { borderTopColor: 'var(--color-panel)', borderBottomColor: 'var(--color-panel)' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul': { borderRadius: 'inherit', fontFamily: 'inherit', minWidth: '180px', maxWidth: 'calc(100vw - 32px)', maxHeight: '240px' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li': { boxSizing: 'border-box', display: 'flex', alignItems: 'center', padding: '6px 10px', minHeight: 'var(--control-height-sm)', lineHeight: '20px' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]': { backgroundColor: 'var(--color-accent-tint)', color: 'var(--color-ink)' },
  '.cm-tooltip.cm-completionInfo': { padding: '8px 12px', maxWidth: 'calc(100vw - 32px)' },
  '.cm-completionDetail, .cm-completionIcon': { color: 'var(--color-muted)' },
  '@media (max-width: 767px)': {
    '&.cm-editor[data-search-open=true]': { minHeight: '480px' },
    '.cm-panel .cm-button': { whiteSpace: 'normal', overflowWrap: 'anywhere' },
    '.cm-panel .cm-textfield, .cm-panel .cm-button, .cm-panel button[name=close], .cm-panel .cm-dialog-close, .cm-panel.cm-search label': { minHeight: '44px' },
    '.cm-panel .cm-dialog-close': { width: '44px' },
    '.cm-panel.cm-search': { display: 'flex', flexWrap: 'wrap', gap: '8px' },
    '.cm-search input[name=search]': { order: '0', flex: '1 1 calc(100% - 52px)' },
    '.cm-panel.cm-search button[name=close]': { order: '1', flex: '0 0 44px', width: '44px' },
    '.cm-search button[name=next], .cm-search button[name=prev], .cm-search button[name=select]': { order: '2', flex: '1 1 calc((100% - 16px) * .3)', minWidth: 'max-content', whiteSpace: 'nowrap', overflowWrap: 'normal' },
    '.cm-search button[name=prev]': { flexBasis: 'calc((100% - 16px) * .4)' },
    '.cm-panel.cm-search label': { order: '3', flex: '0 0 auto', whiteSpace: 'nowrap' },
    '.cm-search input[name=replace]': { order: '4', flex: '1 1 100%' },
    '.cm-search button[name=replace], .cm-search button[name=replaceAll]': { order: '5', flex: '1 1 calc((100% - 8px) / 2)', minWidth: '0' },
    '.cm-tooltip.cm-tooltip-autocomplete > ul > li': { minHeight: '44px' },
  },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'var(--color-code-selection)'
  },
});
// A non-fallback override disables basicSetup's fallback highlighter. Keep the
// complete light palette explicit, with its three weakest colors darkened for
// legibility on the app's active-line and selection surfaces too.
const lightSyntax = syntaxHighlighting(HighlightStyle.define([
  ...defaultHighlightStyle.specs,
  { tag: [tags.typeName, tags.namespace], color: '#06734a' },
  { tag: [tags.regexp, tags.escape, tags.special(tags.string)], color: '#b53600' },
  { tag: tags.invalid, color: '#c5221f' },
]));
const darkSyntax = syntaxHighlighting(HighlightStyle.define(
  oneDarkHighlightStyle.specs.map((spec) => {
    if (spec.color === oneDarkColors.coral) return { ...spec, color: '#ed8088' };
    if (spec.color === oneDarkColors.violet) return { ...spec, color: '#cf87e5' };
    if (spec.color === oneDarkColors.stone) return { ...spec, color: 'var(--color-code-muted)' };
    return spec;
  })
));
const subduedSyntax = syntaxHighlighting(HighlightStyle.define([
  { tag: [tags.comment, tags.meta, tags.processingInstruction, tags.documentMeta], color: 'var(--color-code-muted)' },
]));
const themeFor = (dark) => [
  ...(dark ? [oneDarkTheme, darkSyntax] : [lightSyntax]),
  Prec.highest(appEditorTheme),
  Prec.highest(subduedSyntax),
];

function mount(textarea) {
  const theme = new Compartment();
  const extensions = [
    basicSetup,
    EditorState.phrases.of({ next: 'Next', previous: 'Previous', all: 'All', replace: 'Replace', 'replace all': 'Replace all', 'match case': 'Match case', regexp: 'Regex', 'by word': 'Whole word', close: 'Close', go: 'Go' }),
    EditorView.editorAttributes.of(view => ({ 'data-search-open': String(searchPanelOpen(view.state)) })),
    EditorView.contentAttributes.of({
      'aria-label': textarea.getAttribute('aria-label') || textarea.labels?.[0]?.textContent.trim() || 'HTML source',
      ...(textarea.getAttribute('aria-describedby') ? {'aria-describedby': textarea.getAttribute('aria-describedby')} : {}),
    }),
    tooltips({ tooltipSpace: () => {
      const viewport = window.visualViewport;
      const left = viewport?.offsetLeft ?? 0, top = viewport?.offsetTop ?? 0;
      return { left: left + 8, top: top + 8,
        right: left + (viewport?.width ?? document.documentElement.clientWidth) - 8,
        bottom: top + (viewport?.height ?? document.documentElement.clientHeight) - 8 };
    } }),
    keymap.of([indentWithTab]),
    html(),
    theme.of(themeFor(darkScheme.matches)),
    EditorView.updateListener.of((update) => {
      if (update.docChanged) {
        textarea.value = update.state.doc.toString();
        textarea.dispatchEvent(new Event('input', { bubbles: true }));
      }
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
