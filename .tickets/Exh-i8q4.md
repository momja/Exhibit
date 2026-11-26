---
id: Exh-i8q4
status: open
deps: []
links: [Exh-ghuv]
created: 2026-11-26T05:49:21Z
type: epic
priority: 2
assignee: Max Omdal
tags: [ui, detail, accessibility, agent-guidance]
---
# Artifact bar: bottom-pinned controls that shrink, replacing the draggable pill

Replace the artifact detail page's draggable picture-in-picture pill (Exh-ghuv) with one bar pinned to the bottom centre of the window, in the manner of iOS Safari's address bar. It shrinks to a title strip when the artifact is scrolled or used, and expands on scroll up, at either end, on tap, focus or hover. By default the artifact's frame ends above the bar (a reserved band), so the bar never covers the artifact; an edge-to-edge artifact can opt in to an overlaid bar with <meta name="exhibit-bar" content="overlay"> and reads the bar's height from var(--exhibit-bar). Agents learn the opt-in from the shared agent guidance.

Supersedes the grip/drag/corner/stash design decided in Exh-ghuv.

Implementation doc: docs/artifact-bar.md (lands with the implementation; full text under Design below). Prototype: local branch prototype/pip-bottom-bar (commits 5118728d..34282551), demoed on a throwaway instance.

## Design

## 1. Decisions

1. **One placement, every width.** The bar sits at the bottom centre. There is
   no grip, no dragging, no corners, no stash tab and no "Hide controls" item.
   Dragging existed to get the pill off the artifact. §3 solves that for every
   artifact without asking the user to do anything, and keeping drag on large
   screens would mean two placement systems and two test suites for a feature
   nobody could discover without the grip.
2. **Two states, full and compact.** Compact is a short strip showing only the
   title. The bar compacts when the artifact scrolls down **or is used** (a
   press or a key inside it), so a tool that never scrolls is not left with
   the bar over it. It expands on scroll up, at either end of the scroller,
   when tapped, on keyboard focus, and, with a mouse, on hover.
3. **A reserved band by default.** The artifact's frame ends above the bar and
   follows it as it changes size, so the bar never covers the artifact. This
   is iOS 26 Safari's own default for a page that does not opt in.
4. **Overlay is an explicit opt-in.** An artifact that declares
   `<meta name="exhibit-bar" content="overlay">` gets a full-height frame with
   the bar floating over it, and is told the bar's current height as the CSS
   variable `--exhibit-bar`. This mirrors Safari's `viewport-fit=cover` plus
   `env(safe-area-inset-bottom)`; neither can be reused, because
   `viewport-fit=cover` is common for notch reasons and `env()` is always 0
   inside our frame.
5. **Only edge-to-edge tools opt in.** A full-screen map, photo, canvas or game
   board. Never a tool with its own bottom bar, tabs or input: floating buys it
   nothing (its own bar sits behind ours either way) and any mistake in
   applying the variable covers its controls.
6. **Agents learn the opt-in from the shared guidance**
   (`internal/agentguidance/artifact_format.tmpl`), the one source for the Pi
   system prompt and the MCP instructions. Every other artifact gets the band,
   so an agent that ignores the paragraph breaks nothing.

The names `exhibit-bar`, `overlay` and `--exhibit-bar` become a contract that
stored artifacts depend on. They do not change after shipping.

### Background: how Safari does it

Researched rather than assumed. Sources at the end.

- iOS 15–18: with the floating bar expanded the viewport runs to the bottom of
  the screen and `env(safe-area-inset-bottom)` covers the bar (112px on an
  iPhone 11 Pro, Odland); collapsed, the viewport stops above it and the inset
  is 0. A fixed element clears the bar only if it uses the inset.
- iOS 26: without `viewport-fit=cover` the page stops at the safe-area
  boundary, i.e. a reserved band; with it, content runs under the glass.
  Fixed content is not drawn below the controls.
- While the bar is minimised Safari claims taps in the bottom ~44px (Frain;
  ~72px measured on an iPhone 11) to re-show it. Our compact strip has the
  same property in overlay mode, which is one reason overlay is opt-in.

## 2. Bar geometry

| | Full | Compact |
|---|---|---|
| Height | 54px phone, 46px desktop | 30px |
| Gap to window bottom | 12px | 4px |
| Width | phone: window less 12px each side; desktop: content width, centred | phone: 22vw from each side; desktop: title width |
| Band reserved / `--exhibit-bar` | bar height + 2 × 12px (78px phone) | 38px |

All of it sits above `env(safe-area-inset-bottom)` on the host page. Transitions
are 0.32s on `cubic-bezier(.32,.72,0,1)`, near zero under
`prefers-reduced-motion: reduce`, and the frame's `bottom` animates with the
same curve so the band and the bar move together.

## 3. Behaviour

### State machine

```
            scroll 'down' | 'use'
   FULL  ─────────────────────────▶  COMPACT
         ◀─────────────────────────
   scroll 'up' | 'top' | 'end', tap, focusin, mouse enter, popup open
```

- Compaction is refused while a popup (info card, ⋮ menu) is open or focus is
  inside the bar.
- A tap on the compact strip only expands it. It does not also open the info
  card under the finger (capture-phase click, `preventDefault`).
- Mouse hover expands and remembers that hover did it; leaving re-compacts
  after 500ms unless the user pressed something in the bar meanwhile.
- Compacted controls stay in the tab order (they collapse to `max-width: 0`,
  not `display: none`), and focusing one expands the bar, so keyboard users
  never meet a control they cannot see.
- Popups open upward from fixed anchors: the info card from the bar's bottom
  left (`'bl'`, aligned to the bar), the ⋮ menu from its own trigger (`'br'`).
  Escape, arrow keys and Tab keep their current menu behaviour; outside press,
  window blur and resize close them.

### Band and overlay

- On load the bar is in band mode. When the frame announces `overlay`, the
  host drops the band and starts sending heights. A later announcement (the
  frame reloads after a network approval, for instance) re-decides.
- In band mode the host sends `0`, so an artifact that opted in somewhere else
  still lays out correctly.
- Outside the detail page — exports, `/a/:id` opened top-level, `/s/:id` —
  nobody sets the variable and `var(--exhibit-bar, 0px)` falls back to 0. The
  guidance's wording must keep that fallback.

## 4. The frame–host protocol

One message family in each direction, added to the render preamble. It carries
no authority: the worst a forged message can do is resize Exhibit's own bar or
the band, which is the artifact affecting only its own presentation.

**Frame → host**, posted to `API_ORIGIN` like every other preamble message:

| Message | When |
|---|---|
| `{__avBar: 'scroll', dir: 'down'\|'up'\|'top'\|'end'}` | A scroller with ≥40px of overflow moves ≥3px, or reaches either end (4px tolerance). Sent only when `dir` changes. Captured on `window` so inner scrollers count. |
| `{__avBar: 'use'}` | `pointerdown` or `keydown` in the frame, at most every 300ms. Resets the last scroll direction so the next scroll up is reported. |
| `{__avBar: 'mode', mode: 'overlay'\|'band'}` | Once the DOM is parsed: the first `meta[name="exhibit-bar"]`, `overlay` only on an exact match. |

The host accepts these only when `e.source === frame.contentWindow` for
`.panels > iframe`, the same identity check the other bridges use.

**Host → frame**: `{__avBarHeight: n}` posted with target `'*'` (the frame's
origin is opaque, as for the shim's resync). The preamble accepts it only when
`e.origin === API_ORIGIN && e.source === window.parent`, and only a finite
number in `[0, 400]`, then sets `--exhibit-bar` on `document.documentElement`
as an inline style.

**Where the script is injected**: framed, host-present, non-widget renders —
the same condition as the capability bridges. Widgets, anonymous `/s/` shares
and top-level renders get none of it. Version views and the agent preview also
receive it; their hosts do not listen, so it is inert there. Gating it to the
detail frame would need a new render flag and is not worth one.

The prototype's message names (`__avScroll`, `__avBar: true`) are consolidated
into `__avBar` as above when this is built.

## 5. Changes by file

**Render (`internal/render/render.go`)**
- Replace `scrollReporterScript` with `barScript`, implementing §4. Splice it
  into `bridges` under `!noHost && !widget`.
- Tests (`render_test.go`): present for a framed artifact render; absent for
  widgets, `/s/:id` and top-level renders; the height receiver rejects
  non-numbers, negatives and >400; the mode reader requires the exact
  `overlay` value.

**Page script (`web/gallery/pip.js`)**: rewritten. It is a module and keeps
the `connectFloatingHeader(root)` entry point that
`floating-header-controller.js` calls.
- Keep: popups and their keyboard handling; `setCompact`; band/overlay
  switching; the message listener; hover; the disposer removing every
  listener, the timer, `.pip-compact`, `.pip-inset` and `--pip-reserve`.
- Delete: drag, throw projection, corner snapping, stash and tab, the
  `aria-live` placement announcer, and the persisted placement under
  `exhibit.pipControls`. Remove that key once on connect so stale placements
  do not linger in users' storage.
- Drop the prototype's `?bar=overlay|band` override.

**Placement (`web/gallery/pip-placement.js`)**: keep `popupPlacement` and
`POPUP_GAP_PX`/`INFO_MIN_WIDTH_PX`; delete `cornerPosition`, `stashEdgeAt`,
`pointerVelocity`, `releaseCorner`, `arrowMove`, `cornerForTab`,
`STASH_ZONE_PX`, `PROJECTION_MS` and the velocity constants. Trim
`pip-placement.test.mjs` to match. If only bottom corners remain,
`popupPlacement` can drop its top branches.

**Markup (`internal/api/templates/components.tmpl`)**: remove `#pip-grip`,
`#pip-hide` and its separator, `#pip-tab` and `#pip-announce`. Update the
partial's comments, and the "draggable" comment on `floatingHeaderView` in
`internal/api/appheader.go`.

**Styles**
- `web/gallery/components.css`: bottom-centre placement, compact state and the
  phone variant as prototyped. Delete the drag, stash and tab rules
  (`pip-dragging`, `pip-tab` and friends: about 24 lines).
- `web/gallery/detail.css`: `body.pip-inset .panels{bottom:…}` with its
  transition. The notices rule keyed on `data-pip-corner^=t` goes; notices
  always sit at the top now, and `data-pip-corner` itself can be removed.
- Overlay mode wants a more frosted bar (the full bar is 88% opaque and reads
  as solid over content). Check on a device first: headless WebKit does not
  draw `backdrop-filter` over the cross-origin frame.

**Agent guidance (`internal/agentguidance/artifact_format.tmpl`)**: add, after
the colour-scheme paragraph:

> Exhibit shows its controls in a bar along the bottom of the window. By
> default the artifact's page ends above that bar, so nothing is covered and
> you need do nothing. Only a tool whose content is meant to run edge to edge
> with nothing to tap along the bottom — a full-screen map, photo or canvas —
> should opt in, with `<meta name="exhibit-bar" content="overlay">`; never opt
> in when the tool has its own bottom bar, tabs or input. If you opt in, pad
> the end of scrolling content and keep any floating control clear of the bar
> with `var(--exhibit-bar, 0px)`, which Exhibit keeps set to the bar's current
> height.

Pin `exhibit-bar`, `overlay` and `--exhibit-bar` in `agentguidance_test.go`,
and assert the same strings in the render test, so prompt and implementation
cannot drift apart.

**Docs**
- `architecture.md` §3.5 "Detail page": replace the floating, draggable pill
  paragraph with a summary of this design and a link here. §3.2 "Render
  preamble": add the bar script to the list.
- `security.md` §4 preamble taxonomy: the bar messages, why they carry no
  authority, and that the band is host-owned space the artifact cannot draw
  into.
- `technical_stack.md` §4: add the bar script to the preamble list.
- `ui-components.md`: the floating-header entry loses dragging and stashing.
- `widgets.md` is unaffected: widgets get no bar script.

## 6. Accessibility

Shrinking is presentation only: every control stays in the accessibility tree
and the tab order, and any interaction with the bar brings it back. The
following must hold before this ships. Items marked *measured* were checked
against the prototype in WebKit; the rest are design requirements.

1. **Activation is never swallowed for assistive technology.** *Measured:* a
   click delivered to the compact title without focus (as some screen-reader
   and voice-control activations are) only expands the bar and the info card
   stays shut, so the user activates twice without being told why. Intercept a
   click only when a `pointerdown` on the bar (touch, mouse or pen) preceded
   it; every other click expands the bar *and* activates the control.
   Keyboard activation already works: focusing the title expands the bar and
   Enter opens the card (*measured*).
2. **Focus not obscured (WCAG 2.4.11).** The band satisfies this outright:
   the bar is never over the artifact. In overlay mode the preamble also sets
   `scroll-padding-bottom: var(--exhibit-bar, 0px)` on the root element, so a
   control focused by keyboard is scrolled clear of the bar, and a key press
   inside the artifact compacts the bar anyway. Inner scrollers are the
   artifact's job; the guidance already tells it to pad them.
3. **Contrast (1.4.3).** In band mode the bar sits on the page ground and the
   palette contrast test covers it. In overlay mode the compact strip is 72%
   opaque over arbitrary artifact pixels, and text visibly shows through the
   title in the prototype. Overlay mode uses a background opaque enough that
   the title meets 4.5:1 over any content (in practice solid, or ≥95% with
   blur), rather than relying on `backdrop-filter`.
4. **Text size and zoom (1.4.4, 1.4.10).** Heights are fixed px today (54, 46,
   30), and the band's sizes are a constant (38px) and a measurement taken
   once on connect. Size the bar from `rem` with a `min-height`, and take the
   band and `--exhibit-bar` from a `ResizeObserver` on the bar, so larger text
   grows the bar and the band follows it. At 400% zoom on a 1280×1024 window
   the viewport is 320×256 and the full band would take 30% of it: below a
   short-viewport threshold (around 450px tall) start compact. Device
   verification at maximum Dynamic Type belongs to the QA pass in
   [Exh-bs9j](/home/maxomdal/Exhibit/.tickets/Exh-bs9j.md).
5. **Motion (`prefers-reduced-motion`).** Transitions are already near zero.
   The frame resize then becomes a jump on every state change; under reduced
   motion keep the band at its full height and let only the bar shrink, so the
   artifact never resizes (see open question 1).
6. **Semantics.** The bar is `role="toolbar"`, which promises arrow-key
   movement between its controls with a single Tab stop. Neither the shipped
   pill nor the prototype implements that; this is a pre-existing gap. Either
   implement roving focus or drop the role for a labelled group inside the
   header landmark. Do not announce compact/full changes: they change no
   content or capability.
7. **Hover (1.4.13).** Content revealed by hover stays while the pointer is
   over it and goes 500ms after it leaves, which also forgives a slightly
   unsteady pointer. Hover is never the only way in: tap and focus expand too.
8. **Target size (2.5.8).** The compact strip is 30px tall, above the 24px AA
   minimum and below Apple's 44pt guideline. On iPhone in a browser tab it
   also sits near Safari's own bottom tap zone, which is a further reason to
   keep it in the band.
9. **Screen readers on iOS.** Whether moving the VoiceOver cursor onto a
   compact control moves DOM focus (and so expands the bar) is unverified.
   Test with VoiceOver on a device; if it does not, item 1 still guarantees
   the control works when activated.

## 7. Tests

- **Node (`pip.test.mjs`)**, through `testdom.mjs`'s postMessage boundary:
  scroll `down` and `use` compact the bar; `up`, `top` and `end` expand it;
  messages from any other source are ignored; no compaction with a popup open
  or focus inside; a tap on the strip expands without opening the info card;
  hover expand and leave re-compact; `mode: 'overlay'` removes the band and
  heights follow state; the disposer leaves no classes or listeners behind.
- **Chromium (`appheader_browser_test.go`)**: replace drag and stash coverage
  with the bar at the bottom centre at 1280×800 and 390×844, a real scroll in
  a real render frame compacting it, the band resizing the frame, and the
  overlay variable reaching the frame.
- **Accessibility** (§6): a focus-less click on the compact title expands the
  bar and opens the card; Tab into the compact bar expands it; a pointer tap
  on the strip only expands it; in overlay mode a keyboard-focused control at
  the bottom of the artifact scrolls clear of the bar; with a 200% root font
  size the band matches the bar's measured height in both states.
- **Template (`detail_controls_test.go`)**: owner and recipient control sets
  are unchanged apart from the removed grip and hide item.
- **WebKit**: the Playwright scripts used for the prototype (scroll,
  sketchpad, Field notes, map) become an e2e check if the `e2e` suite takes
  WebKit; otherwise they are run by hand for the PR, with screenshots under
  `/tmp/exhibit-review/`.

## 8. Open questions

1. **Frame resizing in band mode.** Each state change resizes the frame and
   fires `resize` inside the artifact, as Safari does to pages. A canvas that
   clears on resize loses its drawing. Options if this bites: snap the band
   only after scrolling stops, or keep the band at the full height and let
   only the bar shrink (no resize, 40px less room).
2. **End-of-scroll stability.** Reaching the end expands the bar, which
   shortens the frame. Reasoned to settle expanded, not observed. Test on a
   device.
3. **Stacking with Safari's own bar.** In a browser tab on iPhone our bar sits
   above Safari's: two bars at the bottom. In a home-screen web app there is
   only ours. Decide whether the compact strip should be smaller in a browser
   tab (`display-mode: browser`).
4. **Agent compliance.** Run the built-in agent with real inference (the
   dev-instance skill) on prompts that should opt in (a full-screen map) and
   ones that should not (a calculator, a chat, a tabbed app). Pass: the tag
   appears only on the first kind, with the variable applied.
5. **The pill as an agent tool palette.** Exh-ghuv imagined the pill growing
   into one. A bottom bar can still do that, but a palette taller than the
   bar would need the band to grow with it. Out of scope here.

## 9. Rollout

No migration and no server state. Saved placements in `localStorage` are
ignored and cleared. Artifacts written before the guidance change carry no tag
and get the band. The feature needs no flag: band mode is strictly safer than
today's overlay pill, which already covers artifact content.

## Sources

- [Johannes Odland, CSSWG #4329: Safari 15 viewport and safe-area measurements](https://lists.w3.org/Archives/Public/public-css-archive/2021Jul/0027.html)
- [Pavel Larionov: Safari 26 Liquid Glass, toolbar tinting and viewport bugs](https://1ar.io/updates/safari-26-liquid-glass-web/)
- [Ben Frain: The iOS Safari menu bar is hostile to web apps](https://benfrain.com/?p=3536)
- [web.dev: the large, small and dynamic viewport units](https://web.dev/blog/viewport-units)
- [Apple forums: iOS 26 Safari will not render fixed content below the controls](https://developer.apple.com/forums/thread/800798)
- [Apple forums: sticky `bottom: 0` does not follow the toolbar](https://developer.apple.com/forums/thread/801028)

## Acceptance Criteria

Everything in the Design section's Decisions (§1), Behaviour (§3), protocol (§4) and Accessibility (§6) holds, verified by the tests in §7 (Node page-script, Chromium browser, template and render tests) plus WebKit runs at 390x844 and 1280x800 with screenshots saved outside the checkout. docs/artifact-bar.md is committed, and architecture.md, security.md, technical_stack.md and ui-components.md are updated as §5 lists. The agent guidance paragraph ships with its names pinned by tests. A real-inference agent check (open question 4) is recorded on the epic. Grip, drag, stash, hide-controls and placement persistence are removed.

