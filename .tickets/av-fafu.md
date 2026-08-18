---
id: av-fafu
status: closed
deps: []
links: [av-siqf]
created: 2026-08-01T04:21:41Z
type: epic
priority: 1
assignee: Max Omdal
tags: [ui, render, agent]
---
# Artifact widgets: an at-a-glance tile per gallery card

Give every artifact an optional 'widget': a second self-contained HTML document that renders inside its gallery card, reading the SAME server-persisted state as the artifact itself. Informative only — pointer events are disabled and a click opens the artifact. Same opaque-origin sandbox and same per-artifact CSP allowlist as the artifact; no download/clipboard bridges and no state write-through, so the widget's authority is a strict subset of the artifact's. Artifacts with no widget fall back to a server-rendered default tile, so opting out costs nothing. A static widget (a stateless tool's icon card) is just a widget with no script — no separate mechanism.

## Design

Blob + column: artifacts.widget_blob_id (migration 011). API: GET/PUT/DELETE /api/artifacts/:id/widget — the single write path; PUT scans the widget body and reports its footprint but NEVER seeds the allowlist. Render: GET /w/:id on RENDER_ORIGIN, same buildCSP(allowlist), preamble in widget mode (state inlined for reads, writes cache-only, no bridges, no snippet picker) plus a minimal base stylesheet. Gallery card: pointer-events:none iframe, or the default tile when widget_blob_id is empty. Agent: set_widget/get_widget tools in ext/exhibit.ts + a widget section in the system prompt; a widget save emits exhibit_widget_saved and refreshes the agent preview pane. Edit page: a 'Gallery widget' panel (source + live preview + remove).

## Acceptance Criteria

1. An artifact with a widget shows it live in its gallery card, reading the artifact's cross-device state. 2. Clicking the widget opens the artifact — the widget is never interactive. 3. A widget cannot write state, download, or use the clipboard. 4. A widget's network reach is exactly the artifact's allowlist. 5. An artifact without a widget renders the default tile. 6. The agent can build and update a widget from the chat surface. 7. Four sample artifacts (stateful chart, next-item, static, progress) ship in dev/samples and seed into the dev environment.


## Notes

**2026-08-01T04:56:45Z**

POC landed. Schema: artifacts.widget_blob_id (migration 011). Render: GET /w/:id, same CSP/state, narrowed preamble (WIDGET short-circuits write-through; bridgeScript not spliced in). API: GET/PUT/DELETE /api/artifacts/:id/widget. Gallery: cardWidget partial (frame or default monogram tile), pointer-events:none. Edit page: widget panel + /partials/card-widget htmx swap. Agent: set_widget/get_widget + widget contract in the system prompt + exhibit_widget_saved. Five samples in dev/samples covering live/static/none, seeded by scripts/seed-samples.py. Docs: docs/widgets.md.
