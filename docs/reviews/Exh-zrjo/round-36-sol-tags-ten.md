# Round 36 — independent Sol Tags review

**All 14 independent scores are 10.** No actionable visual or interaction finishing issue remained in the inspected Tags section and Edit tag sheet. I personally viewed the 16 required MAIN states and all four isolated sample pairs, including the fully exposed Create controls in every pair. This is one reviewer's assessment, not a joint pass or completion of the consecutive-round requirement.

I read the original `tags-ten-brief.md`, both round-35 reports, the operative round-36 cumulative brief and changes, and the specified safe runtime/playground/scope/validation records. Prior scores were context only. I did not inspect current-round peer reports/status/launches, parent score/summary/resume aggregates, tickets or timer prompts. No subdelegation occurred.

## Candidate and provenance

- Frozen application: `f58de10bf88acff268b9339e0c7dae406a93c664`; assets: `4beedb277545`. Entry HEAD matched the candidate on `feature/Exh-zrjo/ui-council`; cleanup HEAD was `d62235a0da3b063b1f924feaa3a57e0e32999f52`. At entry and cleanup, `git diff --exit-code f58de10bf88acff268b9339e0c7dae406a93c664 -- internal web e2e cmd scripts Makefile go.mod go.sum` returned 0 with empty output, and porcelain over that entire scope was empty. No exclusions.
- I independently inspected the complete previous-to-current scoped delta: only three CSS rule edits in `web/gallery/edit.css`, constraining the whole Add row and releasing that bound at the existing phone breakpoint. No behavior changes were introduced by this delta.
- DOM asset URLs on both instances reported only `4beedb277545`. MAIN snapshots also showed current asset responses. Independently read SHA-256 hashes of `bin/server`, MAIN `/proc/305222/exe`, and sample `/proc/305238/exe` matched at entry and cleanup: `7be17fc28c646d805fb5deaa0accd791bc338a837d11c7900c44a6ce5e5d7458`. Listener inspection identified these processes on 18280/18580; both executable paths resolve to this worktree's `bin/server`.
- The build receipt records `make build` exit 0, 17.61s, candidate HEAD and unchanged source fingerprint `1fc8899f65781b39212653be286d8064d2ea02bcfd772558ad9a2033ac71992a`. The binary has no VCS stamp. Provenance rests on source comparison, the supplied build/fingerprint records, live assets and matching process hashes; I did not extract a commit from the binary or recompute the fingerprint.

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

Unrounded judgments of the inspected scope. **Blockers to 10: none identified.** The column alignment, compact capsules, restrained utilities, palette contours, readable hierarchy and explicit workflow language form a consistent composition in both appearances.

## Actual viewed coverage

After `preview_status`, I opened my own new tabs using `reuseExistingTab:false, open:false`. MAIN used **default**, tab `tab_30_0d23e38c-d413-4e76-b0eb-59687a206343`; the isolated sample used **incognito**, tab `tab_32_0d23e38c-d413-4e76-b0eb-59687a206343`. Only the sample required login; profiles were never cross-logged-in. Every subsequent preview operation supplied its exact owned tab ID.

Each pair used real `preview_resize` and `preview_set_appearance`, followed by `innerWidth`/`innerHeight` and both color-scheme `matchMedia` checks. Every listed surface was returned with `includeImage:true` and personally viewed. Images remain in tool history; no screenshot files were written. No frames, metadata-only checks, another scheme/instance, parent captures or tests substitute for coverage.

MAIN: [Run Log](http://t3code:18280/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit).

| CSS viewport | Scheme | matchMedia light / dark | Tags panel actually viewed | Edit sheet actually viewed |
| --- | --- | --- | --- | --- |
| 1280×800 | Light | true / false | Top + overlapping lower exposure, full Create button | Health preset; complete body/footer |
| 1280×800 | Dark | false / true | Top + overlapping lower exposure, full Create button | Reading custom; complete body/footer |
| 768×900 | Light | true / false | Entire panel through full Create button in one exposure | Reading custom; complete body/footer |
| 768×900 | Dark | false / true | Entire panel through full Create button in one exposure | Health preset; complete body/footer |
| 390×844 | Light | true / false | Top + overlapping lower exposure, full Create button | Health preset; complete body/footer |
| 390×844 | Dark | false / true | Top + overlapping lower exposure, full Create button | Reading custom; complete body/footer |
| 320×740 | Light | true / false | Top + overlapping lower exposure, full Create button | Reading custom; complete body/footer |
| 320×740 | Dark | false / true | Top + overlapping lower exposure, full Create button | Health preset; complete body/footer |

All **16 core states** were viewed. Both 320 sheets measured `(0,34,320,706)` and had no overflowing auto-scroll descendants; their complete bodies and footers were visible. Horizontal overflow measured zero at 1280 Dark and all 768/390/320 pairs. Incidental hover fills and restored-pencil focus rings were treated as interaction states; a hovered Gray tile was re-captured with the pointer over the heading before judging the resting custom state.

Isolated sample: [Tag editing example](http://t3code:18580/artifacts/252b5594-bfb3-4df5-bb9e-3d91579a5be8/edit).

| CSS viewport | Scheme | matchMedia light / dark | Actually viewed | Add/list/create right edge |
| --- | --- | --- | --- | ---: |
| 1280×800 | Light | true / false | Long label; placeholder/disabled Add; Utilities/enabled Add; scrolled lower controls/full Create | 554px |
| 1280×800 | Dark | false / true | Long label; placeholder/disabled Add; Utilities/enabled Add; scrolled lower controls/full Create | 554px |
| 320×740 | Light | true / false | Long label; placeholder/disabled Add; Utilities/enabled Add; scrolled lower controls/full Create | 294px |
| 320×740 | Dark | false / true | Long label; placeholder/disabled Add; Utilities/enabled Add; scrolled lower controls/full Create | 294px |

At **every** sample pair I selected Utilities, viewed enabled Add, then restored the placeholder without pressing Add; Add disabled again. All four pairs had zero horizontal overflow. One post-login snapshot failed with “The page changed while capturing its snapshot”; its successful retry preceded inspection, and the failure counts as no coverage.

## Fresh findings and safe interactions

- **Round-35 Add overhang: resolved in the actual current views.** At desktop the selector is now about 239px within the 320px row, with Add ending exactly at the list/create boundary. At 320 the shared edge remains intact. The former lone protruding control is gone in both themes.
- **Original badge/corner and pill concerns: resolved.** Selected checks remain inset with clean rounded contours and clear the complete circular samples, including the narrowest tiles. Compact name capsules stay separate from aligned Edit/Remove slots. The long sample wraps cleanly to two lines, retaining its dot and unobstructed actions. Dark dots retain separating rims.
- **Scope and Save clarity: resolved.** MAIN accurately states that all existing tags are attached and omits an impossible Add. Immediate add/remove persistence is stated in the panel; library-wide editing is stated in the sheet. The phone footer visibly says **Save tag**, alongside Cancel. Clean and invalid Save are disabled; a valid local preset change enables Save.
- **Custom, selection and focus remain distinct.** Reading opens with Custom and no checked preset. At 320 Light, Tab from Name reached Gray; ArrowRight selected Red. Exactly one radio was checked and one was a tab stop, native/hex values both became `#b86562`, and the checked tile had a separate visible keyboard focus ring. Subsequent Tab order was native picker → hex → Delete tag… → Cancel → Save tag → Close → Name.
- **Validation is nearby and stable.** I viewed blank Name plus incomplete `#37` in the 320 sheet in both schemes. Errors replaced the helper text, invalid borders rendered correctly (Light `rgb(180,35,24)`, Dark `rgb(242,114,106)`), the caption hid, and the neutral sample showed its minus. The color feedback gap was 6px. The sheet stayed exactly `(0,34,320,706)` before/after errors. Restoring Reading/`#8a5db0` cleared invalid state and disabled clean Save. A local creation draft enabled Create; blank Name plus `#12` visibly produced corresponding near-field errors and disabled Create. Nothing was submitted.
- **Safe confirmation works locally.** At 320 Light, Delete tag… opened the same-sheet confirmation naming stored Reading, explaining removal from every artifact and preservation of artifacts. Keep tag received focus. Keep returned focus to Delete tag… with the Red draft intact. I restored the stored custom color before Cancel; Cancel restored focus to Reading's pencil. Escape also dismissed clean sheets throughout the matrix. Final Delete was never activated.
- **Server outcomes remain source/supplementary evidence.** Source retains nearby list/attach errors through htmx swaps and selects/focuses a newly created available tag after partial attach failure. The rejected-name guard precedes Edit `readDetails`, and both Edit Enter routes use it, matching Create. I did not trigger 409/503, pending/success, retry, global mutation or htmx mutation outcomes. The supplied parent regression/tests do not constitute my live inspection or determine scores.

## Limits and cleanup

No valid Save/Create/Add/Remove/Delete, Enter in Name/hex, build, test, commit, application edit or fixture change occurred. Resource timing showed **zero `/api/tags` requests** in both tabs before cleanup. Native color-dialog UI, supplementary 320×640/640×360 sheets, physical phones, Safari and screen readers were not inspected or certified.

The supplied validation records completed build/native/E2E checks and carried Node results on unchanged JS/tests. At my direct Go check, `/tmp/exhibit-zrjo-tags10-r36-go-final-status.json` did not exist and the log was partial; I infer no current Go completion or pass. No tests award UI ratings.

All local drafts were restored/cancelled. MAIN was reloaded and its clean top/lower panel personally viewed: original Health `#377a64`, Reading `#8a5db0`, Test `#8470ac`, Utilities `#23559e`; blank creation Name, default `#6b7280`, disabled Create, no visible alerts/dialog, header Saved. Sample cleanup verified its original three attached names/colors, Utilities still available, placeholder selected/Add disabled, blank/default creation fields and no alerts/dialog. Both appearances were reset to system and **only my two tabs** were successfully closed by 21:57:42 UTC, 2026-11-05. This assigned report is my only written file.
