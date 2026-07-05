package api

// phosphorCSSLink is injected into every app-shell page's <head>. It self-hosts
// Phosphor Icons (phosphoricons.com) from the app origin — never a third-party
// CDN, per docs/technical_stack.md §9 — via the CSS + webfont vendored into
// internal/api/assets/phosphor (regenerate with `make assets`; see
// web/icons/build.mjs). The render origin (untrusted artifact content) never
// loads this; it is app-shell only.
//
// Usage pattern for any new UI (pill hover controls, add/edit modals, ...):
//
//	<i class="ph ph-<icon-name>"></i>
//
// <icon-name> is any regular-weight slug from https://phosphoricons.com, e.g.
// ph-pencil-simple (edit), ph-x (close/detach), ph-plus (add), ph-trash
// (delete), ph-check (confirm).
const phosphorCSSLink = `<link rel="stylesheet" href="/assets/phosphor/regular.css">`
