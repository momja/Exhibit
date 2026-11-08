# Round29: preserve destination-specific empty behavior

Frozen575559b5/assets2f37. R28 source68655 superseded for a real template branch
regression caught by fullGo, not a cosmetic old-test mismatch. Restored Search
welcome/creation before generic empty rendering; generic empty uses destination
icon/action metadata instead of hard-coded sharing text and root self-link.
Starred uses star icon and existing PR273 Search discovery action. Shared keeps
its own explanation without irrelevant action. Original destination messages,
access checks and PR273 semantics retained. No productionJS/CSS/icon changes.

Full initialGo exit1/603.26s, sole failure TestGalleryEmptyStateLinksToNewPage.
Actual T3320 bothschemes confirmed missingSearchCTA and incorrectStarred share
instruction/selflink. Added discovery assertion in existing personal-star test:
both old defects FAIL before production repair. Focused existing unit+native
empty/redesign/search group passes28.35s, then new runtime committed575559b5.
New24 actual T3 captures allfourwidths/bothschemes on separate empty fixture show
correct actions/schemes/containment; source-keyedmanifest retained. Before4 shots
are old68655 and after24 new575559b5. This is real behavior, not inferred CSS.

Sol28 interrupted before full scores/report, actual status retained; partial
observations honest, mistaken repeatedSearch frames excluded. Opus28 never
launched, no missing obsolete phase. Fresh BOTH29 required. Current Go/E2E and
populated capture refresh statuses are in followup-validation.md; no tests or
previous score grants9. Exact user tag pixels/spacing/Save clarity still in scope.

CURRENT fixture browser note: cookies are host-scoped, not port-scoped. Use
profileId:default for main18280 and profileId:incognito for empty18380, with
OWN distinct tabs/exactIDs; do not share tabs or overwrite the main profile's
login by signing into the secondary port in that same profile. Both profiles
are available in preview_status. Login only with scratchadmin/changeme.
