# Brand refresh design references

Design inputs for the brand refresh epic, [Exh-od5a](../../.tickets/Exh-od5a.md).
They come from throwaway mocks laid over the real UI. **None of this is
production code.** Use it to see what was decided, then build it the
repo's way: tokens in `tokens.css`, colours only from tokens, templates
and Stimulus as described in `docs/`.

## What is here, and which story uses it

| Path | What it is | Used by |
|---|---|---|
| `paintings/hero-day.png`, `hero-dusk.png` | Hero paintings (1600x900): poppies, Mondrian grid on the screen. Day for light mode, dusk for dark. | [Exh-90b2](../../.tickets/Exh-90b2.md) until [Exh-qhvv](../../.tickets/Exh-qhvv.md) replaces them |
| `layers/{day,dusk}-{sky,clouds,hill}.webp` | Paintings cut into three layers by `tools/split.py`. The sky is 1600x1420, extended 520px upward. Expect halos; the real layers come from Exh-qhvv. | [Exh-vsw0](../../.tickets/Exh-vsw0.md) |
| `prototypes/descend.html` | Working descent animation over the layers. Open the file in a browser; Day/Dusk/Replay in the top bar. | Exh-vsw0 |
| `logo/logo-pigments.svg` | Current logo recoloured in painting pigments, hanger still attached. | [Exh-d5ku](../../.tickets/Exh-d5ku.md) after [Exh-bv8l](../../.tickets/Exh-bv8l.md) |
| `css/pal.css` | Palette on the real token names, behind `html[data-pal]`. Use the **ink** accent and the slate values; ignore sky, meadow and the wear/"trodden" rules (out of scope). | [Exh-plvh](../../.tickets/Exh-plvh.md), [Exh-z8i2](../../.tickets/Exh-z8i2.md) |
| `css/soft-round.css` | The chosen card style: rounded, full-bleed tile, soft shadow, no outlines. | Exh-z8i2, [Exh-odyv](../../.tickets/Exh-odyv.md) |
| `css/frame.css` | Rejected hard-outline layer. Only its Fraunces title setting (`SOFT 100, WONK 0, opsz 72`) is kept. | [Exh-wjqh](../../.tickets/Exh-wjqh.md) |
| `css/login.css`, `login-round.css` | Sign-in page mock; `login-round.css` holds the chosen rounded box and ink button. | Exh-90b2 |
| `references/*.webp` | Screenshots of the chosen looks (see below). | all |
| `tools/` | Playwright and image scripts used for the mocks. They expect the old `/tmp/exhibit-mock` server and are kept for the record, not for reuse. | none |
| `prototypes/story-graph.html` | Story dependency graph. | planning |

## References

- `gallery-light`, `gallery-dark`: palette, grainy tiles and rounded cards on Starred.
- `sign-in-light`, `sign-in-dark`: the chosen sign-in page.
- `sign-in-phone-*-early`: phone layout concept from **before** the ink button and
  rounded box were chosen. Use it for layout, not styling.
- `descent-frames-*`: frames from the descent animation.
- `logo-current-hanger`: today's logo, showing the hanger problems Exh-bv8l fixes.
- `logo-current-vs-pigments`: today's colours beside the pigments.

## Where the mocks differ from the tickets

The tickets win where they disagree with these files:

- The prototype fades the sign-in form in after about 2.3s. The form must be
  present and usable from the first frame; only the scenery moves (Exh-vsw0).
- The mocks load fonts from Google Fonts with every axis. Load only the
  axes in use (Exh-wjqh).
- The header logo still has its hanger. The flat mark replaces it everywhere
  except the 404 page (Exh-bv8l).
