---
id: Exh-z3fh
status: open
deps: []
links: []
created: 2026-11-26T18:21:16Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, widget, agent, design-tbd]
---
# Rework the edit page's widget generation UI

Placeholder ticket. The Card widget panel on the artifact edit page needs a redesigned generation experience; the interaction spec is TBD and should be written before implementation.

Current surface (av-fafu, Exh-rwgv): one panel with a live tile preview, a raw "Widget HTML" code editor, and a promptless Generate / Regenerate button. Generate POSTs to /api/artifacts/:id/widget/generate, which starts a one-shot agent session and returns a session id; the page follows the session SSE stream and swaps in the saved widget when exhibit_widget_saved arrives. Remove deletes the widget and falls back to the default tile. The only generation input is the button itself — internal/agent/prompts/generate_widget.tmpl is a fixed user turn.

Observed problems to address (not yet specified): the panel reads as a code editor rather than a widget builder on a phone; generation offers no way to say what the widget should show or emphasize; there is no preview of a candidate before it is saved, and no way to compare or choose among results; progress and failure live in a single status line (180 s timeout, guardrail refusal, "agent finished without saving a widget"); the relationship between Generate and hand-editing the HTML is implicit.

Specs TBD — decide before building:
1. Generation entry point: keep the single button, or collect a short brief (goal/emphasis/style/size)? If a brief is collected, where is it composed and validated, and does the fixed server-side prompt become a template with user input?
2. Preview model: live tile as today, skeleton while generating, candidate carousel, or before/after? What is saved vs previewed?
3. Editor model: keep raw HTML as the advanced surface, hide it behind a disclosure, or replace it with presets plus a custom-HTML escape hatch?
4. Iteration: Regenerate only, or refine/retry with feedback, variants to choose from, or undo to the previous widget?
5. Failure and in-progress states on desktop and phone, including guardrail refusals, timeouts, no-agent-key states, and lost SSE.
6. Mobile layout of preview, source, actions and status at phone widths.
7. Reuse: should this share the agent chat session surface and set_widget events, or stay a self-contained panel?

Constraints: follow docs/widgets.md (widgets are read-only, tile-sized, state from server-injected storage) and the UI conventions in docs/ui-components.md — Go template partials, htmx region swaps, Stimulus controllers, native ES modules, self-hosted Phosphor icons. The widget save path and optimistic concurrency (av-n0gl, av-t5l8) stay as they are unless the spec says otherwise.

## Design

TBD. No design decision has been made. Candidate directions to evaluate once the questions above are answered: (a) keep the button and move guidance into the fixed prompt; (b) a small brief form that feeds a server-rendered generation request; (c) a gallery of generated variants with pick-and-refine; (d) a minimal panel that hides raw HTML until "Edit HTML" is chosen. Whatever is chosen must say how the preview is produced without saving, and must preserve the current single write path (PUT /api/artifacts/:id/widget).

## Acceptance Criteria

TBD — write these after the interaction spec is decided. At minimum they should pin: the generation entry point and its server contract; what the user sees while generating and on failure; how a candidate becomes the saved widget; the mobile layout; and browser tests plus screenshots for desktop and phone.

