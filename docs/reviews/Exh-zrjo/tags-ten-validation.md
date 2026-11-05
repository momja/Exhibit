# Current focused tag UI validation

Application f58de10bf88acff268b9339e0c7dae406a93c664, assets 4beedb277545.
Entire internal web e2e cmd scripts Makefile go.mod go.sum scope is clean against
f58de10b, with no exclusions. Source fingerprint:
1fc8899f65781b39212653be286d8064d2ea02bcfd772558ad9a2033ac71992a.

| Check | Current actual result |
| --- | --- |
| Full make build (all asset workspaces + server) | PASS17.61s |
| Gallery Node suite | Carried479 PASS from35 on unchanged JS/tests; not rerun for CSS-only change |
| Icon registry + served CSS + native Tag modal | PASS20.14s; actual1280/390 native subtests, no skips |
| Complete Playwright suite | PASS24tests,119.39s |
| Current actual Add-row layout measurements/photos | Eight before +eight after at1280/768/390/320, both real schemes;81px desktop overhang→0px, no overflow or tag API requests |
| Full standard Go short confirmation | PASS exit0,441.10s; API426.015s/store188.443s, entire fingerprint unchanged |

The ENTIRE delta from the previous235a5c9e candidate is three rule edits in
web/gallery/edit.css. No JS/templates/Go/tests changed. The explicit whole-scope
proof and source/served JS byte agreement are in tags-ten-r36-scope-change.json.
Node479 remains a previous execution on the unchanged JS/test code, not a fresh
run or current full fingerprint claim. No source paths are excluded from the
strict current-candidate comparison.

Current completed receipts/logs: tags-ten-r36-*-status.json/.txt, confirming exit0
and unchanged entire current fingerprint during each. Runtime identity proves
bin/server and both current process executables match SHA
7be17fc28c646d805fb5deaa0accd791bc338a837d11c7900c44a6ce5e5d7458.
The binary has no VCS stamp. Provenance is real source/build/assets/process hash.
Full current Go confirmation is complete; saved tags-ten-r36-go-final-status.json/.txt.
Original durable status: /tmp/exhibit-zrjo-tags10-r36-go-final-status.json and.log.
Read actual exit; don't rebuild assets during Go or repeat passed checks unchanged.
Previous full35Go PASS443.14s is retained as history, not claimed as the new run.

Main http://t3code:18280/render18281/default and isolated Tag sample
http://t3code:18580/render18581/incognito use the same new binary and unchanged
fixtures. Public identity tags-ten-playground.json. No Main or sample tag writes;
only local Utilities selection, restored. Persistent tests use disposable18080.
OldEMPTY18380 is outdated. Other-task18480/18481 were never touched.

All existing browser behavior checks remain: real409/Enter guard, pending locks,
503/retry/global rename/recolor, association-only Remove, global Delete retaining
artifacts, partial201/attach503 retry/focus and htmx error retention, invalid local
fields, palette geometry/keyboard, footer DOM order and short sheets. No new test
for the small reversible width fix; real browser geometry and photos directly
verify alignment. Current full E2E pass covers these existing assertions.

Parent captured and measured all eight current sample pairs, visually inspecting
representative desktop and phone dark views. Photos focus the Add row, not a
parent full-core certification. Each reviewer must actually VIEW its own complete
16Main states +4sample pairs, including lower controls at every relevant pair.
Parent photos/tests do not award ratings or substitute for personally viewed
outcomes. R35 outcome photos remain historical supplementary evidence; persistence
paths unchanged. No physical-phone/Safari/screen-reader certification.
