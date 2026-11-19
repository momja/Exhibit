# Round 27 — independent Sol screenshot follow-up review

Reviewer: **gpt-6.1-sol**, 2026-11-05, approximately **03:03–03:15 UTC**, including required historical context, live inspection and supplementary-image review.

Frozen application **947758e0ed4fee4f3590aa2837981218118520d0**, served assets **b2069bfa62d2**, base **origin/main 44def51a**. Reviewed **http://t3code:18280** and its normal routes; the renderer is the separate **http://t3code:18281** origin. Initial HEAD was the frozen application; final evidence HEAD was **5bde1b4344981ae3059658f3fabc9dad02f1a1d2**. The committed diff from the frozen application, uncommitted diff and untracked-file check were empty under `internal web e2e cmd build Makefile go.mod go.sum package.json package-lock.json`. **No exclusions were used.** Current versioned assets were confirmed on entry and after the final editor reload.

**Verdict: PASS for Sol. All fourteen fresh scores are 9.** The user-reported corner fringe, stretched tag-pill spacing and ambiguous mobile Save checkmark are repaired in the actual interface inspected here. The wider sampled interface also meets the excellent bar in both schemes. I found no concrete material weakness requiring a sub-9 score.

This is one independent model's result, **not a joint pass or council-completion claim**. The corrected application's incoming joint streak is zero. Historical rounds 25/26 concern another application and do not transfer. I did not read the current Opus report, parent score/summary/resume/checkpoint files, or ticket contents. Prior ratings and passing tests did not determine these ratings.

## Fourteen fresh scores

Each score considers the weakest materially inspected app-owned surface. Nine means excellent, intentionally composed and harmonious, allowing only trivial refinements; stronger Library pages do not average away a weak sheet or editor. Light and dark were inspected separately.

| Category | Light | Dark | Fresh basis |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | Page, group, caption, fact and helper roles agree. Expanded data explanations have comfortable line spacing and separated roles. Source syntax, native-panel labels, receipt text and the new Save/status wording are readable. |
| Composition and spacing | **9** | **9** | Compact tag capsules and adjacent utility slots form a clear group. Color controls align. History dates share columns; transfer notes belong to their actions; sheets separate fixed chrome from scrolling content. Cards, Settings and Add retain deliberate gutters and rhythm. |
| Color harmony | **9** | **9** | Slate surfaces, restrained blue emphasis, muted presets/default tiles, recessed source fields and opaque overlays remain coherent. The inset selected badge belongs to the existing accent vocabulary. No foreign dark editor/control palette appeared. |
| Contrast and emphasis | **9** | **9** | Dirty Save gains filled primary emphasis, with explicit status; clean Save is visibly inactive. Local actions and destructive entries remain subordinate. Selected presets/data modes and actual source selection/current Find results clearly lead secondary hints. |
| Component visual consistency | **9** | **9** | Creation and Edit tag share the corrected preset contours. Tag utilities use the compact/phone slot family. Fields, sheets, receipts, inspector controls, transfers and native source chrome use compatible type, surfaces and density. |
| Responsive visual polish | **9** | **9** | Tags and Edit tag work at all four required widths. Save's word and floppy glyph remain visible, including 320, without overflow. Phone sheets/history/actions regroup intentionally; short Share opens/reopens at the top and actually scrolls. The 768 rail leaves comfortable cards. |
| Save discoverability and feedback | **9** | **9** | The named Save button removes the checkbox ambiguity. Live changes show Unsaved/enabled primary, restoration shows Saved/disabled, and status stays visible deep in the editor. Current authenticated test screenshots show Saving, Saved and an in-view failure explanation beside a retryable Save. Current test/receipt evidence supports persistence, single-write behavior, retry and issue clearing; I did not execute a persistent save myself. |

No material repair is necessary for these scores. Optional finishing observations and the limits of that conclusion are explicit below.

## The three user-reported defects

I inspected all three supplied user attachments. They show the actual problems: a selected badge touching the rounded cell outline, an oversized capsule enclosing widely separated Health/Edit/Remove content, and an unlabeled header checkmark beside the viewer eye. Those are app defects, not excluded user-tool designs.

Live route for the checks below: `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` (the app adds `?return_to=%2F`).

### Selected preset corner — repaired in both shared contexts

Opened Tags, inspected its creation picker, then opened Health's actual Edit tag sheet and dismissed it unchanged. Repeated at **1280×800, 768×900, 390×844 and 320×740, light and dark**.

The check now sits on a small rounded **square badge inside the selected cell**, with visible space between badge and outline. In the rendered views, the top-right cell arc continues cleanly around the badge; the old circle/outer-corner intersection and protruding fringe are absent. This applies to the creation picker's selected gray and the sheet's selected Health green, rather than assuming that one context proves the other.

Supporting live geometry, identical in both schemes:

| Viewport | Creation selected cell | Edit tag selected cell | Shared badge and contours |
| --- | --- | --- | --- |
| 1280×800 | 34.75×36px | 48.5×36px | Outer radius 8px, border 1px; badge 16×16px, top/right inset 3px, radius 4px |
| 768×900 | 34.75×36px | 48.5×36px | Same |
| 390×844 | 76.5×44px | 81×44px | Same |
| 320×740 | 61×44px | 63.5×44px | Same |

The measurements support the observed repair; the equation alone is not my approval. The actual wider phone cells clearly show the badge separated from the outer arc, and the narrow desktop cells also keep the two outlines apart. Preset color dots remain round, the selected tint is restrained and the check remains readable in dark. I did not alter a preset or custom color, nor save a tag.

### Tag-pill spacing — repaired without losing the utility targets

The **Health label capsule is 70.41×28px** at every inspected width. Edit and Remove sit **outside** its background in a neighboring action group, rather than enlarging the capsule to accommodate their targets.

- At 1280/768, the item is **138.41×32px**, with two **32×32px** actions.
- At 390/320, the item is **162.41×44px**, with two **44×44px** actions.
- The label capsule and action group have a **4px box gap**. The text/dot have a compact internal rhythm; the larger phone utility slots no longer create a long empty pill.

At rest, this reads as one tag with two adjacent utilities. The pencil and × are visibly distinct and centered. Some post-dialog frames showed the legitimate restored keyboard-focus ring on Edit; I also inspected frames with focus moved to the title, so that ring was not mistaken for a resting circular background. Existing-tag Add remains secondary and phone Create and add tag remains full width. The single attached Health tag is the live specimen; multiple attached tags, very long labels and wrapping combinations were not populated in this read-only review.

### Header Save — purpose and feedback are now clear

At every required width and actual scheme, the header contains a visible **floppy glyph plus “Save”**, with a persistent neighboring **Saved / Unsaved** status. The separate eye/View artifact control still expresses viewing. Save therefore reads as committing edits in place; it no longer resembles a completion checkbox or promises navigation.

Live clean Save measures **80.37×44px at 1280/768**, **78×44px at 390/320**. The header remains 56px tall. The label and glyph fit together, the status is separate and readable, and document horizontal overflow was **0** in every checked width/theme pair. At 320, the action group adjusts its position for the longer Unsaved status while keeping Library and View distinct.

**Reversible live interaction at all eight width/theme pairs:** changed Artifact title from `Run Log` to `Run Log — local review`, observed **Unsaved** and an **enabled, filled primary Save**, then restored the exact original `Run Log`. The header immediately returned to **Saved** and a **disabled Save**. I never clicked Save. The title/header also update coherently when the longer local title wraps at 320. Scrolling through deep state, source, widget and Versions sections kept the header and clean status available; the former status-below-the-fold ambiguity is gone.

**Current supplementary pending/success/error evidence:** inspected all **14** images in `docs/screenshots/Exh-zrjo/followup-save-states/`, including both schemes' pending/success at 1280, 390 and 320, and failure at 320. The [manifest](../../screenshots/Exh-zrjo/followup-save-states/manifest.json) now explicitly identifies **947758e0 / b2069bfa62d2**, refreshed from the final 20-case disposable-server run. These are current application images, not the superseded 9a candidate.

- Pending: **Saving…** remains beside the named Save at the deep Versions position; the control is visibly subdued while the request is held.
- Success: **Saved** remains visible in the same edit-page composition and Save returns to its clean disabled appearance. The supplied 390/320 success captures are header strips; they prove the header's visual outcome, not a whole-page success layout by themselves.
- Failure: **Review** is accompanied by the visible message **“Error: Temporary save failure. Try again.”** directly under the sticky controls. It stays inside the 320px viewport rather than requiring a scroll to a section error. Save is visibly active for another attempt. [Light failure](../../screenshots/Exh-zrjo/followup-save-states/save-error-320-light.png) and [dark failure](../../screenshots/Exh-zrjo/followup-save-states/save-error-320-dark.png).

I separately read `e2e/specs/editor-save-feedback.spec.ts` and the current durable E2E/native receipts. The tests use disposable artifacts, hold a real PATCH to inspect pending state, assert disabled/aria-busy, repeat activation and require one write, verify the API title and unchanged edit pathname, simulate 503 with no persisted change, then retry and require Saved plus a hidden explanation and the persisted title. The validation note records the earlier stale-Review defect, its correction and the passing permission workflow. **That is supplementary validation of reliability and resolved-issue reset, not my own save/error/retry execution.** My visual judgment comes from the rendered live clean/dirty states and these current outcome images. Screenshots alone cannot prove persistence or deduplication.

## Retained editor repairs — fresh live observations

**Network & permissions, 1280/320 both:** expanded via its real summary. Group heads remain **14/600/21**, captions **13/600/19.5 desktop / 14/600/21 phone**, matching neighboring form roles. “No origins approved.” explains the empty list. Add is secondary; the persistent Add origin caption and complete `https://example.org` prompt fit at 320. The caret is beside the first line of the wrapped summary. No origin or permission was changed; populated/referenced/blocked rows were not rendered.

**Stored state, 1280/320 both:** inspected the introduction, first nested records and complete lower action area. Inspector utilities have matching square slots and trailing field removal. Save state leads secondary Cancel/local Add/transfers; each transfer has its own readable note; outlined Erase follows behind a hairline. At 1280 Save/transfers/Erase are compact **36px**, transfer buttons **180px** wide; at 320 they are full-width **268×44px**. The old solid danger endpoint, short phone stack and Export-note/Erase collision do not recur. The loaded summary shows **Stored state · 1 key**. Existing records were read, never changed.

**Stored assets, 1280/320 both:** introduction, permanent-deletion warning and empty message are distinct paragraphs, **13/19.5 desktop / 14/21 phone**, with **12px paragraph margins** between those roles. Transfer notes use their explicit paired groups rather than paragraph spacing to supply the layout. Both controls and notes are reachable and coherent. The artifact has no stored assets; populated asset-row Delete remains unverified.

**Gallery widget, 1280/320 both:** app-owned preview frame, helper, source chrome and actions remain composed. Help has a deliberate gap below the preview, source is readable, and phone Regenerate/Remove use full-width 44px actions. Disabled Regenerate reads as inactive. The widget's own visual design is excluded.

**Versions, 1280/320 both:** two existing rows. At 1280 both date cells begin **x715.90625**, action cells **x859.6484375**, and View/Restore stay side by side. At 320 facts and actions regroup clearly above centered full-width Return and quiet Delete. No View/Restore was invoked. A separate 768 history-column measurement was not repeated this round.

**Native source, without content changes:**

- Actual Go panel opened at **320 dark** with **Control+Alt+G**. Complete label, field, Go and Close; navigation to `4:1` worked.
- Four Shift+Right gestures selected exactly **`meta`** on the active row at **320 light/dark**, confirmed by native selection and `.cm-content` focus. The real selection fills are **rgb(219,231,251)** / **rgb(39,54,74)**; active-line paint retains **.25 alpha**, echo paint **.08 accent alpha**. Live views show the selected word leading its next-row echo. Both syntax palettes remain differentiated. No exhaustive composited-contrast calculation was repeated.
- Populated **Find `meta` / Replace `link`**, invoked Next, never replacement. Inspected **1280/390/320 both**. Readable flat controls, field to fixed Close, complete Previous, whole-unit options (one row at390, 2+1 at320), clear current-result emphasis. Both fields were cleared and Find closed.
- Go to `27:8`, then real **Control+Space** completion at **320 both**: centered readable rows, clear selected tint, **x50.55–312**, **242px** high, retaining the **8px inset**. Dismissed without acceptance.

## Share — first/reopen, actual scrolling and lower paint

Used the actual viewer **Share** control, never direct `showModal()`. Every row below was independently exercised in **both actual schemes**: open, inspect, real container wheel, inspect lower content, dismiss, reopen and measure.

| Viewport | Body client / scroll height | Actual downward scrollTop | First / reopen scrollTop | Footer top |
| --- | --- | ---: | --- | ---: |
| 1280×800 | 612 / 905px | **292.5** | **0 / 0**, Close focused | 706 |
| 768×900 | 712 / 905px | **192.5** | **0 / 0**, Close focused | 806 |
| 320×640 | 477 / 1124px | **647** | **0 / 0**, Close focused | 559 |
| 390×640 | 477 / 1061px | **584** | **0 / 0**, Close focused | 559 |
| 640×360 | 172 / 947px | **774.5** | **0 / 0**, Close focused | 266 |

The opening frame starts with the notice; there is no People autofocus or half Replace button under the header. Lower phone views show the complete data choices, public-link rule and disclosure. Native radios stay with their cards and clip with the body; footer hit probes returned its own region. Landscape shows the long choices in portions because its body is short; I do not claim both cards fit wholly at once. Header/footer remain fixed and separate.

At **320×740 both**, additionally expanded Embed tile and the switch explanation. Code/Copy/helper remain contained; actual wheel scrolling reaches **scrollTop998**, with the final paragraph visible above footer **y659**. Closed both disclosures again. No checkbox, radio, grant, Copy, Revoke, Replace or data-mode operation was invoked.

## Broader live coverage

| Surface | Fresh observations and limits |
| --- | --- |
| Library/cards | 1280 both, 768 both, phone Library beneath Duplicate and Filters. Reserved title areas, coherent title/tag/menu anchors, quiet boundaries and restrained default tiles. User-authored tile bodies excluded. |
| Shared spaces / active rail | Actual rail clicks at768 through incoming/outgoing/Library in both schemes. Four rows remain **44px at y76/124/172/220**, with complete active labels and separate counts. Both shared destinations at390/both retain readable attribution and balanced fixed tabs. |
| Search | Blank, `run`, `packing`, `not-in-this-library` at320/both. Integrated clear, readable owner chip, balanced empty copy and active Search location. Every measured narrow Search document had **0px** horizontal overflow. Desktop Search results were not separately reopened. |
| Card receipt / menu | Run Log receipt at1280/both: **x264/y191.09, 320×323px**, document remains800px. Readable common headers/body and clear Manage action. Run Log menu at1280 dark has even icon/text rows and an opaque surface. |
| Duplicate | 1280/320 both. Common Title/choice/helper hierarchy, shared checkbox scale, trailing desktop actions and intentional full-width phone primary/Cancel stack. Dismissed unchanged. |
| Filters | Library at390/both: shared header rule, aligned Collections/Tags rows and centered full-width Done. Dismissed without selection. Owners variant was not separately opened. |
| Add | Agent, URL and empty Paste at1280/320 both. Persistent selected method, one page title, common captions/upright prompts, complete Paste prompt and clear snapshot explanation. Phone Paste footer separately scrolled in dark: helper above consecutive primary/Cancel. No execution, fetch or submission. Paste remained **0 characters**. |
| Settings | 1280/320 both, top through Profile/Administration/Storage and lower Sign out/complete disabled-delete explanation. Aligned desktop facts, consistent value weights, intentional phone stacking and restrained danger treatment. No account operation. |
| Viewer overlays | Toolbar, actual title/posture receipt and More menu at320/both after **450ms settling**. Opaque readable surfaces and coherent icon/text rhythm. The light Run Log tool body under dark preference is user-authored and excluded. |

## Minor opportunities and coverage limits

The short landscape Share shell could recover body space through smaller chrome padding; its present real scrolling and fixed controls work. Native dark unselected radios remain gray discs, but the selected radio and full-card treatment clearly distinguish state. Share's repeated public-link sentence is harmless redundancy. Add's bold phone method labels, uppercase read-only facts versus sentence-case captions, Current's slight inset, the generous 240px rail and quiet source hints remain defensible on this fresh inspection. The inset preset badge remains visually heavier than an unselected dot, appropriately communicating selection. None warrants arbitrary ornament or a new feature.

For Save, “Review” is an umbrella status rather than a specific error name, but the accompanying visible failure message provides the missing meaning and next action. A Retry label could be a future microcopy refinement; the current active Save plus “Try again” is clear enough here. Pending/success/error states at768 were not independently captured or executed. I did not exercise invalid-title validation, permission review/resolution, persistent saves or navigation-after-save myself.

Used only my own T3 tab **tab_1r_0d23e38c-d413-4e76-b0eb-59687a206343**, opened after preview_status with `reuseExistingTab:false, open:false`; its exact ID was supplied on every subsequent browser call. **The tab is closed.** Explicit preview_set_appearance and live `matchMedia('(prefers-color-scheme: dark)').matches` established **false/true at EACH1280/768/390/320**, also at the short/landscape Share sizes. No root-class simulation or other-tab access.

Read visual-brief.md, nine-brief.md, prior reports for both models through26 (quota/deferral files treated as no-score records), all requested change notes6/7/8/10/11/12/13/15/18/19/20/21/22/23/24/25, the new change27 and followup-validation.md. Both25 and both26 were read in full across bounded reads; older large reads were followed by smaller reads for truncated portions. No current peer or parent aggregate was consulted. The current validation record/receipts report **20 E2E exit0/96.67s**, native group **exit0/31.53s**, applicationUnchanged true; fullGo was still recorded running when consulted. These are supplied records, not suites I ran, and no test determined a score.

Primary evidence is fresh live browser images, actual reversible interaction and DOM/computed geometry. Supplementary images are the three user attachments and the14 current Save-state captures; no old-source screenshot was used as current proof. No screenshot/capture file was written. Production implementation was not used to claim that unseen UI is excellent; the one test-source read explains the supplementary Save evidence. An initial evaluation-result extraction returned undefined and was corrected to read structuredContent; that undefined result was not evidence. A source search initially targeted the wrong spec, then found the actual Save spec; no test was run.

Coverage is broad but sampled, not every route/theme/width/state cross-product. Unverified: arbitrary long/multiple tags and custom colors, populated stored assets, populated/referenced/blocked origins, alternative/deep state shapes and boolean specimens, enlarged text, uncommon native completion detail/special-character/bracket/fold variants, hosted identity and administration. No physical phone, software keyboard, alternate engine, screen reader or accessibility certification. Freeform resizing retains desktop Chromium/fine pointer. User-authored tool/widget designs and credentialed execution excluded.

**Exit restoration:** final reload showed original **Run Log**, source length **28,568**, header **Saved**, **Save disabled**, **zero open dialogs**, current assets. All title specimens were restored without saving; Find/Replace inputs cleared; completion dismissed; local sheets/disclosures dismissed unchanged. No builds, tests, commits, application edits, capture writes, subdelegation or persistent fixture changes. No valid saves, creates, deletes, grants, transfers, erase, link replacement or data-mode changes. **Only this assigned report was written.**
