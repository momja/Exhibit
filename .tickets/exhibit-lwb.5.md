---
id: exhibit-lwb.5
status: closed
deps: [exhibit-lwb.1]
links: []
created: 2026-06-30T02:57:53Z
type: task
priority: 2
parent: exhibit-lwb
---
# Make scanner base-aware; report residual origins

The scanner discards relative URLs today (scanner.go:93 returns '' for relatives). For snapshots, anything that couldn't be inlined (runtime-constructed fetch URLs, opt-out/over-limit assets) must still surface in the network footprint so the user approves it into network_allowlist -> CSP. Teach the scanner to resolve relatives against the source base when one is present, so residual external origins appear in the footprint instead of silently 404ing at render.

## Acceptance Criteria

when a source base is supplied, relative refs resolve to their real origin and appear in the footprint; absent a base, behavior is unchanged (relatives dropped).


