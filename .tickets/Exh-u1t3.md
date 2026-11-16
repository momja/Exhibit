---
id: Exh-u1t3
status: open
deps: []
links: []
created: 2026-11-16T06:04:43Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-38vu
tags: [spike, ui, render, security, design]
---
# Spike: how to display the "Made with Exhibit" attribution on a share page

Decide how an attribution line is displayed on `/s/:shareID`, before anything
in the epic is built. The share document is not a page we own in the ordinary
sense: it is the artifact's own stored HTML, rewritten at render time and served
under an opaque-origin sandbox. "Add a div at the bottom" has security, layout
and embed consequences, and this spike is where they get settled.

## The constraints that make it a real question

- **We can only inject server-side.** The body is a stored blob; the render path
  composes the document (`serveDoc` / `injectPreamble` / `headMeta` in
  `internal/render/render.go`, tags from `internal/render/opengraph.go`). Any
  badge is markup we prepend/append here, not something the share route's HTML
  can rely on a script to add.
- **Artifact code lives in the same document.** The sandbox is
  `sandbox allow-scripts allow-forms` over an opaque origin, so the artifact's
  own script shares the DOM and can remove, reorder, restyle or cover anything
  we inject. Decide what we actually promise: "layout-safe and honest" versus
  "cannot be removed by a hostile artifact" — the second needs a mechanism
  (re-asserted shadow root, closed shadow DOM plus MutationObserver, or chrome
  that is structurally outside the artifact's reach) and that mechanism is much
  more expensive. Say which one we are buying and why.
- **A share can be framed by a third party.** `EMBED_ORIGINS` exists so anyone
  may iframe a share. Inside a frame, the sandbox omits `allow-popups` and
  `allow-top-navigation`, and share pages deliberately have no capability
  bridges — so an attribution link inside a framed share **cannot open a new
  tab**. The options are: no-op in frames, mediate through the parent (there is
  no host to mediate), self-navigate the frame (rude and usually refused by the
  destination), or extend the share sandbox with `allow-popups` +
  `allow-popups-to-escape-sandbox` (a deliberate loosening of the share
  security model). This is the crux: measure it in a browser, do not reason it
  out on paper.
- **The artifact may be full-viewport.** Canvas tools, `100vh` layouts,
  `position:fixed` UIs, scroll-locked documents, drag surfaces. A fixed corner
  badge can overlap real controls; a reserved strip changes the artifact's
  viewport and can break a tool that measures the window. It must not capture
  focus, intercept pointer events over the tool, or sit on top of an artifact's
  own bottom bar.
- **Not our frame.** `/a/:id`, `/w/:id`, version views, widget documents and
  direct-open pages are framed by our own gallery, which already carries the
  brand. The spike confirms (with a test) that none of them gain the badge.

## Prototype and evaluate, at minimum

1. **Fixed corner badge** (bottom-right or bottom-left), minimal mark + "Made
   with Exhibit", click behavior per the embed answer above.
2. **Reserved bottom strip** outside the artifact's layout viewport (or a
   compensating CSS variable), for documents that scroll normally.
3. **Injected chrome the artifact cannot reach** — the expensive option, if 1
   and 2 both fail on real tools.

For each: a full-viewport canvas tool, a scroll-heavy document tool, and a tool
with its own bottom-fixed UI. Screenshots at 320 and 1280, light and dark, plus
one shot inside a third-party iframe. Record what breaks.

Also settle: does it show on the share only (recommended) or also on the widget
snapshot that becomes `og:image` (it must not turn the picture into a logo card);
does the owner's *hide* choice apply to embeds; and whether the badge should be
dismissible (if it can be dismissed it is not attribution, so probably not).

## Deliverable

A short report under `docs/reviews/<ticket>/` with the comparison, the browser
evidence for the framed-link question, a recommendation, the proposed env flag
and artifact column names, the exact injection point in `internal/render`, and
implementation tickets filed under Exh-38vu with one of them clearly the first.
Prototype artifacts are throwaway; review images live outside the checkout.

## Acceptance criteria

1. At least three placements compared with screenshots and a named
   recommendation.
2. The framed-share link question is answered with measured browser evidence,
   and the answer is either "works without changing the sandbox" or a written
   proposal to change it with its security cost.
3. The robustness guarantee is stated plainly, including what a hostile artifact
   can still do, and how much that costs.
4. Confirms no change to `/a/:id`, `/w/:id`, version views, widgets, direct
   open, or `og:image`.
5. Names the flag, the column and its tri-state semantics, and the injection
   site (file + function) for implementation.
6. Implementation tickets filed under the epic, ordered.

