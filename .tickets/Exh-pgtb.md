---
id: Exh-pgtb
status: open
deps: [Exh-un03]
links: []
created: 2026-07-11T05:02:15Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-avau
---
# exhibit build: new CLI subcommand driving the static export

Add an 'exhibit build --manifest=<path>' subcommand to the existing binary (not a separate tool). It opens the same Store/Blob a running instance uses (real data dir/DB path via existing config), loads the manifest (Exh-un03), enumerates every artifact currently in the library, and drives the static-output pipeline (per-artifact pages from Exh-* render ticket, index pages from the gallery ticket) into the manifest's output directory. Must only run against real, already-ingested artifacts -- it does not fabricate or scaffold example content.

## Acceptance Criteria

Running 'exhibit build --manifest=<path>' against a real instance's data directory enumerates all artifacts in that instance and invokes the static render/index generation for each, writing output under the manifest's output directory. Fails clearly if pointed at a missing/empty data directory. Reuses the same Store/Blob interfaces the live service uses -- no parallel data-access path.


## Complexity

M
