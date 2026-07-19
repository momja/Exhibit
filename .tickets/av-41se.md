---
id: av-41se
status: closed
deps: [av-isb3]
links: [av-isb3, av-p0a1, av-hwx2]
created: 2026-07-18T15:15:41Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-jafp
tags: [ui, gallery]
---
# Capability posture detail popover (card + viewer)

A detail popover anchored to the capability cluster (the av-isb3 badge) on both the gallery card and the artifact viewer. Turns the at-a-glance cluster into the exact, transparent breakdown — the "receipt" for what the user approved at ingest (spec 6.2).

## Mockups

Paper file 01KWQV7Y6G82HDZJ5CKPJTSJ7Y, artboard "av-isb3 — B · Capability row + tooltip":
- The "ON HOVER OR FOCUS" section shows the full-set popover (Network + Downloads + Clipboard) and the Sandboxed popover.
- The "SPEC NOTES" panel gives the trigger/colour/accessibility rules and the glyph -> stored-field mapping.

## Design

Popover header "Sandbox posture". One row per active capability — fixed icon slot + label + one-line meaning:
- Network — lists the approved origins (monospace) from NetworkAllowlist.
- Downloads — "Can save files to your device" (shown when downloads_approved).
- Clipboard — "Can read and write your clipboard" (shown when clipboard_approved).

Footer link "Manage in allowlist settings ->" navigates to the Edit page security section (av-p0a1). The no-grants (Sandboxed) state shows a single "Fully contained" reassurance row. Neutral styling — no verdict colour. Origins rendered via html/template (contextual auto-escaping).

Trigger: opens on hover, keyboard focus, AND tap; dismisses on blur / mouse-leave / Esc — never hover-only. The cluster is a focusable control; the popover is its aria-describedby. On the public share view (no owner session) the popover renders WITHOUT the Manage link.

## Acceptance Criteria

- Hover, keyboard focus, and tap all open the popover; blur/Esc close it; keyboard-only users can reach and read every row.
- Network row lists exactly the approved origins; the Downloads/Clipboard rows appear iff their flags are set.
- Footer "Manage" link navigates to the Edit page (av-p0a1); it is absent on the no-auth share view.
- Neutral treatment (no green/amber), matching the mockup.
- Singular/plural origin phrasing ("1 origin" / "N origins") is correct here.
