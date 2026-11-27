---
id: av-fcew
status: open
deps: []
links: []
created: 2026-10-18T16:49:32Z
type: bug
priority: 3
assignee: Max Omdal
tags: [agent, ui]
---
# Agent page: keystrokes stop reaching the composer (focus stuck in preview frame)

Placeholder. Seen once in production (2026-10-18) on /agent?artifact=b9eccab1-8a8c-4c79-9922-e8ad5682d63a (GIF Viewer), desktop Chrome on macOS. Not reproduced yet.

## Symptom
The chat composer looks normal but typing does nothing. A reload probably clears it (not verified; the tab was kept in the broken state for inspection).

## Evidence from the stuck tab
- `#input` is not disabled, readOnly or inert; `elementFromPoint` over it returns the textarea; `document.activeElement` is the textarea and `document.hasFocus()` is true.
- Real keystrokes never reach the top document. Capture listeners on `window` saw no keydown, beforeinput or input. A freshly injected `<input>` also could not be typed into.
- Synthetic `KeyboardEvent`s dispatched from JS propagate normally and are not prevented, so no page script is eating them. Nothing in agent.js, components.js, network-prompt.js or state-api.js calls stopPropagation.
- Pressing Tab moved focus to the artifact's `#dropZone` inside `#pv-frame` (focus ring visible), with no blur or focusin on the host. Chrome routes keyboard input to the sandboxed preview frame while the host document believes the textarea is focused. Clicking the textarea, or blur() then focus() from JS, does not recover it.
- Neither the artifact nor the render preamble calls focus(). The artifact has a tabindex=0 drop zone and document-level paste/keydown listeners.

## Suspected trigger
The user thinks it began after opening the artifact in a new tab via "Open ↗" and switching back. Possibly Chrome restoring focus into the cross-origin frame on tab reactivation. Attempts that did NOT reproduce it:
- fresh load, then type
- click in preview, click composer, type
- focus an element inside #pv-frame, fire exhibit:artifact-saved (htmx swap), click composer, type
- "Open ↗", return, type (extension-driven and by hand, with and without clicking the preview first)

The session had several real agent saves (preview swaps) before the bug appeared.

## Next step
Find a repro first. Candidate mitigations once one exists: on htmx:before:swap of #pv-frame, or on returning to the tab, move focus to the composer if the frame holds it.


## Notes

**2026-11-27T08:00:44Z**

Headless Chromium: a sandboxed srcdoc iframe was focused, replaced via innerHTML, then the textarea was focused and sent real keys; typing succeeded, so the reported failure was not reproduced. An initial autofocus attempt timed out before focusing the frame. Tab reactivation and swaps while hidden were not reproduced or tested. Before a preview-pane htmx swap, move focus from the current preview iframe to the composer, preserving other focus.

**2026-11-27T08:14:33Z**

A focus mitigation is implemented for preview pane swaps and tab reactivation, but the production symptom remains unexplained and unreproduced; a reproduction is still needed.
