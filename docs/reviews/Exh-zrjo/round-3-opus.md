# Exh-zrjo UI council: round 3 (Claude Opus), frozen build 006b14ef

**Result: passes, 8/8/8/8/8/8.** This is the **first** passing round from this reviewer. It starts a streak but does not complete one: a fresh round-4 review has to confirm it independently.

## Method

- **Read:**
  - the brief, both round-1 reports, both round-2 reports, `round-2-changes.md`, `round-3-changes.md` and `validation.md`
  - `git diff baf7a0c1 006b14ef`. The only app change is `web/gallery/new.js`: the progress text now moves after payload validation. Everything else in the diff is the E2E spec, docs and recaptured screenshots.
- **Screenshots inspected (round 3):**
  - `add-error-320-light` and `add-error-320-dark`, compared with `round-2/add-error-320-light`
  - `gallery-1280-light`
  - `editor-390-light`
  - `history-320-dark`
  - `viewer-320-dark`
  - `settings-390-light`
- **Observed live:** in my own T3 tab (`tab_6_…`), signed in as admin, at 320×800. I wrapped `window.fetch` to count non-GET requests and submitted each invalid state on `/new`:
  - blank Agent
  - blank Paste
  - a switch to URL
  - blank URL
  - `ftp://example.com`
  - `not a url`
- **Regression checks (observed):**
  - I loaded these pages in same-origin 320px frames and measured document width, h1 and visible controls under 32px: `/`, the filter miss, the search miss, `/shared`, `/shared-by-me`, the RunLog editor and `/settings`.
  - I opened the editor's Tags panel and measured the chip controls.
- **Untested:**
  - a successful create
  - the pending-lock/double-submit path and refusal recovery (the parent reports the E2E suite covers them; I did not rerun it)
  - share panel internals
  - the duplicate submit
  - real touch hardware
  - screen-reader speech
  - true `pointer:coarse` emulation (my resize changes width only)

  Nothing was created, saved or shared.

## B1 verification: fixed (observed)

Results for every invalid state:

| State | `#status` | Field error | Focus | Non-GET requests | Submit enabled |
|---|---|---|---|---:|---|
| Agent, blank | empty | "Describe what you want the tool to do." | `#agent-description` | 0 | yes |
| Paste, blank | empty | "Paste an artifact first." | the "HTML source" CodeMirror (`aria-invalid` on both `#body` and `.cm-content`) | 0 | yes |
| Switch to URL | empty | errors cleared | not checked | 0 | yes |
| URL, blank | empty | "Enter a URL first." | `#url-input` | 0 | yes |
| URL, `ftp://…` | empty | "Enter a full http:// or https:// page URL." | `#url-input` | 0 | yes |
| URL, `not a url` | empty | same as `ftp://` | `#url-input` | 0 | yes |

`#url-input` has `aria-describedby="url-input-error"`, and `#status` keeps `role=status`.

**Screenshots:** the round-3 `add-error-320` light and dark captures show only the red field border and the inline error. Round 2 showed a "Uploading…" panel in the same place. In source, the status text is set immediately before `lockIngest(true)`, so it shows only when a request is actually about to be sent.

## Scores

| Category | Score | Evidence |
|---|---:|---|
| Visual hierarchy | **8** | **Unchanged from round 2 (observed):** the "Your library" h1 and intro, the primary "+ Add artifact" button, bold card titles, the "Editing artifact / Run Log / All changes saved" heading block, and the titled permission summary. **Minor and retained:** `/new` and Settings repeat context in the header title and the hero heading ("Add artifact"/"Three ways in.", "Settings"/"Your account"). |
| Component consistency | **8** | The shared header, buttons, card surfaces, empty-state pattern and Phosphor icons are consistent in both palettes. The floating viewer pill is a legitimate variant. **Minor and retained:** on phones `/new` and `/settings` show a bare ←, where the editor shows "← Library". Only some Settings headings have icons. |
| Navigation / discoverability | **8** | **Observed:** every space page renders its h1 at 320px. Card action triggers are visible. The editor keeps `return_to`. The viewer pill names the tool at 320px. Empty states offer recovery. Nothing changed here. |
| Workflow feedback | **8** | **B1 fixed (observed, table above):** a validation failure now produces exactly one message, tied to its field, with focus moved, no request and a retryable form. Switching mode clears stale feedback. Pending and refusal recovery rest on the parent's E2E evidence and my reading of the source; I did not observe them myself this round. |
| Accessibility | **8** | Named editors, `aria-invalid` and `aria-describedby` on errors, and a live status region that no longer announces false progress. Relative type and the 44px phone viewer controls are unchanged from round 2. **Minor (observed, pre-existing, also present in round 2):** at 320px the editor's tag-chip pencil and × buttons are 20×20 with centres about 25px apart. That satisfies WCAG 2.5.8 through its spacing exception, but it is below the 44px used elsewhere on phones. |
| Responsive behavior | **8** | **Observed:** at 320px, no horizontal overflow on the library, filter miss, search miss, shared, shared-by-me, editor and Settings pages (scroll width equal to the viewport minus the scrollbar). History rows wrap with 44px View/Restore buttons. The viewer pill fits at 320px. Dark mode is coherent. **Minor and retained:** the "2 saved versions" count runs on from the paragraph before it, and "current" is plain text. |

## Blockers

None.

## Non-blocking refinements (carried forward, plus one new)

1. **Tag-chip targets:** in `.tag-pill-btn` (`partials.tmpl` tag pills on the editor), add a `(pointer:coarse),(max-width:767px)` rule giving at least 32px, ideally 44px, matching the phone convention. Color swatches (22px) in the tag edit dialog could get the same treatment.
2. Remove the duplicated header/hero context on `/new` and Settings.
3. Use one phone back pattern: "← Library" everywhere, or a bare arrow everywhere.
4. In the versions panel, add space before "N saved versions" and give "current" a badge treatment.
5. Give section-heading icons in Settings either to every heading or to none.

## Verdict

The only blocker from round 2 is fixed, I verified it directly, and the representative surfaces I checked show no regressions. All six categories score **8**. This is the first passing round; the next fresh review must confirm it before the council can complete.
