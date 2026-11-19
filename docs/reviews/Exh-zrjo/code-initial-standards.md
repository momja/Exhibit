Documented-standard finding:

- **P3 — Shared modal typography is copied into page CSS.** `web/gallery/detail.css:110,116` adds `font-size:var(--text-helper);line-height:var(--line-helper);color:var(--color-muted)` for Share helpers, including a second declaration for `.share-field-hint`. `docs/ui-components.md:203-205` says migrated dialogs use component defaults: “Do not copy these into page styles.” Those elements already use `.form-help` or `.modal-check small`, whose shared rules live in `components.css:455-456`. Remove these redundant typography declarations; retain feature-specific spacing. This keeps later component changes from being masked by higher-specificity page selectors.

Heuristic simplicity opportunities:

- **P3 — Possible Duplicated Code:** `web/gallery/pip.js:312-326` adds its own Arrow/Home/End/Tab menu navigation, including `const current = items.indexOf(document.activeElement)` and the same wrapping-index expression as `action-menu-controller.js:91-97`. The copies already differ: the shared controller supports Space on links and typeahead. Compose the existing `action-menu` behavior for More actions, retaining floating placement in `pip.js`; if the sibling trigger/panel geometry prevents direct composition, extract only the common keyboard navigation into a small imported helper.

- **P3 — Possible Duplicated Code / Feature Envy:** `web/gallery/tags.js:89-92` sets picker `aria-invalid` and error text, then dispatches `color-picker:refresh`, whose controller immediately sets those same properties (`color-picker-controller.js:57-62`). The hex regex also appears in Tags and twice in the controller (`:17,27`). Let the picker own validation presentation; use its existing `color-picker:change` validity result for feature buttons. Route `input()` and `refresh()` through one validation path.

- **P3 — Possible Duplicated Code:** `internal/color/tag.go:12-44` enumerates every preset hex twice, once in `Presets` and again in `PresetName`’s switch (`case "#B86562": return "Red"`). Store each ordered palette entry’s hex and name together, deriving lookup/display from that one definition.

No material htmx, native-module, modal-lifecycle, credential-client, or icon-loading violation found in the changed implementation. Tag mutations use server partial swaps; working-copy updates appropriately remain local. Read-only review; no builds or tests run.
