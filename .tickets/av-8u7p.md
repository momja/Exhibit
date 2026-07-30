---
id: av-8u7p
status: closed
deps: []
links: []
created: 2026-07-07T05:39:46Z
type: task
priority: 2
assignee: Max Omdal
---
# Add a hover-revealed Edit control to the gallery card

The original story asked to *replace* the card's "Details" link with an edit
button. Half of it is already done: the Details link was removed earlier as
redundant (it navigated to the same page as the title click) — see the comment
in `internal/api/templates/gallery.tmpl`. What remains is adding the edit
control.

Add a control on each gallery card that navigates to
`/artifacts/{{artifact_id}}/edit`, revealed on card hover / keyboard focus.

## Design

Chosen from five mockups (frame `e-chosen`): a **labelled pencil chip in the
card's title row**, right-aligned, as a flex sibling of the title.

- **Labelled, not icon-only.** The chip reads `<pencil> Edit`. A bare glyph is
  targetable by voice control via its accessible name, but the name is not
  discoverable because nothing on screen states it. A visible label is
  speakable.
- **Contrast.** Chip text/icon at `--color-ink-soft` (#333) is 12.63:1 on the
  card. A muted #888 glyph measures 3.54:1 — it clears the 3:1 floor for
  non-text UI (WCAG 1.4.11) but would be carrying 100% of the control's meaning
  at the bottom of the scale.
- **Target size.** 58x24 CSS px, clearing WCAG 2.5.8's 24x24 minimum with
  margin.
- **In flow, not absolutely positioned.** As a flex sibling the chip's hit box
  never overlaps the title link's. Corner-positioned variants put the chip on
  top of the title anchor, which stays clickable underneath.
- **No layout shift.** The chip occupies its space at rest and only its opacity
  animates — the same approach the tag pills already use in `index.css`.
- **Touch.** Hover does not exist on touch, so the chip stays visible under
  `@media (hover:none)`. Cross-device use is a core product flow (spec 8.3); a
  hover-only control is unreachable on iPhone.

Accepted cost: the chip reserves ~66px beside the title permanently, so a
medium-length title wraps to two lines. That is the price of the visible label,
and it applies to any labelled placement in this row.

## Acceptance

- Each gallery card carries an Edit control linking to `/artifacts/:id/edit`.
- Hidden at rest; revealed on card hover and on keyboard focus of the control.
- Visible unconditionally on coarse pointers.
- Activating it does not also trigger the card's `data-href` navigation.
- Phosphor `ph-pencil-simple`, per the project icon convention.
