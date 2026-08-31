---
id: av-tan0
status: open
deps: []
links: [av-kmwj, av-g234]
created: 2026-08-31T05:33:14Z
type: feature
priority: 3
assignee: Max Omdal
tags: [security, render, ui]
---
# Top-level render and shares: a blocked origin fails with no explanation

Opened top-level at RENDER_ORIGIN/a/:id — the "Open in new tab" destination —
or through a share at /s/:shareID, an artifact whose request the CSP blocks
fails silently. There is no prompt and no banner, and the visitor is given no
indication that anything was refused or that a setting exists.

This is the current behaviour by design rather than by oversight, and the
design is worth revisiting rather than assuming. av-kmwj's prompt and av-yvtb's
capability banner both live in the render preamble's framed-only half, guarded
by `window.parent !== window`, for two reasons that still hold:

- There is no host frame to post a report to. The report has nowhere to go.
- That document's only DOM belongs to the artifact. A prompt drawn there is a
  prompt the artifact could forge, and the visitor has no chrome to tell ours
  from a convincing copy. This is why browsers put permission prompts in
  browser UI rather than in the page.

## Design

The question is not "can we show a prompt there" — we should not, for the
forgery reason above. It is whether showing *nothing* is the right answer, and
what a non-deciding notice would cost.

## The shape worth considering

A notice, not a prompt. It states that a request was blocked and links to the
artifact's allowlist editor on APP_ORIGIN, where the decision is actually made
under chrome we control. Nothing is granted in the artifact's document.

## The cost, stated honestly

An artifact can already render arbitrary HTML, including a convincing fake
Exhibit banner, so the notice does not create a forgery capability. What it
creates is a *habit*: visitors learn that Exhibit speaks to them from inside an
artifact page, which is exactly the intuition a malicious artifact wants them
to have. That is a real cost and it is why this is a design decision rather
than a missing feature.

## Shares are a separate case

A share visitor has no account and cannot change an allowlist at all. A notice
that links to a settings page they cannot reach is worse than silence. If
anything is shown there it has to say something different, or nothing.

## What is not the answer

Making /open frame the render origin instead of redirecting to it. That would
give the prompt trusted chrome, but it destroys the reason top-level render
exists: a real origin, where the things an opaque sandbox denies actually work.
av-yvtb (module workers) and av-mv3k (camera/microphone) both send users there
for precisely that.

## Acceptance Criteria

1. A decision is recorded, either way. If nothing is shown top-level, the
   reasoning is written down in security.md as a decision rather than left to
   be rediscovered as a gap.
2. If a notice ships, it takes no decision inside the artifact's document: it
   states what happened and sends the visitor to app-origin chrome.
3. The share case is answered separately from the signed-in case, because a
   share visitor cannot act on an allowlist at all.
4. Nothing regresses the reason top-level render exists: it keeps a real
   origin, and "Open in new tab" keeps working for the capabilities the
   sandbox denies.

