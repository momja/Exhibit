# Current PR273 stack validation

Frozen runtime575559b5abaadc997a6faf62be3669e4881cfe27, assets2f37c8103d52,
base PR273 ab0135c902559d5912deec420947d6ab3cbdbcf6. Later commits evidence only.
Strict runtime/validation scope has NO exclusions. No same-round scores here.

All current required checks PASS:

| Check | Actual result and scope |
| --- | --- |
| make build | exit0/19.19s, identical tree committed575559b5. No later runtimeJS/CSS changes. |
| Gallery Node |478 passed/0failed17.633s on integrated productionJS, unchanged since. Full currentGo also confirms its exit. |
| Empty-state unit + Chromium | exit0/28.35s(API14.673s): welcome creation, star discovery/access, empty destinations, Search/redesign. Assertions firstFAIL old, thenPASS new; preserved/strengthened. |
| Full current E2E |21 passed, confirmedexit0/140.55s, runtimeAndValidationUnchanged=true on575559b5. |
| Standard full Go | go test -short ./..., CONFIRMEDexit0/503.36s(API486.398/store228.996), runtimeAndValidationUnchanged=true on575559b5. |
| Current populated corpus |244 captures, confirmedexit0/306.12s, runtimeAndValidationUnchanged=true. Everyasset2f37, actualmediaagreement/errors[] and explicit containment assertions. Bothschemes at320/390/768/1280 plus640shortlandscape. |
| Current empty fixture |24 own-T3 views:3routes x4widths x2schemes, all media/containment/correctactions.4 actual old beforeframes retained. Manifest old68655/current575559b5. |
| Current Save outcomes |14 screenshots from passing current21E2E, source575559b5/asset2f37 manifest. Disposable artifacts only; deliberate503 simulation. |

Go/E2E/capture rawlogs and durable exit receipts: stack-empty-*-status.json/.txt.
Current corpus: docs/screenshots/Exh-zrjo/followup-round-29/manifest.json.
Empty proof: stack-empty-states/manifest.json. Save proof: followup-save-states-pr273/manifest.json.
E2E covers real Save pending/success/error/retry/persistence and repeat activation,
exact tag corners/external actions, actual source selection/chrome, Share opening/
scrolling/footerpaint, popup placement, enlarged history and creation feedback.

Earlier integrated native group passed104.59s(API88.114s) before final spacing/
empty refinements: stars/access, Activity owner/conversations, redesign/navigation/
races/noJS, Search/unifiedsharing, appheaders, editor navigation/permission saves,
Tag/lifecycle and agent composer/markdown/panes. Final spacing/navigation group
passed42.03s at320/390/768/1280 with dark/safearea/keyboard/stable rows. These
scopes are explicit, not relabeled as another execution. Current final Go,
focusedempty and21E2E results are above.

Initial stacked fullGo exit1/603.26s caught REAL missing Search creation CTA.
Actual old Starred also showed incorrect Share guidance/selflink. Assertions
preserved/strengthened; production branch/action metadata repaired. Initial
combined mouse/touch Share matrix hit30s overall deadline under contention;
trace showed completed late actions, cleanup caused later404. Split independent
mouse/touch cases retain every viewport/theme/first/reopen assertion and timeout.
Final21case pass140.55s. Raw failed receipts retained; history in
followup-validation-history.md /followup-validation-before-pr273.md.

Actual before/after stack-comparison.html and stack-empty-comparison.html were
checked with html_preview(console[]) then html_render. All current checks pass;
do not repeat unchanged or rebuild assets duringGo.

Populated http://t3code:18280/render18281; EMPTY http://t3code:18380/render18381,
same current binary, scratchadmin/changeme. Cookies are HOST-scoped: OWN default
profile formain, OWN incognito profile forempty. Don't overwrite main login by
signing into secondary in the same profile. Parent only seeded personal stars
RunLog/ReadingList in scratch. Sample contents/state/versions unchanged.

One Chromium engine, real scheme emulation. No physical-phone/Safari/software-
keyboard/alternate-engine or exhaustive accessibility certification. Validation
and screenshots do not award ratings or certify unseen states.
