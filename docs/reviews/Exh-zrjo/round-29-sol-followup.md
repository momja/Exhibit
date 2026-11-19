# Round 29 — independent Sol UI review

Reviewer: **gpt-6.1-sol**, 2026-11-05, approximately **04:01–04:17 UTC** for context and live inspection, followed by report writing.

Frozen runtime **575559b5abaadc997a6faf62be3669e4881cfe27**, served assets **2f37c8103d52**, stack base **ab0135c902559d5912deec420947d6ab3cbdbcf6**. Populated application: **http://t3code:18280**, renderer **http://t3code:18281**. Separate empty fixture: **http://t3code:18380**, renderer **http://t3code:18381**. These are separate origins, not `/render` paths.

**Verdict: PASS for Sol. All fourteen fresh scores are 9.** The corrected destination-specific empty states work in the live empty fixture at all four widths in both actual schemes. The exact user-reported tag corners, tag spacing and Save ambiguity are repaired in the rendered interface. The broader inspected surfaces also meet the excellent bar; I found no concrete material weakness requiring a sub-9 treatment.

This is one independent review, **not a joint passing round or council-completion claim**. The supplied incoming joint streak is zero. No current Round 29 peer report, parent score/summary/resume aggregate, ticket or current parent timer was consulted. Prior scores and test passes did not determine these ratings. Round 28's interrupted partial observations have no score table and are not a pass; the unlaunched Opus phase supplies no evidence.

## Fourteen independent ratings

Each category considers its weakest materially inspected app-owned surface. Nine means excellent, intentionally composed and harmonious, with only trivial refinements remaining. It does not require decoration or a new product feature. Light and dark were judged separately.

| Fixed category | Light | Dark | Fresh rendered basis |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | Page titles, section heads, captions, facts and helpers have clear roles across the new destinations and editor. Expanded explanations and receipts have comfortable line rhythm. Native source controls and Save/status wording remain readable. |
| Composition and spacing | **9** | **9** | Destination titles and descriptions are separated; cards, empty states and Activity groups have deliberate spacing. Compact tag labels sit beside external utilities. History shares columns, transfers pair with their explanations, and sheets separate fixed chrome from scrolling content. |
| Color harmony | **9** | **9** | Slate neutrals and restrained blue emphasis remain coherent across navigation, fields, empty illustrations, presets, receipts and source chrome. Dark surfaces belong to the same system. User-authored tool and widget bodies are excluded. |
| Contrast and emphasis | **9** | **9** | Task primaries and selected states lead; local Add, transfers and destructive entries remain subordinate. Dirty Save becomes the enabled primary while clean Save is inactive. Actual selected text and current Find matches lead quieter echoes. |
| Component visual consistency | **9** | **9** | Tags and Edit tag share the inset badge treatment. Form controls, panel actions, sheet shells, receipts, menus and native source panels use compatible type, surfaces and control density. The new destination empty states retain appropriate distinct actions without foreign styling. |
| Responsive visual polish | **9** | **9** | Actual 1280/768/390/320 layouts remain composed, with no measured document overflow. Phone navigation, main Search form, permission fields, tag actions and sheet footers regroup intentionally. All six short Share first/reopen journeys start at the top and genuinely scroll to the lower content. |
| Save discoverability and feedback | **9** | **9** | Visible Save plus a floppy glyph removes the checkbox ambiguity. Live title changes produce Unsaved/enabled Save, and exact restoration produces Saved/disabled Save at all four widths in both schemes. Current-source supplementary images show Saving, Saved and an in-view failure explanation with a retryable Save. Persistent save/retry execution was not performed by this reviewer. |

No material repair is required for these scores. The limits and minor opportunities below bound that conclusion.

## New empty destinations — current live evidence

Used the genuinely empty secondary fixture, without creating content, stars or shares. Independently visited **Search `/search`, Starred `/`, and unified Shared `/shared` at 1280×800, 768×900, 390×844 and 320×740, each in light and dark**: 24 current live rendered views.

At each width/theme/route combination, I read the current URL, main text, main links, media query and document width. `matchMedia('(prefers-color-scheme: dark)').matches` was **false in light / true in dark**. Every combination measured **0px document horizontal overflow**. Loaded versioned assets matched **2f37c8103d52**.

| Empty destination | Observed current content and action | Rendered judgment |
| --- | --- | --- |
| Search | Main Search field remains present. The welcome illustration and **WELCOME TO EXHIBIT** precede **Create your first artifact**, whose actual `href` is **`/new`**. | Clear creation emphasis, contained illustration and readable button at every width in both schemes. At 320×740 the action is visible above the fixed bottom navigation. The missing creation CTA does not recur. |
| Starred | Star icon, “Keep your favorites close. Choose Star in an artifact’s action menu.” and **Browse your artifacts**, whose actual `href` is **`/search`**. | The action offers discovery rather than returning to its own empty page. No unrelated Share instruction remains. Copy wraps within the dashed panel; the button and icon retain their own breathing room. |
| Shared | Share icon and “Shared artifacts will appear here. Share one of yours, or ask someone to share with you.” **No main action link.** | A neutral explanation of both directions of sharing. No irrelevant creation/discovery CTA or self-link. At 320 the longer explanation remains readable and contained. |

The empty Search welcome has a different, more illustrative composition from the quieter destination panels; that distinction is purposeful onboarding emphasis, not a component mismatch. Link destinations were read from the live anchors; I did not submit creation or sharing flows to prove them.

I read the source-keyed [empty-state manifest](../../screenshots/Exh-zrjo/stack-empty-states/manifest.json) as context, but the observations above come from my own current live views. Its four old-source before frames and the superseded populated captures were not used to certify unseen current states.

## Populated destinations and Search

Fresh live coverage:

| Width | Both actual schemes — inspected surfaces |
| --- | --- |
| **1280×800** | Starred, blank Search with six accessible cards, unified Shared with incoming and owned cards, Activity; later desktop card receipt. |
| **768×900** | All four destinations. Main Search form and two-column card grid; stable active rail; Shared attribution; Activity groups and version entries. |
| **390×844** | Blank Search, `run` results, unified Shared and Activity. Main Search field measured 44px high, with integrated clear in results. |
| **320×740** | Starred, unified Shared, Activity, incoming `packing` result and `not-in-this-library` miss. Later actual `run` form submission and real navigation clicks through Shared, Activity, Starred and Search. |

The actual dark media query was checked at every required width in both schemes. Desktop navigation rows at 768 remain **44px high at y76/124/172/220**, including the active label. The 240px rail leaves two comfortable card columns. New heading/description spacing is positive and visually clear; no inherited negative overlap appeared. Phone destination labels share balanced 46px targets without the obsolete two-line reservation.

Search remains in the main content at narrow widths. Actual 320 light/dark typing of `run` and Enter produced `/search?q=run`, a **296×44px** main field and two matching cards. Actual phone navigation then confirmed each requested route and active label, rather than trusting a repeated Search frame. Incoming `packing` attribution is discernible, and the miss state has a clear heading, helper and full-sized Clear search action. No measured horizontal overflow occurred in these states.

Unified Shared clearly includes **Shared packing list / council-guest** and owned **Run Log**. Activity separates the empty Running agents region, explicit unavailable detailed data-audit explanation, and Recent changes. Version badges, timestamps and history links have readable roles and regroup on phones. I did not launch an agent, inspect a populated running-agent conversation, or certify unavailable audit functionality.

## Exact user defects — freshly checked

Read all three supplied user screenshots. They show the intersecting selected corner, long capsule around widely spaced tag actions, and the unlabeled mobile Save checkmark. Those app-owned defects remain in scope.

Live route: `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit`, with the application's **`?return_to=%2Fsearch`** fallback.

### Preset corners and tag spacing

Inspected **Tags creation and the actual Health Edit tag sheet at all four required widths in both schemes**. The check is visibly separated from the cell's outer rounded outline. The former badge/outline intersection is absent in both contexts, including the narrow desktop cell and wider phone cell. This is a rendered observation supported by geometry, not acceptance of the radius equation alone.

| Width | Creation selected cell | Edit tag selected cell | Capsule and external utilities |
| --- | --- | --- | --- |
| 1280 / 768 | **34.75×36px** | **48.5×36px** | Health **70.41×28px**; two **32×32px** actions |
| 390 | **76.5×44px** | **81×44px** | Same compact capsule; two **44×44px** actions |
| 320 | **61×44px** | **63.5×44px** | Same compact capsule; two **44×44px** actions |

Both pickers use **8px outer radius / 1px border**, with a **16×16px badge inset 3px at top/right and a 4px radius**. The badge stays within the outer arc. Muted dots and selected blue treatment remain coherent in both schemes.

The label background ends before the action group, with a **4px gap**; utility target space no longer stretches the capsule. The pencil and remove glyphs are centered and distinct. Focus restoration can legitimately draw a ring on Edit, and pointer hover can draw its circular background; those are interaction states, not resting pill spacing. Name, native well and hex fields align. Phone creation primary spans its field column; Edit tag retains deliberate Save/Cancel/outlined Delete stacking. No tag, color, preset or attachment was changed or submitted.

### Save purpose, clean/dirty state and outcome evidence

At **1280/768/390/320, light and dark**, the sticky header visibly says **Save**, with a floppy glyph and adjacent status, while View artifact remains separate. Save measures approximately **80.37×44px desktop / 78×44px phone**. The shared header remains 56px high. It reads as saving edits in place, not completing a task or navigating away.

At all eight width/theme pairs I temporarily changed `Run Log` to `Run Log — local review`, observed **Unsaved / enabled filled Save**, then restored the exact original title without clicking Save. Restoration returned **Saved / disabled Save**. At 320 the longer local title wraps normally, while the header keeps the full status and named action. An immediate transition frame briefly appeared to shorten Unsaved; after settling, the full text was visible and measured **52.10px**, with no clipping or overflow. That early frame was not treated as a settled defect. Scrolling through deep editor panels kept Saved and Save available.

**Current supplementary evidence:** inspected all **14** images in [followup-save-states-pr273](../../screenshots/Exh-zrjo/followup-save-states-pr273/manifest.json). At the time read, the manifest identified **575559b5 / 2f37c8103d52**, refreshed from the current disposable-artifact run. Both schemes show:

- **Saving…** beside the named, visibly subdued Save at 1280/390/320, including a deep editor position.
- **Saved** with clean inactive Save on the same editor page at those widths.
- At 320, **Review** plus **“Error: Temporary save failure. Try again.”** immediately below the sticky controls, contained in the viewport, with an active Save for another attempt.

Read the existing `editor-save-feedback.spec.ts` to understand those images. It holds a real PATCH to inspect disabled/`aria-busy`, checks one write after repeated activation, verifies persisted title and unchanged editor pathname, simulates 503 without persistence, then retries and checks Saved plus hidden explanation and persisted title. The allowed validation record reports the current 21-case run passed. **Those are supplementary source/validation records, not my own save or retry execution, and tests did not award a score.** I did not independently execute permission Review/resolution, a persistent save, or a 768 pending/error journey.

## Deeper editor and native source

**Network & permissions:** expanded at 1280 and 320 in both schemes, additionally 390 dark. Group heads compute **14px/600/21px**. Persistent Add origin caption, complete `https://example.org`, secondary Add and “No origins approved.” retain the common form grammar. Phone caption/select pairs are aligned and the summary caret sits with the first line. No origin or capability was changed. Populated/referenced/blocked rows remain unverified.

**Stored state:** introduction, existing nested records and complete lower actions at 1280/320 both. Square inspector utilities and trailing removal controls agree with their density family. Save state leads secondary Cancel/local Add/transfers; Export and Import each pair with their own explanation, followed by a hairline and quiet outlined Erase. Desktop standalone actions are 36px; attached key field/select/Add are 44px; phone actions are full-width 44px. No state value or operation was changed. Loaded summary shows **Stored state · 1 key**.

**Stored assets:** complete empty section and transfers at 1280/320 both. Introductory, deletion and empty paragraphs use **13/19.5 desktop / 14/21 phone**, with 12px separation between those roles. Transfer helpers belong to their controls rather than touching Erase. This fixture has no stored assets; populated asset-row Delete remains unverified.

**Gallery widget:** app-owned frame, explanation, source chrome and actions at 1280/320 both. Help has a deliberate gap below the preview; phone Regenerate/Remove are full width. Disabled Regenerate is visibly inactive. The tile's authored design was excluded.

**Versions:** 1280/320 both, without invoking View/Restore. Both desktop dates begin **x715.90625**, both action cells **x859.6484375**; older View/Restore stay together. Phone facts/actions regroup clearly above Return to Search and quiet Delete. A separate 768 history measurement was not repeated.

**Native source, unchanged content:**

- Actual Go panel at **320 both**: Control+Alt+G, complete label/field/Go/Close, navigation to `4:1`. Four Shift+Right gestures selected exactly **`meta`**, confirmed by native selection and `.cm-content` focus. Selected fills are **rgb(219,231,251)** light / **rgb(39,54,74)** dark, active-line alpha **.25**, echo alpha **.08**. The actual selected word visibly leads its echo. No exhaustive token/composited-contrast audit was repeated.
- Populated Find **`meta`** / replacement **`link`**, invoked Next, never a replacement action. Inspected **1280/390/320 both**. Flat readable controls, complete Previous, fixed Close, whole-unit options (one row at 390, 2+1 at 320) and retained source space (**257px desktop / 240px phone**) remain composed. Both inputs were cleared and Find closed.
- Completion opened with actual Control+Space at existing CSS **`27:8`**: inspected **1280 dark and 320 both**. Narrow popup **x50.55–312, 242px high**, retaining the 8px right inset. Readable centered rows and clear selection. No item accepted. An earlier attempt after resizing left source offscreen and returned an unpositioned `y=-10000` tooltip; that was discarded. A fresh editor load, visible source and explicit Go produced the observed narrow popup.

## Share — actual opening, scrolling and lower paint

Used the real **`#share-open`**, never direct `showModal()`. No sharing value was changed.

| Viewport, each in light and dark | Body client / scroll height | Actual wheel-scrolled position | First / reopen position |
| --- | --- | ---: | --- |
| **320×640** | **477 / 1124px** | **647px** | **0 / 0**, header Close focused |
| **390×640** | **477 / 1061px** | **584px** | **0 / 0**, header Close focused |
| **640×360** | **172 / 947px** | **774.5px** | **0 / 0**, header Close focused |

All six short cases start with the notice rather than autofocus into People. Fresh top and lower images show real scroll movement, reachable data choices/rule/disclosure and fixed Done. Footer hit probes return its own region in every case; radios clip with their cards rather than painting over the footer. Landscape necessarily shows the cards in portions; I do not claim both full cards fit simultaneously. Its 77px header/footer and 172px body remain tight but functional and orderly.

Also opened and actually wheel-scrolled **1280×800 and 768×900, both schemes**: body **612/905** and **712/905**, lower positions **292.5 / 192.5px**. At **320×740 both**, expanded Embed tile and the switch explanation. Code/Copy/helper remain contained; actual scrolling reaches **998px** with the final explanation above the fixed footer. Both disclosures were closed again. No Copy, Replace, Revoke, grant, checkbox, radio or data-mode operation was invoked.

## Other sampled surfaces

- **Card menu / Duplicate, 320 both:** actual Run Log menu, then Duplicate. Even icon/text rows, common checkbox/helper hierarchy and full-width phone primary/Cancel. Dismissed unchanged. No duplicate created.
- **Card receipt, 1280/320 both:** actual posture/sharing trigger. Readable section heads, body rhythm and distinct Manage action. Desktop Starred receipt fits below its trigger within the 800px document; phone receipt fits above. No document-extension defect. This desktop placement differs naturally from historical lower-card captures that had to flip upward.
- **Viewer receipt and More, 320 both:** inspected after settling. Opaque readable surfaces, compatible icon/text spacing and contained menus. The light Run Log body under dark preference is user-authored content, excluded from scoring.
- **Filters, 320 both:** Collections/Tags variant, shared header separator, aligned rows and full-width Done. No filter chosen. Owners variant was not separately reopened.
- **Add Agent/URL/empty Paste, 1280/320 both:** persistent selected method, shared captions and upright prompts, readable snapshot explanation and complete empty Paste prompt. Phone primary/Cancel stack is deliberate; Paste's lower helper/actions were separately scrolled into view in dark. All input/source values remained empty; no execution, fetch or submission.
- **Settings, 1280/320 both:** top Profile/Administration/Storage through lower Sign out and complete disabled-delete reason. Desktop facts align, phone facts stack, comparable values share weight, destructive emphasis is restrained. No account action invoked.

## Minor opportunities and limits

No material treatment is needed to earn these scores on the inspected states. Optional refinements remain: recover some landscape Share body space with smaller chrome padding; reduce repeated public-link explanation; make Add/Settings home-link wording more specific than Library if the product wants it to mirror Starred. Native dark unselected radio discs, bold phone method segments, Current's small inset, the generous 240px rail, quiet source hints and the utilitarian completion marker remain defensible choices. “Review” is broad wording, but its visible failure explanation and “Try again” provide the needed meaning and next action.

Coverage is broad and sampled, not every surface/theme/width/state permutation. Unverified: physical phones/software keyboard/Safari/alternate engines, coarse-pointer context, enlarged text, screen readers or exhaustive accessibility; arbitrary long/multiple tags/custom colors; populated assets and approved/referenced/blocked origins; many grants, alternative/deep/boolean state specimens; unusual native completion details/special characters/bracket/fold variants; hosted identity/admin operations; populated running agents/conversation destinations; invalid-title and live persistent/error/retry/permission-resolution states. User-authored tool/widget design and credential-dependent execution were excluded.

## Independence, source identity and exit restoration

Read `visual-brief.md`, `nine-brief.md`, all requested change notes through 29, the Round 28 interruption/status record and the allowed `followup-validation.md`. Both Round 25 and both Round 26 reports were read in full across bounded reads, plus completed Sol 27 and its no-score Opus record. Older Round 5–24 reports were consulted through findings/repair/minor excerpts and the brief's cumulative history; some large extracts were truncated, so this is **not a claim to have re-audited every historical coverage paragraph line by line**. Historical quota/deferral records were treated as no-score context, not reviews. No current Round 29 peer outcome was accessed.

The latest permitted validation note at exit reports **current 21 E2E exit0/140.55s**, current-source Save images, and **full Go still pending**. The populated capture refresh was still described as running. I neither ran nor independently certified those suites and did not infer a pending result. The old 68655 populated captures were not used as current visual proof.

Read-only Git comparisons at entry and exit named only **internal web e2e cmd scripts Makefile go.mod go.sum**. The diff from frozen **575559b5**, uncommitted diff and scoped untracked-file check were empty, with no exclusions. No repo-wide aggregate diff was used. Live asset URLs matched at the final editor check.

Used only my own tabs **`tab_1w_0d23e38c-d413-4e76-b0eb-59687a206343`** (populated) and **`tab_1x_0d23e38c-d413-4e76-b0eb-59687a206343`** (empty), opened after preview_status with `reuseExistingTab:false, open:false`, and exact IDs on every subsequent call. **Both tabs are closed.** Signing into the second server under the same default profile displaced the primary server's cookie; subsequent login frames were excluded from populated coverage, then the primary was reauthenticated and those routes freshly reviewed. An initial Sign in locator timed out; the snapshot-provided exact target succeeded. Neither was scored as an app-layout defect.

Actual appearance was set with `preview_set_appearance` and proved with `matchMedia`, never a root class. Primary evidence is fresh live images, reversible interactions and DOM/computed geometry. Supplementary images comprise the three original user attachments and fourteen current-source Save captures. No screenshot/capture file was written by this reviewer.

**Final editor reload:** original **Run Log**, source **28,568** characters, widget source **10,505**, header **Saved**, **Save disabled**, **aria-busy=false**, **zero open dialogs**, current assets and **0px document overflow**. Every local title specimen was restored without saving; Find/Replace cleared, completion dismissed without acceptance, sheets dismissed unchanged. Navigation's automatic opened receipt was the allowed normal product behavior. No artifact, grant, star or preference action was persisted; no valid save/create/delete/transfer/erase/link replacement/data-mode change, build, test, commit, app edit or subdelegation. **Only this assigned report was written.**
