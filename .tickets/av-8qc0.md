---
id: av-8qc0
status: open
deps: [av-e1sr, av-g8oa]
links: []
created: 2026-07-07T05:44:23Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-f3cp
---
# e2e: allowlist enforcement and sandbox containment

Expand [av-f3cp](av-f3cp.md) after the [harness](av-e1sr.md) and [minimal CI gate](av-g8oa.md). This later security slice can proceed independently of modal delivery. Exercise actual browser policy against the current [security model](../docs/security.md), rather than asserting only generated CSP text.

## Design

- Add `fetcher.html` and `escape.html` fixture artifacts, created through the API and opened in the real authenticated app's opaque-origin iframe.
- Start a test-owned HTTP destination on a third loopback port with request counters and a test-only inspection API. Use unique probe URLs, disable target caching, and allow the fixture's opaque-origin CORS request on this target so an approved fetch can succeed. Restrict fixtures to local deterministic targets.
- For an empty allowlist, verify both the artifact's failed fetch and zero probe requests received at the destination. Observe a policy failure before checking the counter; do not infer network containment solely from a JavaScript error, a CORS failure, or an arbitrary sleep.
- Approve the target through the normal API, obtain a fresh render document, retry, and require both the expected response in the fixture and a received destination request. Also cover a pre-approved origin with no runtime approval prompt. Distinguish the runtime prompt from CSP enforcement; API approval is deliberate setup for this boundary test.
- Probe parent DOM access, document cookies/native storage under the opaque sandbox, and app-origin API reads/mutations. Assert specific denied access and unchanged protected data. A visible failure alone is not evidence that the request was never sent: CORS may deny response access after a server receives it. Authenticated browser activity and the host's legitimate state bridge must still work.
- Two ports on one hostname establish origin isolation but not cookie-domain isolation. Keep cookie host-isolation claims out of that configuration. If testing the app/render cookie boundary, use distinct loopback hostnames and assert cookie placement explicitly. Do not relax CSP, sandbox, auth, or SameSite to make tests pass.
- Ensure a browser's independent local-network protection cannot be the sole cause of a passing denial test: use the approved-target positive control and the existing test-only Chromium policy convention where necessary, with a documented reason. Keep production policy unchanged.

## Acceptance Criteria

1. An unapproved fixture fetch fails under the app's runtime policy and never reaches its test-owned destination. The identical target succeeds and receives the expected probe after approval and fresh rendering.
2. A pre-approved target works without an approval prompt. Tests use fixture request counters and DOM results, not CSP substring inspection alone.
3. Untrusted artifact code cannot read its parent DOM, gain app credentials, read protected API data, or mutate protected app state outside an authorized bridge. Check server state for attempted mutations and report each probe independently rather than accepting a catch-all script failure.
4. The normal owner UI and flagship storage bridge still operate in the same harness. All probes run in required CI with traces and destination diagnostics on failure.
5. Tests claim only the boundaries they actually exercise; cookie host isolation requires a distinct-host setup and does not follow from distinct ports alone.

