---
id: Exh-bs9j
status: open
deps: []
links: []
created: 2026-11-26T01:37:07Z
type: chore
priority: 2
assignee: Max Omdal
tags: [mobile, ios, qa, accessibility]
---
# Test artifact behavior and zoom UI on iOS with large accessibility text enabled

Manual QA pass on a physical iOS device with the largest Dynamic Type size enabled (Settings > Accessibility > Display & Text Size > Larger Text, slider at maximum). Verify that artifacts still behave correctly and that the zoom UI remains usable and legible: opening and navigating artifacts, the artifact viewer chrome, zoom controls and their labels at maximum text size, layout at the smallest and largest supported viewports, and no clipped or overlapping controls or horizontal overflow. Confirm the artifact surface itself is unaffected by the host page's text-size setting. Note that this is a verification chore, not a redesign: file separate feature/bug tickets for any defect found rather than fixing it here.

## Acceptance Criteria

Each checklist item is exercised on a real iOS device (not only the simulator) at maximum Dynamic Type and recorded as pass or fail in a note on this ticket. Screenshots capture the zoom UI and an open artifact at maximum text size, saved outside the checkout. Any failure has a linked follow-up ticket with reproduction steps, device/iOS version and dynamic type size. Ticket is closed once the results and follow-up links are recorded.


## Notes

**2026-11-26T21:55:24Z**

### Artifact bug prevention investigation

The owner clarified that the reported problem was in the artifact, and that a new artifact version fixes it. Treat that as an artifact defect rather than evidence for changing Exhibit's viewer. The original physical-device QA checklist above remains separate; this investigation does not satisfy it or close the ticket.

**Evidence and limits.** On 2026-11-26 the [shared Groceries artifact](https://artifacts.dizzard.net/s/371ddbb3-1fd2-40d8-919c-b9d1f5a80334) served version 28 (document title `List`). Its current source measures the bottom composer with `ResizeObserver` and adjusts its position using `visualViewport`. These are observations of the fixed version, not a verified before/after explanation. The ticket does not record the original symptom. The versions API returned 401, and the browser profile reached a sign-in page, so the preceding source and generation transcript were unavailable. The owner has been asked for the exact failure and fixing version. No claim is made that the current Exhibit prompt originally generated the defective code.

**Confirmed system gaps.**

- Shared [artifact guidance](../internal/agentguidance/artifact_format.tmpl#L2) covers state, dependencies and light/dark mode, but has no responsive-layout, text-enlargement or keyboard guidance. Both [Pi](../internal/agent/prompts/role.tmpl#L4) and [MCP](../internal/mcpserver/prompts/instructions.tmpl#L6) include it, making it the appropriate shared home for artifact authoring guidance.
- The built-in agent has source/state/widget/selection/question tools, but no browser inspection or execution tool. Pi starts with [built-in tools disabled](../internal/agent/agent.go#L356); the [save tools](../internal/agent/ext/exhibit.ts#L311) return persistence and network information, not evidence of correct rendering or interaction. A preview updating for the user does not mean the agent inspected it.
- Existing [scanner warnings](../internal/scanner/warnings.go#L14) are networking heuristics. They provide a useful advisory-report pattern, but are not layout validation.
- Existing enlarged-text tests exercise [Exhibit-owned pages](../internal/api/readability_browser_test.go#L50), not arbitrary generated artifacts. One [composer regression](../internal/api/readability_browser_test.go#L225) already checks that enlargement actually applied before checking bounds; that is worth reusing conceptually.

**A directly measured testing blind spot.** In the current artifact, temporarily changing the root font from 100% to 200% changed the root from 16px to 32px, but left the input at 17px and Add label at 15px. The same unchanged sizes occurred at 1280x800 and 390x844. Aisle labels, suggestion chips and explanatory text also stayed unchanged in the desktop probe. The root style was restored; no artifact source or stored data was saved. These were Chromium checks with a desktop pointer, not iOS Dynamic Type or software-keyboard tests. A future artifact test must verify that its intended text enlargement actually happened; simply setting the root to 200% and checking for overflow can give false confidence.

**Recommended first change: a short shared prompt addition.** Proposed text for `artifact_format.tmpl`, not applied:

> Build for the artifact's available viewport, including narrow phone screens and enlarged text. Prefer normal document flow; let labels wrap and controls and containers grow with their contents. Keep essential actions reachable when text grows or a software keyboard opens. If controls are fixed to an edge, reserve space from their actual rendered size rather than a guessed height. Keep browser zoom available unless the user explicitly requests otherwise; do not hide clipping by suppressing zoom or text enlargement. Review these risks before saving, and only claim browser or device testing that you actually performed.

This improves source review without pretending the agent has a browser. If this must also apply under operator role overrides, account for [prompt composition](../internal/agent/prompt.go#L133): the role (including the shared template) is replaceable today. Widget-only guidance must retain its different sizing contract. Do not silently redesign every existing artifact during an unrelated edit.

**Recommended harness follow-ups, in order.**

1. Add advisory artifact-quality warnings to create/read/edit/write results in both Pi and MCP. Start with readily identifiable zoom restrictions (the current artifact has `maximum-scale=1`, `user-scalable=no`, restricted touch gestures and Safari gesture cancellation). They are review signals, not a diagnosis of this incident. Fixed positioning alone is legitimate and should not be an error. Do not reject imports or rewrite user source automatically; report findings separately from networking warnings and permissions.
2. Add a bounded `check_artifact` tool, scoped to the session's artifact. Return screenshots plus structured JS exceptions, failed/blocked resources, document overflow and visible-control bounds/hit testing at desktop and narrow widths. Exercise focused fields, long content and genuine text enlargement, not just the initial empty screen. Include the exact source revision and tested conditions; report unavailable checks as unavailable. Feed findings back into a bounded repair/check loop, and distinguish a successful save from a passed check. A screenshot alone is not a functional pass, and geometry cannot generically prove every artifact's intended behavior.
3. Use an ephemeral copy of source and representative state, with in-memory writes and restricted or mocked external effects, rather than driving a live artifact with owner credentials. Reuse [Chromium context isolation](../internal/chromium/chromium.go#L66) and [network confinement](../internal/chromium/chromium.go#L104). Existing [link preview rendering](../internal/ogsnap/ogsnap.go#L65) is an infrastructure precedent, not an artifact QA implementation. It renders only a small widget; its restrictions do not by themselves simulate an authenticated artifact. [Version rendering](../internal/render/render.go#L1266) provides a precedent for nonpersistent storage, not a ready-made full validation environment. Restrict relay/AI/connector effects as well as direct network access.
4. Keep real iOS/Safari testing for claims about Dynamic Type, focus zoom and keyboard positioning. Desktop viewport resizing, Chromium touch emulation, page zoom and root-font changes are different probes and should be reported separately. For artifacts with absolute font sizes, add a deliberate text-only stress mode and verify computed sizes changed; label it as synthetic rather than an exact iOS simulation.

**How to judge whether prevention works.** Obtain the original failing source, exact trigger and fixing diff; minimize them into a fixture that fails before and passes after the artifact fix. Add benign fixtures so warnings and checks do not reject intentional horizontal scrollers or legitimate fixed controls. Verify new guidance reaches both Pi and MCP with their existing prompt tests, then compare artifact generations under old/new prompts against behavioral checks. Prompt snapshots alone prove delivery of instructions, not a reduction in defects. No original-bug regression result is claimed here because the failing source and exact symptom were unavailable.

Only this investigation note is persisted. No application prompt, harness, production artifact, stored data or permissions were changed.
