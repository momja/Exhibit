---
id: av-6xvs
status: closed
deps: []
links: [av-kmwj]
created: 2026-08-31T05:32:44Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, ui, agent]
---
# Agent chat preview: host the runtime network prompt

The agent chat page embeds an artifact in a sandboxed frame and hosted none of
the runtime network prompt, so an artifact reaching an unapproved origin while
being built there failed silently.

av-kmwj listed this as out of scope on the grounds that the surface hosts no
capability bridges at all. That reasoning holds for downloads, clipboard,
external links and camera/microphone, which are separate capabilities with
their own approval semantics. It does not hold for the network prompt: /agent
is on APP_ORIGIN, served by our own template, with the artifact in a sandboxed
cross-origin iframe. That is the same trusted chrome the detail page has. The
prompt was simply never wired up.

Found alongside it: agent.js never sent __avHostReady at all, so the preamble's
buffered reports were never flushed there. That also left av-yvtb's
module-worker diagnostic undeliverable on this surface.

## Design

Extract the prompt into web/gallery/network-prompt.js and have both pages
install it, rather than copying ~90 lines into agent.js. The precedent is
state-api.js, which exists because the same ".." path bug had to be fixed in
three copies of hand-rolled URL building.

The dialog markup moves to a shared `networkPrompt` template partial, so the
ids the module addresses by name have one definition too.

The module owns what both pages share: the message listener, the report queue,
the dialog wiring, the host-ready handshake, the decision write. The caller
supplies the four things the surfaces genuinely differ in:

  frame()      - resolved on each use. htmx replaces #pv-frame after every
                 agent save, so a cached reference goes stale.
  artifactId() - resolved on each use. There is no artifact until the agent
                 creates one, and it can change afterwards.
  reload()     - the detail page reassigns src through /open, which mints a
                 fresh render token; the agent page re-renders the preview
                 fragment, which mints one in the fragment.
  report()     - a callback taking text, not an element: the detail page has a
                 status span and the agent page writes to its transcript.

announceTo() carries the handshake, and the agent page runs it again on every
htmx:afterSwap because each swap produces a new frame.

Explicitly still out of scope: the download, clipboard, external-link and
camera/microphone bridges on this surface. Each is its own capability with its
own approval and its own dialog; they belong together in one piece of work, not
smuggled in behind this one.

## Acceptance Criteria

1. An artifact in the agent preview that reaches an unapproved origin raises
   the prompt in the agent page's own chrome.
2. Allow writes the decision through the per-origin route and re-renders the
   preview pane so the widened CSP applies.
3. The prompt and the dialog have one definition shared with the detail page,
   so a later fix to either cannot land on one surface and miss the other.
4. Each swapped-in preview frame is told the host is listening, so a violation
   raised before the page noticed the new frame is not lost.
5. A session with no artifact yet raises no prompt: there is no id to record a
   decision against.
6. Tests cover the agent surface specifically, not only the shared module.

