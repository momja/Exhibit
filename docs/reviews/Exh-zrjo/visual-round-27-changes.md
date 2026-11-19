# Round27 — response to concrete user screenshots after the historical9 milestone

Frozen application947758e0ed4fee4f3590aa2837981218118520d0, assetsb2069bfa62d2.
Earlier completed25/26 ratings belong to d30dbbe9/e29fcce0e122, not this source.
The user identified THREE concrete misses despite that council: corner-fringe
selected color check, stretched/odd tag-pill spacing, unlabeled phone Save/check
with no visible result. These are required fixes, not optional finishing.

Selected preset check is inset3px from its1px cell border. Inner radius4 equals
outer8 minus border1/inset3: corner centres align and the badge no longer
intersects the outline. The working color-picker behavior/aria-pressed remain.
Editable tag labels retain a compact28px capsule, with separate Edit/Remove
32desktop/44phone targets. No giant empty pill around the action slots. Long
labels can wrap; glyphs16 instead of12. Shared list APIs/htmx/edit sheet unchanged.

Header Save uses explicit text at every width and a registered self-hosted
Phosphor floppy-disk. Persistent sticky Saved/Unsaved/Saving…/Review feedback is
associated with Save. Clean/pending Save is disabled; section-provided failure
or review text appears in-view under the control, not only below the fold. The
shared56px header remains. Save keeps editing in place; View artifact is separate.
Existing editor-session owns persistence and in-flight dedup. Unsaved-changes
Stimulus controller owns the named action/status/buttons; removed duplicate page
click handler. Native Save-and-leave and section snapshots/concurrency preserved.
A native permission regression caught an old Review resurfacing after resolution;
fixed the session issue reset, regression passes. This is an actual behavioral
repair, not a relaxed assertion. No live sample grants/data or user-tool design changed.

Final buildpass;468 Nodepass16.743s; permission regressionpass18.45s. Current full
validation receipts in followup-validation.md.20-case E2E now includes real Save
pending/success/API-persistence/no-navigation/single-write and503 retry plus corner/
chip geometry tests. Current20/group/fullGo pending.14 actual Save-state shots
and6 actual before/after tag views saved; final current E2E will refresh its14
source-specific shots. Actual beforeafter shown via html_preview then html_render.

The council must independently judge actual visual and UX treatment. Tests do
not make a9. Exclude arbitrary user tool bodies/credentialed execution; preserve
honest live versus supplementary-image evidence and unverified device/state limits.
