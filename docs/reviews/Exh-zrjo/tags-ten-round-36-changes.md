# Round 36: align the complete Add row

Candidate `f58de10bf88acff268b9339e0c7dae406a93c664`, assets `4beedb277545`.
Both round-35 tasks were terminal before this change. Sol gave all 14 values 10;
Opus gave composition 9.5 in both schemes and the other 12 values 10. The raw
Opus prose miscounts the tens; its original table and report are preserved.
The joint round failed. Focused passing streak remains 0/2.

Opus's concrete remaining issue was real: the available-tag select occupied the
entire desktop 320px control column and pushed Add 81px beyond it. Main cannot
show this because all its tags are attached. Parent reproduced it on the isolated
sample at 1280 and 768 in both schemes, with actual photographs and measurements.

This change is only three CSS rule edits in `web/gallery/edit.css`. The entire
Add row now shares the desktop column boundary; its select flexes within the
remaining space. At the existing phone breakpoint the row remains full width,
matching the tag list and create form. No JS, template, API, tests or behavior
changed. No new test was written for this small reversible layout adjustment.

Actual after measurements at 1280/768/390/320 in light and dark show the Add
button ending exactly at the create/list column edge: zero overhang in all eight
pairs, no horizontal overflow and zero tag API requests. Local Utilities selection
was restored. Eight before and eight after photos are source-keyed; parent viewed
representative desktop/phone dark images. They are not the council's live matrix.
Actual cropped before/after, with full-size originals, is saved as
`../../screenshots/Exh-zrjo/tags-ten-comparison-round36.html`.

Build, current browser and native checks are recorded in safe validation. The
479 Node results carry on unchanged JS/tests, explicitly proved by the complete
one-file source delta and source/served JS agreement. Current full Go confirmation
is required and its actual progress must not be inferred. No tests award ratings.

Round-35 coverage limits also remain recorded: some Main lower controls and the
sample's lower controls were not separately fully viewed by Opus. The sub-10
scores already fail that round. Fresh reviews must actually expose and VIEW all
required controls in every pair, never substitute another appearance or instance.

All prior tag refinements remain. The fresh round re-assesses this small scope
honestly; no rating carries across changed source. Two genuine joint rounds with
all 14 values exactly 10 from each reviewer on the same application are required.
