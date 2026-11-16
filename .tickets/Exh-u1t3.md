---
id: Exh-u1t3
status: in_progress
deps: []
links: []
created: 2026-11-16T06:04:43Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-38vu
tags: [ui, render, security, shares]
---
# Top banner on share pages: build the default, and let reviewers judge it

The display question is **decided**: a bar across the top of the share page,
the exhibit mark at icon size, the words "made with exhibit", linked to
`https://exhibit.dizzard.net`. Build that, and let reviewer subagents judge the
result instead of the product owner — iterate against their verdicts until they
pass.

The rest of this ticket is what makes that default safe to ship. The share
document is not a page we own in the ordinary sense: it is the artifact's own
stored HTML, rewritten at render time and served under an opaque-origin sandbox.
A banner has security, layout and embed consequences, and they still have to be
answered — just not by choosing a placement.

## The decided default

- **Placement:** a full-width strip above the artifact, at the top of the page.
- **Content:** the Exhibit mark at icon size, then the text "made with exhibit".
- **Target:** `https://exhibit.dizzard.net` (absolute, hardcoded as the public
  product origin for this feature; make it a single named constant, not a
  string sprinkled through templates).
- **Copy case:** lower case as written above. Do not title-case it.
- **Only on `/s/:shareID`.** Never `/a/:id`, `/w/:id`, version views, widget
  documents or direct-open pages — those are framed by our own gallery, which
  already carries the brand.

## Still open — resolve with evidence, not preference

- **Framed shares.** Inside a third-party iframe under `EMBED_ORIGINS` the
  sandbox omits `allow-popups` and `allow-top-navigation`, and share pages have
  no capability bridges, so a link there may not open. Measure what actually
  happens in Chromium and WebKit and pick one: link is live, link is inert in
  frames, or the sandbox is loosened with a written justification. Do not
  silently ship a link that does nothing.
- **Top strip and viewport assumptions.** A top strip reduces the artifact's
  viewport. Verify a `100vh` layout, a full-bleed canvas tool, and a
  scroll-locked document still work. If a strip breaks them, the fallback is a
  fixed-position overlay with reserved space agreed by CSS rather than a
  permanent layout shift — but measure before diverging from "top banner".
- **Robustness.** The artifact's script shares the document and can remove or
  cover anything we inject. State plainly what the guarantee is (layout-safe and
  honest vs. hostile-artifact-proof) and how much it costs. A rejected
  mutation-observing shadow-root mechanism is an acceptable answer if it is
  written down.
- **Interaction.** It must not capture focus, swallow pointer events over the
  tool, or sit on top of the artifact's own chrome.

## How it is judged

Subagent reviewers, not the owner, decide whether the banner ships. Run at least
two independent rounds — a visual round (screenshots at 320 px and 1280 px,
light and dark, plus one inside a third-party iframe) and a code/security round
against `docs/security.md`'s render surface rules. Each round is a fresh
delegated task carrying the previous round's findings. Iterate until both pass;
report the verdicts with the evidence they were based on.

## Acceptance criteria

1. The banner renders on `/s/:shareID` as decided: top strip, icon-size mark,
   "made with exhibit", linking to `https://exhibit.dizzard.net`.
2. `/a/:id`, `/w/:id`, version views, widget documents and direct-open pages are
   byte-identical to today, pinned by test.
3. The framed-share behaviour is measured in a real browser and matches what the
   ticket says it does.
4. A `100vh` tool, a canvas tool and a bottom-fixed-chrome tool are all still
   usable with the banner present; no focus or pointer theft.
5. The robustness guarantee is stated in the epic and justified.
6. Reviewers pass it: visual round and code/security round, findings resolved,
   screenshots attached to the epic's PR.

## Notes

**2026-11-16T21:02:06Z**

Recovery preserved existing implementation. Fixed attribution test configuration starting the app twice, pinned direct-open response digest, tested duplication policy and agent-credential refusal. Chromium layout/theme/embed and native anchor checks pass. WebKit rerun accounts for browser focus restoration across reload and shared-host load. Final evidence and exact check limitations will be recorded on PR; reviewer verdicts are not supplied by implementer.
