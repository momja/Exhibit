# Round 4: independent Opus confirmation

**Frozen candidate:** `006b14ef6828e7623d1057fc1ae9085d51ac5c7a`. I checked that HEAD matches it and that no application files have changed.

**Context read:** the [brief](brief.md), both reports for rounds 1–3, and the [round-2](round-2-changes.md) and [round-3](round-3-changes.md) responses.

**Result: all six categories score 8, and there are no blockers.** I am not carrying round 3's scores forward. Each score below is based on this round's evidence.

## Evidence and limits

### Saved screenshots (observed)
I looked at these round-3 captures:
- `gallery-1280-light`
- `editor-390-dark`
- `viewer-320-dark`
- `history-320-light`
- `filters-empty-390-dark`
- `add-error-320-dark`
- `settings-390-light`

I relied on the manifest for the other 29 states.

### Live checks in my own tab only (observed)
All checks ran in `tab_7_0d23e38c-…`. Nothing was saved, created, granted or deleted.

**`/new` at 320px.** I wrapped `fetch` to record any request that was not a GET, then submitted each method in its invalid state. Every case left `#status` empty, sent no requests (`posts: []`) and kept the submit button enabled.

| Method | Field error shown | Focus moved to |
|---|---|---|
| Agent, blank | `agent-description-error`: "Describe what you want the tool to do." | `#agent-description` |
| Paste, blank | `body-error`: "Paste an artifact first." | `.cm-content` |
| URL, blank | `url-input-error`: "Enter a URL first." | `#url-input` |
| URL, `not a url` | "Enter a full http:// or https:// page URL." | `#url-input` |

- Switching method afterwards cleared all errors (0 visible).
- The header back link is named "Library" and measures 44×44.
- Document width equals the viewport.

**320px regression sweep.** I loaded each page in a same-origin 320px frame:
- `/`
- `/?tag=Health&tag=Reading`
- `/?q=not-in-this-library`
- `/shared`
- `/shared-by-me`
- `/settings`
- RunLog `/edit`

Results:
- None of these pages overflows horizontally (scroll width = client width).
- Each page has the right h1.
- The filter-miss and search-miss empty states explain the situation and offer recovery ("Clear filters" / "Clear").
- The only interactive elements under 24px are on the editor (details below).

**Viewer at 320px.** Library, Move, the named "Run Log" posture button, Share and More actions all measure ≥44px high. Document width is 320px.

### Source (inspected)
- `web/gallery/components.css:228-229`: the `.details-panel` summary rules.

### Not tested
- A successful ingest, save or duplicate.
- The pending lock and refusal recovery. These rest on the supplied 9-test E2E run plus round-3 source review.
- Keyboard behaviour of the viewer menu. Both round-3 reviewers checked it, and I did not repeat it.
- Share panel internals.
- Real touch hardware, true `pointer:coarse`, and screen-reader speech.
- I did not rerun any suites. The full Go short run was still listed as "running" in `validation.md` when I read it.

## Scores

| Category | /10 | Evidence |
|---|---:|---|
| Visual hierarchy | **8** | **Desktop library:** a clear "Your library" h1 and intro, the labelled primary "+ Add artifact", and bold card names with quiet menus. **Editor:** the "Editing artifact / Run Log / All changes saved" block reads well in dark mode at 390px. **Retained minor:** `/new` and Settings repeat their context in both header and hero ("Add artifact"/"Three ways in.", "Settings"/"Your account"). |
| Component consistency | **8** | Shared header, bordered controls, card surfaces, a shared empty-state pattern and Phosphor icons, all coherent in both light and dark. The floating viewer pill is a valid variant with 44px controls. **Retained minor:** on phones, `/new` and Settings show a bare ← while the editor shows "← Library". Both are named "Library" and 44px, so this is visual inconsistency only. Only some Settings section headings have icons. |
| Navigation/discoverability | **8** | **Observed:** h1s are present on every space at 320px, and the phone space tab bar shows full labels. Empty states offer one-click recovery. Back links are named and lead to Library. The viewer identifies the tool by name. |
| Workflow feedback | **8** | **Former B1 is confirmed fixed live** (table above): one accurate field error, no false progress, no request, and the form stays retryable. Stale errors clear when the method changes. "All changes saved" and quiet clean Save are unchanged. Pending and refusal recovery are supplied/source evidence only, not observed by me. |
| Accessibility | **8** | Error fields are tied to their messages (`aria-describedby`) and focused, the CodeMirror field is named, and `#status` is a live region that no longer announces false progress. Phone viewer and header controls are 44px. **Minor (observed, pre-existing):** the editor's tag-chip edit/remove buttons are 20×20. **Minor (new observation, pre-existing since before round 1):** each `.details-panel > summary` hit area is only about 15px tall inside a bordered row that looks about 40px tall, so a tap on the row's padding does nothing. Both pass WCAG 2.5.8 through its spacing exception and do not stop anyone using the page, so neither is a blocker. |
| Responsive behavior | **8** | No overflow on any of the seven core routes or the viewer at 320px. History rows wrap, with View/Restore as full buttons. Filter chips, the empty-state card and the bottom space tabs fit at 390px dark. Settings cards stack cleanly. **Retained minor:** the "2 saved versions" count sits in the paragraph text without separation, and "current" is plain text. |

## Blockers

None.

## Non-blocking refinements

1. **New, pre-existing:** make the whole `.details-panel` row the tap target (`components.css:228-229`). Move the panel's vertical padding onto `> summary`, for example `.details-panel{padding:0 var(--space-5)}` plus `.details-panel>summary{padding:var(--space-3) 0;min-height:44px}` under the phone/coarse query, and adjust `.panel-body` spacing to match.
2. **Tag chips:** in `.tag-pill-btn`, use ≥32px targets on phones. The tag dialog's 22px colour swatches could get the same treatment.
3. **Context repetition:** remove the duplicated header/hero context on `/new` and Settings.
4. **Back links:** use one phone back pattern everywhere.
5. **Versions panel:** separate the count from the paragraph, and give "current" a badge.
6. **Phone Save:** the Save button could carry a visible text label.
7. **Settings icons:** use heading icons on every Settings heading, or on none.

None of these makes a category fall short of "polished, usable, no substantial unresolved problem".

## Verdict

This fresh review confirms round 3. All six categories score 8, with no blockers and no regressions in the core workflows I checked.

This is **Opus's second consecutive passing round**. The council completes only if:
- Sol's independent round 4 also scores every category ≥8, and
- the required regression checks pass, including the full Go short suite, which was still running when I read the validation log.

This report alone does not complete the council.
