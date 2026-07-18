---
id: av-xgik
status: in_progress
deps: []
links: []
created: 2026-07-18T15:23:11Z
type: chore
priority: 2
assignee: Max Omdal
parent: av-jafp
tags: [ui, gallery]
---
# Shared gallery design tokens + component CSS

Foundation for the security-posture UI epic (av-jafp). Gallery styling today is per-page CSS (web/gallery/index|detail|edit.css) plus inline styles hand-written into templates (e.g. partials.tmpl allowlist badges, detail.tmpl) and per-page injected vars (edit.tmpl injects --brand-blue). The badge (av-isb3), popover (av-41se), and edit page (av-p0a1) are about to add more components; give them a shared vocabulary to build on instead of more inline CSS. This is the "adopt a token + small component layer, not Tailwind" decision.

## Design

- tokens.css: promote the design tokens already used in the Paper mockups to committed CSS custom properties, loaded on every gallery page — colours (--color-ink / muted / accent / hairline / panel / ground), spacing (--space-*), radii (--radius-*), and --brand-blue (currently injected inline in edit.tmpl). go:embed'd like the other gallery assets.
- components.css: a small, hand-authored set of shared classes the epic needs — .capability-cluster (+ glyph), .badge, .btn / .btn-sec / .btn-danger, .card, .field, .select, .details-panel, allowlist row — plus the few flex/spacing utilities actually reused. No framework; no new build concept (web/gallery/build.mjs / esbuild already compiles gallery CSS into the embedded assets).
- Serve both under /assets/gallery/, link from the page templates, and remove the inline styles they replace (partials.tmpl, detail.tmpl) plus the per-page --brand-blue injection.
- Explicitly NOT Tailwind: keep the minimal-dependency, self-hostable, go:embed stance. Revisit a utility framework only if this hand-rolled layer starts creaking.

## Acceptance Criteria

- tokens.css + components.css exist in web/gallery/, are compiled into the embedded assets, and are linked from the gallery templates.
- Inline styles in partials.tmpl and detail.tmpl (and the inline --brand-blue in edit.tmpl) are replaced by shared tokens/classes.
- No visual regression on the existing gallery / detail / edit pages.
- av-isb3 and av-p0a1 build on these shared classes without re-introducing inline component styles.

Scoped to what the epic needs — not a full design-system rewrite.
