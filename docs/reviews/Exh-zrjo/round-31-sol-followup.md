# Round 31 — independent Sol confirmation review

2026-11-05. Production application `42c94eb1cb12a74108885cc1cbb7aaf7d11f5721`, assets `aa7a00afd7a2`. Validation-only baseline `c1156ef15c7f5caa282b1068214492bce42018e1`. Stack base `ab0135c902559d5912deec420947d6ab3cbdbcf6`.

**Independent Sol verdict: PASS. All fourteen scores meet 9.** These are fresh judgments of the weakest material surfaces sampled in this round, under the excellent/intentionally composed bar. Prior ratings, the target, and passing tests did not supply these scores. This report alone does not establish a joint result or complete the two-round council requirement. The current Round 31 peer outcome remains unknown to me.

| Category | Light | Dark |
| --- | ---: | ---: |
| Typography | 9 | 9 |
| Composition and spacing | 9 | 9 |
| Color harmony | 9 | 9 |
| Contrast and emphasis | 9 | 9 |
| Component visual consistency | 9 | 9 |
| Responsive visual polish | 9 | 9 |
| Save discoverability and feedback | 9 | 9 |

## Identity, independence and method

I read both Round 30 reports in full, the visual/nine briefs, the supplied cumulative historical archive of findings/responses/unresolved judgments, all available visual change notes through Round 31, `followup-validation.md`, `round-31-validation-baseline.json`, `round-31-filter-fixture.json`, and `round-30-runtime-identity.json`. Earlier history was considered as history; I do not claim to have reread every old report line by line. I did not read the current Round 31 peer report/status, parent score/summary/resume/rating-verification/timer/checkpoint, nine aggregates, or ticket.

The complete production-to-validation-baseline diff is **not empty**. I checked the entire named scope and then read the actual two-file diff: `internal/api/assets_test.go` updates its documented/expected icon markup from the old check to the aria-hidden floppy; `internal/api/galleryspaces_test.go` updates the Shared helper expectation. The assertions remain. These are exactly the two disclosed files, with three insertions and three deletions, including the comment change. I did not compare against production and then silently exclude them.

At entry and exit, the strict comparison of **all** `internal web e2e cmd scripts Makefile go.mod go.sum` against **c1156ef**, plus scoped porcelain status, was empty, without exclusions. Final HEAD was `948317e6ada8863b6299fcd9a1c31cbbb8e4784a`; that evidence commit is not treated as a new UI build. Independently read SHA-256 values for `bin/server`, `/proc/221361/exe` and `/proc/221370/exe` all matched:

`9d4943dc5055211d8076931d3ca474888e41797fee72ebccad665a86426e74f0`

Both fixtures served `/assets/v/aa7a00afd7a2/`; the final editor reconfirmed that version. I found no runtime change invalidating this candidate. I ran no build, test suite, commit, app edit or subdelegation.

I called `preview_status` and opened only my own two background tabs with `reuseExistingTab:false, open:false`: `tab_29_0d23e38c-d413-4e76-b0eb-59687a206343` for main `18280` in `default`, and `tab_2a_0d23e38c-d413-4e76-b0eb-59687a206343` for empty `18380` in `incognito`. Every subsequent browser call supplied its exact tab ID. The archived scratchadmin login was rejected on empty; the operative admin/changeme credentials succeeded in incognito. Login frames were excluded from the UI ratings, and the main session was not overwritten.

These were actual resized tab views, with images inspected after font/layout settling, not hidden frames or metadata-only certification. At **each** required width/scheme pair on both tabs, I called `preview_set_appearance` and explicitly checked `matchMedia('(prefers-color-scheme: dark)').matches`: false for light, true for dark. No root-class simulation was used.

## Rendered coverage and the new filter-only specimen

| Actual viewport, each in light and dark | Fresh live coverage |
| --- | --- |
| 1280×800 | All four populated destinations; all five empty contexts; actual filter reset; editor title clean/dirty/restored, Tags creation and Edit tag, deep sticky Saved/Save and Versions |
| 768×900 | The same destination, five-context empty, filter-reset and focused editor matrix |
| 390×844 | The same matrix, including actual phone Shared/Activity and complete Edit tag footer |
| 320×740 | The same matrix; additional real Search typing/submission/reset, incoming hit, native editor tools, sheets and viewer overlays |

The four populated routes were `/`, `/search`, `/shared` and `/activity`. The five empty contexts were `/search` welcome, `/` Starred, `/shared`, `/search?q=not-in-this-library`, and **`/search?tag=Review%20empty`** on the secondary fixture. Each of these five contexts was actually rendered at every listed width in both schemes. Document horizontal overflow measured zero throughout this matrix.

The new valid filter-only route genuinely returned **“No artifacts match your filters.”** It showed the named **Review empty** chip, regular muted **“Remove a filter to see more of your library.”** helper, filter icon and **Clear filters** action. I clicked the actual Clear filters control at **all eight** pairs. It returned to `/search`, removed the chip, emptied the query and restored **Welcome to Exhibit** and **Create your first artifact**, with the Search title, form and application header still present. This closes the prior live filter-only gap for this review; I did not substitute a corpus filename, unknown invalid filter or combined query/filter miss for it.

The fixture JSON attributes the one unassigned tag to parent setup after Round 30; I did not create or attach it. Main Run Log was observed with Health, Reading, Test and Utilities. I make no claim about the actor/timing of earlier association variation, and no immutable-fixture claim.

The five contexts now read as one component family: short 600-weight headline, regular muted helper, deliberate separation and 44px action where appropriate. Starred still offers **Browse your artifacts → `/search`**, rather than sharing instructions or a self-link. Welcome retains its illustration and **Create your first artifact → `/new`**. Shared uses neutral sharing guidance without an invented CTA. At 320, wrapped miss headings/helpers and the welcome action remain composed within the available column. The previous lost-CTA and bold-instruction-block defects did not recur.

Populated Shared visibly combines the incoming packing list/owner attribution and owned Run Log. Starred contains its two selected artifacts. Search exposes the accessible collection, with a real page-level field at every width. Activity separates the running-agent empty state, explicit unavailable detailed audit, and recent version changes. The 768 rail leaves readable two-column cards, and the four short phone destination labels share a stable baseline and active treatment. The Activity availability badge moves to its own line at 320 without a collision.

Additional Search observations include `q=run` at 1280 both schemes, incoming `q=packing` at 1280 dark and 320 both, and real field typing/Enter at 320 both for a miss and the incoming hit. Actual Clear search returned the populated miss to normal `/search` and its cards in both schemes. The integrated clear and owner attribution remain readable. Arbitrary artifact/widget body design was excluded.

## Exact tag pixels, labels and actions

I rendered **Tags creation and the actual Edit tag Health sheet at all eight width/scheme pairs**, with separate phone scroll positions for attached labels and the complete creation controls. Initial desktop views that put labels under the sticky header were replaced with fully visible resting views. A hovered utility was not used to judge the resting palette.

The selected preset check is visibly separated from the outer outline. I saw no recurrence of the reported outer-corner fringe. Measured geometry matches the rendered treatment: **8px outer radius, 1px border, 16×16 badge, 3px top/right inset, 4px badge radius**. Dots are **18px desktop / 22px phone**. The compact desktop creation badge still overlays part of its dot, but its contour is contained and its check is clear; it reads as a selection overlay rather than the original broken corner. The larger phone and Edit sheet cells have more breathing room.

Attached labels are **28px capsules**, with separate **32×32 desktop / 44×44 phone** Edit/Remove slots. At 1280, Health/Reading/Test/Utilities label widths measured approximately **70.4/79.7/54.6/76.2px**; the action slots do not inflate those capsules. The phone grouping wraps whole label/action units. The 390 layout's mix of one- and two-item rows follows their intrinsic widths, without padding a pill into a large empty container. The Edit sheet keeps aligned fields, named floppy Save, quiet Delete and an intentional full-width phone primary/secondary/destructive stack. I opened and dismissed it unchanged; no preset, tag or association was saved.

## Save clarity and feedback

At all eight pairs I typed **“Run Log — local review”**, observed **Unsaved** and active Save, restored exactly **“Run Log”**, and observed **Saved** with disabled Save. No persistent save was invoked. The full Save word and floppy glyph remain visible on desktop and phone, while View is a separate action. This directly addresses the original ambiguous mobile checkmark.

| Width | Save x / width, unchanged clean→dirty→restored | View x, unchanged |
| --- | --- | ---: |
| 1280 | 1182 / 82px | 962.914 |
| 768 | 670 / 82px | 450.914 |
| 390 | 292 / 82px | 178 |
| 320 | 230 / 82px | 116 |

Save was 44px high and the status reservation 54px in both schemes. There was no control movement. At every pair I opened Versions and scrolled to the document bottom; **the complete Saved label and disabled Save stayed visible**. These were live clean deep-scroll observations; dirty was checked at the title. I do not claim my own pending/persistence/error/retry execution.

I individually viewed **all fourteen** images in `docs/screenshots/Exh-zrjo/followup-save-states-round-30/`. Their manifest explicitly identifies **42c94eb1 / aa7a00afd7a2** and the parent's disposable-artifact E2E source. These remain current supplementary images of the unchanged runtime:

- Pending and saved at 1280/390/320 in both schemes: **Saving…** beside a subdued named Save, then **Saved** with quiet disabled Save, including deep editor positions.
- Failure at 320 in both schemes: amber **Review**, one warning glyph, and **“Temporary save failure. Try again.”** directly beneath the sticky header. Save remains the adjacent active retry action. The raw cross/duplicate Error prefix is absent.

The visible status sequence and explanation make an in-place save understandable without suggesting navigation. Retry/success behavior itself is supported by the parent's validation, not an interaction I performed. No 768 pending/error image or live permission-Review/resolution was inspected.

## Deeper surfaces and earlier repairs

At **1280 and 320 in both schemes**, I freshly inspected all three Add method selections, Settings top through its Delete-account explanation, expanded Network & permissions, the populated state inspector and lower actions, empty Stored assets and transfers, and the application-owned widget help/source/action frame. The Add modes were actually selected; no URL was fetched, source entered, agent started or form submitted.

Permissions retains shared groupheads/captions, quiet Add, the explicit empty allowlist note, complete short origin example and first-line summary caret. The loaded state count resolves to one key. Inspector removal/movement targets align with their fields; the lower state action area pairs Export/Import with their explanations and separates quiet Erase. The assets paragraphs and transfer helpers have distinct gaps. Widget help has a clear gap beneath its tile, with a contained source frame and full-width phone actions. Settings facts align on desktop and stack deliberately on phone; destructive emphasis is confined to its named section. I did not change permissions, state, assets, widget source or account settings.

Versions was freshly rendered at every required pair. Desktop dates share a column and older View/Restore remain together; phone groups each version, date, badge/actions clearly. The historical staggered timestamp/action-track problem was not visible.

Native editor tools were inspected at **1280 and 320 in both schemes**: complete visible syntax, real keyboard-selected **meta** with `getSelection().toString()` verification, native Find/Replace fields, actual Next navigation without replacement, Go and CSS completion at `27:8`. Selection is visible above the quiet active-line/echo treatment. At 320 the complete Previous label and whole option units fit; Go has readable field/action/Close controls. Find and replacement specimens were cleared, and no replacement or completion was accepted.

Two first 320 completion attempts had offscreen tooltip rectangles after horizontal source scrolling; they were **excluded**, not certified from metadata. I reset the source scroller horizontally and reopened the native completion. The actual visible popup then occupied **x50.555–312**, with its 8px right inset, in both schemes. All source observations retained the 28,568-character buffer and Saved state. Uncommon completion detail/special-character/bracket cases were not exhaustively tested.

Card posture receipts and Duplicate were rendered at 1280/320 both schemes. The receipt's headers, paragraphs and separators are readable; the desktop popup stayed inside the 800px view, and phone placement avoided the fixed tabs. Duplicate retains the shared field/check/helper grammar and deliberate phone footer stack. I opened the card menu and selected only the non-persisting Duplicate dialog entry, then cancelled. Filters was rendered at 320 both schemes with aligned body/header/footer insets. Viewer receipt and More menu were also rendered at 320 both schemes. No mutation, download, star or mode action was chosen.

## Short Share: actual opener, scrolling and lower paint

Share was opened through the viewer's real Share button, never injected with `showModal`. At every pair below I inspected opening, actually wheel-scrolled `.modal-body`, inspected the lower painted content, closed/reopened, and inspected the reset top. Close received opening focus; notice/link controls were initially visible.

| Viewport, each in light and dark | Body client / scroll height | Actual lower scrollTop | Reopen scrollTop |
| --- | --- | ---: | ---: |
| 320×640 | 477 / 1124px | 647 | 0 |
| 390×640 | 477 / 1061px | 584 | 0 |
| 640×360 | 172 / 947px | 774.5 | 0 |

Phone lower views show both complete data choices and the switch disclosure above pinned Done. At 640×360 I additionally wheel-scrolled back to the two cards' tops: both native radios painted inside their cards, without appearing over the footer. Footer hit tests belonged to the footer at all six pairs. No horizontal dialog overflow was present.

I also actually expanded Embed and the switch explanation at 320×640 in both schemes, reaching the final paragraph by wheel scrolling. A first dark attempt merely toggled already retained disclosures closed; those incorrectly labelled intermediate views were discarded. Fresh navigation followed by verified open disclosures supplied the scored dark images. No checkbox, grant, person, data mode, Copy, Replace link or Revoke action was changed.

## Judgment of remaining refinements

The material sampled surfaces earn 9 because typography and helpers retain their hierarchy across ordinary and deep views; cards, forms, sheets and source chrome use a coherent control/palette language; action emphasis remains understandable; and narrow compositions are intentionally reflowed rather than simply clipped. Dark uses readable recessed fields and restrained structural boundaries without foreign light native editor chrome. The current empty family also holds through the newly observed filter-only context and real reset.

I reassessed the retained minor objections rather than exempting them automatically:

- **Landscape Share:** 77px header and 77px footer around a 172px body cost several short scrolls. The actual scrolling, visible choice paint, fixed Done and reset make it usable and orderly. A little less vertical chrome would be a worthwhile small refinement; it is not a material responsive failure in these renders.
- **Dark native radio:** the unselected gray disc is less elegant than the surrounding palette, but selected ring/card/text emphasis clearly distinguishes the choice. It did not compromise recognition or contrast.
- **Activity:** repeated “Artifact version saved.” fallback lines are verbose. The feed still has consistent, legible event/title/date/link grouping. Omitting redundant fallback copy would improve economy, not repair a broken composition.
- **Library return wording:** Add/Settings still say Library while `/` is Starred. This is a small naming drift; the back affordance and destination are clear. It is not a reason to restore the obsolete destination model.
- **Save state at rest:** its filled action stays enabled while header Save is clean/disabled. That is a minor local-state consistency opportunity, but the explicitly named panel action is separated from transfers/Erase, and the header's dirty/pending/error vocabulary remains clear.
- **Compact selected dot:** partial dot/badge overlap is still visible on desktop creation. The inset contour is clean and the selection intelligible. It does not reproduce the user's corner defect or require enlarging all preset controls.

No category is below 9, so this review has no mandatory sub-9 treatment. These observations do not imply a perfect 10 or certify unsampled states.

## Supplementary validation, limits and restoration

At entry, the full-Go confirmation receipt was absent, so I did not assume a pass. Before finishing, the safe validation note was updated and I read `round-31-go-confirm-status.json`: **`go test -short ./...`, exit 0, 431.62s, baseline c1156ef, runtimeAndValidationUnchanged true**. The old 470.07s failure remains a failed historical receipt. Focused expectation checks are reported passing in 13.73s. The unchanged build/478 Node/native40+workflow/Save E2E/full21 E2E/current244-capture results are parent validation context, not tests I ran or score proof.

I read the current populated corpus manifest but did not individually inspect all 244 images. The fourteen Save images are the only saved current corpus images I used for visual outcome evidence; live filter-only evidence in this report comes from the new valid fixture itself. Earlier reports/captures supplied historical context. No screenshot files were written by me.

Breadth is representative outside the explicit matrix. Add/Settings/deeper panels/native tools/card overlays were not each inspected at all four widths; native tools were sampled at 1280/320, viewer receipt/More and Filters at 320, and Share at the six stated short pairs. No own persistence/error/retry, populated origin/asset rows, arbitrary long/custom/invalid data, populated running agents/conversations, hosted identity, enlarged text, physical phone, software keyboard, Safari/alternate engine, screen reader or exhaustive keyboard/accessibility certification is claimed. The user-authored light Run Log body in the dark viewer was excluded from application-chrome scoring.

All eight title specimens were restored without Save. Find/replacement fields were cleared, completion dismissed, and every sheet closed. A final fresh editor navigation showed **Run Log / Saved / Save disabled / aria-busy=false**, source length **28568**, widget source length **10505**, zero open dialogs or native Find/Go/completion panels, blank tag creation name, default `#6b7280` creation color and zero horizontal overflow. Empty ended at `/search` with Welcome and a blank query. Both exact owned tabs were closed; no other tab was touched.

No valid save/create/delete/grant/transfer/erase/star/fixture mutation was performed. Normal viewer navigation's opened receipt was the allowed side effect. **This assigned report is the only file I wrote.** The strict scoped baseline and binary identity remained unchanged at exit. Pairing this independent result with the withheld peer, and deciding the joint streak, remains the parent's responsibility.
