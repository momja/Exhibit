---
id: exhibit-fr7
status: in_progress
deps: [exhibit-x87]
links: [av-jafp, av-isb3, av-mf1x, av-41se]
created: 2026-07-01T05:23:56Z
type: feature
priority: 2
---
# Runtime network-permission prompt + 'don't ask again' blocklist (spec §6.2 step 4)

When a rendered artifact attempts an origin not on its allowlist, the browser CSP (connect-src etc.) blocks it and fires a securitypolicyviolation event; today nothing prompts the user (spec §6.2 step 4). Add a runtime permission prompt: detect the blocked origin, ask the user in trusted app chrome, and on approval grant it and transparently reload so the request retries. Provide 'Don't ask again' which records an explicit block (decision='block', source='runtime_prompt') so the origin is not re-prompted on future loads. Depends on the artifact_network_origins data model (exhibit-x87): allow decisions drive the CSP; block decisions only suppress re-prompts and are surfaced for revocation.

## Design

Detection: inject a securitypolicyviolation listener into the render doc (render.injectShim is the existing pre-artifact injection point); capture blockedURI origin + violatedDirective; dedupe. Bridge: the iframe is sandbox=allow-scripts with NO allow-same-origin (opaque origin), so the prompt MUST live in the parent app, not the artifact DOM (untrusted/spoofable). The iframe posts to window.parent via postMessage; the parent validates event.source===iframe.contentWindow (event.origin arrives as 'null'). Prompt UI (parent chrome) shows the specific origin+directive with Allow / Block once / Don't ask again. Allow: SetOriginDecision(allow, runtime_prompt) then TRANSPARENT reload — parent re-assigns iframe.src with a brief 'applying…' state so the new CSP is delivered and the artifact re-runs its request; no manual refresh (user confirmed reload is acceptable if transparent). Block once: dismiss, will prompt again next load. Don't ask again: SetOriginDecision(block, runtime_prompt). Suppression: ship the artifact's block set to the render client at load (inject alongside the shim) so repeat violations for blocked origins are not re-surfaced. Revocation: extend the detail-page origins editor to list block rows and allow removing them, else a block is a permanent trap. Inherent caveat: CSP is a response header fixed at load, so approvals cannot apply live — hence the reload. Out of scope: the standalone/share top-level render view has no trusted app chrome to host a prompt; those stay silently blocked (track separately if wanted). Acceptance: a blocked fetch surfaces a prompt in app chrome; Allow adds an allow row and the retried request succeeds after transparent reload; Don't-ask-again adds a block row and no re-prompt occurs on reload; block rows never widen the CSP; blocks are revocable from settings.



## Notes

**2026-07-20T02:12:00Z**

Linked to av-jafp (PR #61) — confirming direction, not changing scope:
this ticket's Allow/Block once/Don't ask again runtime prompt is already
modeled on the same host-mediated first-use-approval pattern as the
existing downloads_approved/clipboard_approved capability bridges (see
docs/architecture.md §3.2/§6, and the modal pattern already shipped in
the app — previously #dl-modal/#clip-modal in detail.tmpl, now owned by
the Edit page's security panel after av-jafp/av-hwx2 made the viewer
read-only). When this is built, reuse that modal/approval idiom rather
than inventing a new one.

Also see the note just added on exhibit-x87 (this ticket's blocker): the
Edit page's "Referenced, not approved" section (av-p0a1) will need
updating alongside x87's schema migration so a decision='block' origin
(set here via "don't ask again") doesn't render identically to a
never-decided one on the Edit page — flagging now so that UI update isn't
missed when x87 lands.
