# Round 37 — independent Sol Tags confirmation review

**All 14 independent ratings are 10.** I found no actionable visual or interaction finishing issue in the inspected Tags section and Edit tag sheet. I personally viewed all 16 required MAIN states and all four isolated sample pairs, including fully exposed lower Create controls in every pair. This is one reviewer's judgment, not a same-round joint result or a council completion claim.

I read the original `tags-ten-brief.md`, both round-36 reports, the operative round-37 cumulative brief, `tags-ten-round-36-changes.md`, and the specified safe provenance/validation records. Prior ratings did not determine mine. I did not read current-round peer reports/status/launches, parent score/summary/resume aggregates, tickets or timer prompts. No subdelegation occurred.

## Candidate proof

- Application `f58de10bf88acff268b9339e0c7dae406a93c664`; assets `4beedb277545`. Entry HEAD was `b5c0e54d019db9117f75e1b939e5cb5ffebd55fe`; cleanup HEAD was `227f1335abdd6c31a9d91702a04adaba17b82985`, on `feature/Exh-zrjo/ui-council`.
- At entry and cleanup, `git diff --exit-code f58de10bf88acff268b9339e0c7dae406a93c664 -- internal web e2e cmd scripts Makefile go.mod go.sum` exited 0 with empty output. Porcelain over that entire scope was empty. No exclusions. I also read the complete previous-to-current scoped delta: only the three Add-row CSS edits in `web/gallery/edit.css`.
- Both live pages' DOM asset URLs reported only `4beedb277545`. Independently read SHA-256 values for `bin/server`, MAIN `/proc/305222/exe`, and sample `/proc/305238/exe` matched at entry and cleanup: `7be17fc28c646d805fb5deaa0accd791bc338a837d11c7900c44a6ce5e5d7458`. Listener inspection tied those PIDs to 18280/18580; both executable paths resolve to this worktree's `bin/server`.
- The supplied build receipt records candidate HEAD, exit 0/17.61s and unchanged complete source fingerprint `1fc8899f65781b39212653be286d8064d2ea02bcfd772558ad9a2033ac71992a`. The completed Go receipt records exit 0/441.10s and that same unchanged fingerprint. I read these receipts; I did not rerun checks or recompute the fingerprint. The binary has no VCS stamp: provenance rests on the scoped comparison, supplied build records, live assets and matching process hashes. Tests contribute no UI scores.

## Independent scores

| Category | Light | Dark |
| --- | ---: | ---: |
| Typography | 10 | 10 |
| Composition and spacing | 10 | 10 |
| Color harmony | 10 | 10 |
| Contrast and emphasis | 10 | 10 |
| Component visual consistency | 10 | 10 |
| Responsive visual polish | 10 | 10 |
| Tag workflow clarity and feedback | 10 | 10 |

Unrounded judgments of the actual inspected scope. **Blockers to 10: none identified.** The type hierarchy, shared control edges, compact capsules, restrained utility actions, distinct selection/focus states and explicit workflow wording remain coherent in both schemes.

## Actual viewed matrix

After `preview_status`, I opened two new owned tabs with `reuseExistingTab:false, open:false`, and supplied the exact owned tab ID on every subsequent operation:

- MAIN/default: `tab_34_0d23e38c-d413-4e76-b0eb-59687a206343`, [Run Log](http://t3code:18280/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit).
- Sample/incognito: `tab_36_0d23e38c-d413-4e76-b0eb-59687a206343`, [Tag editing example](http://t3code:18580/artifacts/252b5594-bfb3-4df5-bb9e-3d91579a5be8/edit). Only this profile required login; no cross-login occurred.

Each pair used real viewport resizing/appearance emulation, verified `innerWidth`/`innerHeight` and **both** scheme `matchMedia` queries. Every listed view was returned with `includeImage:true` and personally viewed. Images remain in tool history; I wrote no screenshot files. No unviewed resize, frame, other appearance/instance, parent screenshot or test fills a cell.

| MAIN CSS viewport | Scheme | mM light / dark | Tags panel actually viewed | Edit sheet actually viewed |
| --- | --- | --- | --- | --- |
| 1280×800 | Light | true / false | Top + overlapping lower view; full Create | Health/Green; complete body/footer |
| 1280×800 | Dark | false / true | Top + overlapping lower view; full Create at y=628–672 | Reading/Custom; complete body/footer |
| 768×900 | Light | true / false | Entire panel; full Create at y=768–812 | Reading/Custom; complete body/footer |
| 768×900 | Dark | false / true | Entire panel; full Create at y=768–812 | Health/Green; complete body/footer |
| 390×844 | Light | true / false | Top + overlapping lower view; full Create at about y=645–689 | Health/Green; complete body/footer |
| 390×844 | Dark | false / true | Top + separately viewed lower exposure; full Create at about y=645–689 | Reading/Custom; complete body/footer |
| 320×740 | Light | true / false | Top + overlapping lower view; full Create at about y=627–671 | Reading/Custom; complete body/footer |
| 320×740 | Dark | false / true | Top + overlapping lower view; full Create at about y=627–671 | Health/Green; complete body/footer |

All **16 core states** were viewed. Desktop sheet rect was approximately `(400,55,480,691)`, tablet `(144,105,480,691)`, phone `(0,138,390,706)`, smallest phone `(0,34,320,706)`. Measured phone bodies were 563px client/563px scroll: complete bodies and footers fit. Document horizontal overflow readings were zero across the matrix.

| Sample CSS viewport | Scheme | mM light / dark | Actually viewed | Add/list/create-column right edge |
| --- | --- | --- | --- | ---: |
| 1280×800 | Light | true / false | Long label; placeholder/disabled Add; Utilities/enabled Add; restored lower view through full Create (about y=635–679) | 554px |
| 1280×800 | Dark | false / true | Long label; placeholder/disabled Add; Utilities/enabled Add; restored lower view through full Create (about y=635–679) | 554px |
| 320×740 | Light | true / false | Long label; placeholder/disabled Add; Utilities/enabled Add; restored lower view through full Create (about y=549–593) | 294px |
| 320×740 | Dark | false / true | Long label; placeholder/disabled Add; Utilities/enabled Add; restored lower view through full Create (about y=549–593) | 294px |

At every sample pair, I selected Utilities locally, viewed enabled Add, and restored the placeholder/disabled Add **without pressing Add**. All four had zero horizontal overflow. Add measured x≈481–554 desktop and x≈221–294 at 320; it does not protrude beyond the shared column.

Capture/automation artifacts were excluded: an incorrect measurement ID returned null, a login text locator timed out and was corrected using the snapshot ref, and one post-login snapshot reported “The page changed while capturing its snapshot.” Its successful retry preceded inspection. Restored-pencil focus rings and incidental hover fills were not treated as resting-state defects; the initial hovered desktop Gray tile was re-viewed with the pointer over the sheet heading.

## Fresh assessment and safe probes

- **Original corner/check defect: resolved.** Checks sit cleanly inset from the rounded tile corner and clear the circular samples, including the narrowest 320px palette. No contour fringe or sample overlap recurred.
- **Original stretched pill: resolved.** Capsules fit their names independently of the aligned Edit/Remove slots. The sample's long name wraps to two lines without covering its dot or actions. Subtle row separators preserve grouping; dark dots retain separating rims.
- **Original mobile Save ambiguity: resolved.** The footer explicitly says **Save tag**, beside Cancel. Clean/invalid Save is disabled; a valid local Red draft visibly enables the primary action. The panel states immediate add/remove persistence; the sheet names library-wide edits. MAIN accurately replaces the unavailable Add row with the all-attached explanation.
- **Selection versus focus:** at 320 Dark, Tab from Reading's Name reached Gray without changing the custom value. ArrowRight selected Red; exactly one radio was checked and one was a tab stop. Picker/hex both became `#b86562`, with a separate visible focus ring and Save enabled.
- **Safe deletion entry:** Delete tag… opened the same-sheet confirmation naming stored Reading, explaining removal from every artifact and preservation of artifacts. Keep tag received focus. Keep restored focus to Delete tag… and preserved the local Red draft. Final Delete was never activated.
- **Validation:** I viewed blank edit Name plus incomplete `#37` at 320 in both schemes. Near-field errors replaced hints, invalid borders rendered, the caption hid and the neutral sample displayed its minus. The sheet remained exactly `(0,34,320,706)`. Restoring Reading/`#8a5db0` cleared invalid states and disabled clean Save; Cancel returned focus to Reading's pencil. A local creation name enabled Create; blank Name plus `#12` showed both nearby errors and disabled Create. Nothing was submitted.
- **Source-only/supplementary:** inspected source retains nearby add/list errors across htmx replacement and selects/focuses the new available tag after partial create/attach failure. Edit's rejected-name guard precedes `readDetails`; both Enter paths route through the guarded functions. I did not press Enter or personally exercise pending/success, 409/503, retry, global edits or persisted deletion. Parent evidence does not become my live coverage.

## Limits and cleanup

No valid Save/Create/Add/Remove/Delete, fixture mutation, Enter in Name/hex, test, build, commit or application edit occurred. Resource timing before cleanup/reload showed **zero `/api/tags` requests in both tabs**. Native color-dialog UI, optional 320×640/640×360 checks, physical phones, Safari and screen readers were not inspected or certified. Broader pages and authored widget bodies remain outside this review.

All local drafts were restored/cancelled. MAIN was reloaded to clear the local blank-name validation, then its clean upper and lower panel were personally viewed: Health `#377a64`, Reading `#8a5db0`, Test `#8470ac`, Utilities `#23559e`; blank creation Name/default `#6b7280`, disabled Create, no visible alerts or open dialogs, header Saved. Sample retained its original three attached tags, Utilities available, placeholder selected/Add disabled, blank/default creation fields and no alerts/dialogs.

Both appearances were reset to system and **only my two tabs** were successfully closed by 22:07:54 UTC, 2026-11-05. This assigned report is my only written file.
