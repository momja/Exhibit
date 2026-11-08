# Round 28: user-requested stack on PR #273

Candidate 68655f3a78724ee0ed90f6fa820dd1b12a169432, assets2f37c8103d52,
base PR273 ab0135c902559d5912deec420947d6ab3cbdbcf6. This is a NEW candidate:
old R27 is superseded, no inherited joint pass or obsolete missing Opus phase.

Preserved the complete pre-stack branch at
feature/Exh-zrjo/ui-council-before-pr273-20261105 (a8f9d14e). Consolidated its
net UI patch against44def51a and replayed it onto PR273, resolving15 conflicted
files into c20f6f72; final route/test/spacing adaptations are68655f3a.

PR273 behavior remains: personal Starred root and opened preferences; dedicated
Search form at every width with global readable artifacts; unified owned/incoming
Shared and legacy redirect; Activity running-agent/owned-history/conversation
links and explicit unavailable audit placeholder; new migration054; access and
read-only controls; htmx navigation preserves header/Clerk identity. Search is
now the editor fallback return. Viewer return remains Starred. No old Library
or separate outgoing/incoming navigation was restored.

Own shared typography/color/forms/source/sheets/history/feedback work remains.
The user's exact tag corner/pill and ambiguous Save corrections from27 remain:
inset3+border1+inner4/outer8 badge, compact label with separate32/44 actions,
visible Save/floppy glyph, sticky Saved/Unsaved/Saving/Review and visible issue,
clean/pending disabled, existing section persistence and retry behavior.

Integration found duplicate .gallery-description rules: inherited -6px top margin
pulled the description into the new heading line box. Existing GallerySpaces
browser regression strengthened to assert actual title/description separation at
320/390/768/1280 across route changes and dark. It FAILED old served CSS, then
passed with one shared6px margin rule. Removed obsolete forced two-line minimum
for the new short phone destination labels:46px targets instead of62px. Natural
text wrapping and safe-area behavior retained; no truncation/nowrap shortcut.

Updated duplicate/posture E2E and capture routes to dedicated Search, canonical
partial endpoint and new page-level form; outgoing capture becomes Activity.
The32-case Share opening matrix exceeded its30s combined test budget while
native/Go checks competed; trace showed completed assertions through late touch
cases, with cleanup causing the later404 snapshot. Split mouse/touch into two
independent cases, retaining every width/scheme/first/reopen/focus assertion and
all existing timeouts. This is not an app404 regression or a waived assertion.

Sample-only setup: backed up /tmp/exhibit-zrjo-data/app.db before migration;
RunLog and ReadingList starred via normal personal-preference API to exercise
populated Starred. Artifact contents/state/versions unchanged. Reviewers must
not persist any actions. All seeding is confined to the disposable preview.

Current checks/evidence are in followup-validation.md. New independent ratings
must cover the new destinations as well as the original reports and exact
user defects. Tests and previous scores are not evidence of an earned9.
