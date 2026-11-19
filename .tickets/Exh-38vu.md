---
id: Exh-38vu
status: closed
deps: []
links: []
created: 2026-11-16T06:04:24Z
type: epic
priority: 2
assignee: Max Omdal
tags: [ui, render, shares, hosting, branding]
---
# Attribution on share pages: "Made with Exhibit" behind an instance flag

A share link (`/s/:shareID`) is the only page a stranger opens with no
credential, and today it is deliberately unbranded: no footer, no wordmark, no
link, and a favicon that is a monogram of the artifact's own title. That is
right for a self-hosted tool where the artifact is the user's file, and wrong
for a hosted instance, where every share a user sends is distribution and
currently says nothing about the product that is serving it.

This epic adds an optional, unobtrusive attribution line to share pages. It is
**off unless an instance turns it on**, and the artifact's owner can opt out
per artifact even then.

## Scope

1. **Instance flag**, default off — self-hosted and hosted instances both ship
   with it off; turning it on is an explicit operator decision. Parsed with the
   same fail-closed conventions as the other flags (`envBool` in
   `internal/api/publicmode.go` is the model), and documented in
   `docs/deployment.md` next to `EMBED_ORIGINS`.
2. **Per-artifact control**, tri-state, defaulting to the instance setting:
   *inherit* (default) / *show* / *hide*. Stored on the artifact, surfaced in
   the share panel where the owner makes the decision the badge depends on, and
   exposed through the API with validation in the handler and the store (the
   `share_state_mode` pattern in `internal/api/artifacts.go` + `store.UpdateArtifact`
   is the precedent).
3. **The badge itself** on `/s/:shareID` only. Not on `/a/:id`, `/w/:id`,
   version views, widget documents, or direct-open pages — those are framed by
   our own gallery, which already carries the brand.
4. **A way to settle how it is displayed before building it.** The share
   document is the artifact's own HTML in an opaque-origin sandbox, and a share
   can be framed by a third-party blog, so "just put a div at the bottom" is
   not a design — it is a decision with security and layout consequences. That
   question is a spike, and it is the first child of this epic.

## What must be true when it ships

- The badge cannot be removed, hidden, or restyled by the artifact's own script
  or CSS, and cannot break the tool it decorates.
- It never steals clicks, focus, or keyboard navigation from the artifact, and
  a full-viewport tool is still usable.
- It reads correctly in light and dark, at 320 px through desktop, and is
  screen-reader sane.
- It adds no network request, no tracker, and no third-party script.
- It is honest: the link goes to the public product origin, and the copy does
  not imply the artifact was authored by us.
- Self-hosted instances that never set the flag see byte-identical share pages
  to today (a pinned test, like the 404 page's).
- The link preview (`og:*` tags and `/s/:id/og.png`) is unchanged unless the
  spike decides the picture should carry the mark — that is its call to make,
  and it must not become a logo card instead of a picture of the widget.

## Sequencing

Child 1 is the spike. Implementation children are filed from its findings
rather than guessed now. The brand mark and its usage rules come from the brand
refresh epic (Exh-od5a) — reuse, do not invent a second wordmark.

## Acceptance criteria

1. With the flag off, share pages and their headers are byte-identical to
   today, pinned by test.
2. With the flag on, every share page of an artifact left on *inherit* shows
   the badge; *hide* removes it; *show* adds it even on an instance with the
   flag off (owner's explicit choice wins).
3. Badge is present in the server-rendered document (not added by JS), and the
   artifact cannot remove or occlude it.
4. Displayed correctly: light/dark, 320 px and desktop, keyboard, screen
   reader, and inside a third-party iframe under `EMBED_ORIGINS`.
5. Documented in `docs/deployment.md` and `docs/security.md` (what it does and
   does not reveal about a share).


## Notes

**2026-11-16T21:02:06Z**

Implementation uses a separate opaque-origin srcdoc child below a 36px top bar, rather than a shared-DOM node or mutation observer. Child DOM isolation protects the bar; arbitrary hostile tools can still navigate/blank themselves, exhaust resources, or mislead viewers; third-party framers can crop/cover the share. Cost: browsing context, escaped HTML bytes, extra focus boundary and top-level API compatibility; owners can hide it. Default remains off, explicit show/hide wins. Only live /s/:shareID consults attribution. Chromium native-anchor measurements show popup/top navigation blocked and self navigation replacing embedded tool, so framed attribution deliberately loses href; no sandbox permission is loosened. Evidence: /tmp/exhibit-review/banner/. Independent final-head visual and code/security reviews are parent-owned and still required; implementer does not merge.

**2026-11-16T23:00:40Z**

Merged origin/main before PR, retaining newer ticket history. Build and go vet pass; focused attribution/share/API/Node checks and legacy byte goldens pass. Full-suite attempts have not passed: previous real-Pi integration failures and cumulative package timeout remain recorded. The test named at the API timeout passes alone in 0.641s; stack was SQLite migration 018, not attribution code. Longer bounded API run is separately logged. Migration 055 and changed share framing require eligibility review; no autonomous merge by implementer.


**2026-11-17 — Pi visual R3 bounded response**

F1: removed only attributed `frame-src 'self'`, retaining default CSP and sandbox; renderer/browser assertions require no frame-src. F2: documents distinguish ungranted Chromium denial in both modes from origin-granted direct success/readback versus attributed NotAllowedError; WebKit writes in both modes. Focused browser assertions cover these outcomes without a bridge. F3/F5: external-link test now asserts CSP refusal (Chromium error frame, WebKit srcdoc no-op) and docs state tool loss until reload in Chromium. F4: load focus runs only for direct shares; both engines preserve an unfocused host and active host typing. Direct unclicked keyboard/Chromium autofocus, banner traversal, legacy byte pins and default-off policy remain covered. Redundant allowfullscreen removed; existing allow delegation remains.

Verification: isolated harness ports 18880/18881/18882, 17 browser passes plus one explicit Chromium-only grant test skipped on WebKit (all original 14 pass); full renderer package, focused API attribution/share tests, scoped vet, build and 17 Node share tests pass. Logs/evidence: /tmp/exhibit-review/banner/r4/. Initial browser run accidentally used old binary before build completed; interrupted log retained, not counted as passing. Full-suite prior Pi/reconciliation failures, timeouts and interrupted runs remain limitations, not passes. No new-head CI result or independent review approval claimed. Both fresh final-SHA reviews remain parent-owned. GitHub browser still signed out, so images remain local and unattached. No merge/deploy/main changes.


**2026-11-17 — Security R3 bounded implementation follow-up**

B2/H1(a): retained exactly allow-scripts allow-forms. Both-engine regressions now compare hidden data-URL download success against shown no-download (bounded 1.5s observation), and hidden confirm dialog dismissed false against shown silent false/no dialog. Security/deployment docs explicitly describe lost exports and confirmation-gated actions, and distinguish unmeasured pointer lock/orientation/presentation limits. This follows the written no-allow-downloads/no-host policy and the authorized engineering choice; no sandbox tokens or bridges added.

N1: server preview metadata emitted once at top of outer head for shown shares; hidden bytes remain pinned. Escaping test covers hostile artifact-record title, iframe name and retained tool HTML title. N3: frame lookups now resolve the owning iframe/parent; fixed embed port remains documented and isolated test copy uses 18980/18981/18982. Attribution browser suite is explicitly local-only, excluded from Go/Node CI and default browser CI.

Verification: build, complete renderer including unchanged goldens, focused API, scoped vet, 17 Node tests pass; dedicated Chromium/WebKit 21 pass / 1 existing Chromium-only clipboard-grant skip. Full lint exists and reports repository baseline findings; diff lint reports 0 new issues. Logs and screenshots: /tmp/exhibit-review/banner/r5/. Prior full manual suite failed/incomplete; no full-suite pass or new-head CI pass claimed. Screenshots local, GitHub upload remains blocked by signed-out browser. H2 external-link refusal remains an unresolved independent-review product decision; Chromium error frame/WebKit no-op behavior, docs and tests preserved. No human approval or reviewer PASS claimed; parent owns fresh final-SHA reviews and CI watch. No merge/deploy/main changes.


**2026-11-17 — Approved public artifact navigation restored**

Replaces the earlier universal external-link refusal described in historical notes. Attributed live public shares with persisted links_approved and a successful authoritative unprotected-data result now navigate their own share context on activated HTTP(S) requests. The local wrapper validates exact child source, null origin, live browser activation and credential-free bounded URLs; it has no Gallery capability/API authority. Both _self and _blank use this same share context; embedding hosts never navigate. Unapproved/ineligible clicks leave the tool intact. Hidden responses and other routes keep legacy bytes. No sandbox tokens, frame-src, popup escape, devices, downloads or modals added. Arbitrary approved-child URL messages during live activation are intentionally within this permission; exact-anchor attestation is not claimed. Missing/failed resolver and retained connected provenance fail closed. Policy/approval are render-time snapshots.

Before: current-head real-click regression fails in both engines despite links_approved=true. Test-harness activation caused the scratch no-activation failure: page/locator evaluation grants activation in Chromium and WebKit; corrected fresh-context tests observe false automatically at load before sending, then demonstrate evaluate changes it. Actual redirect server replaces unsupported WebKit mock-302 fulfillment. Final verification and new-head handoff live under /tmp/exhibit-review/banner/navigation/. Prior compatibility costs and failed/incomplete broad-suite history remain. Screenshot attachments explicitly waived by user. Parent owns two fresh independent reviews at the final pushed SHA; no technical-design human approval, reviewer PASS, merge or deploy claimed here.


**2026-11-17 — Click cancellation and legacy SVG link follow-up**

Reproduced CodeRabbit review 5935824712's capture-before-cancellation and xlink:href findings on clean 8bcb662 in Chromium/WebKit. The helper now appends one-click propagation tails so synchronous inline/target/document/window handlers can cancel before it applies native sandbox cancellation; only uncanceled eligible clicks send the wrapper request in a subsequent task. It does not patch Event.prototype or count its own preventDefault as artifact cancellation. Ordinary stopPropagation is handled; canceled stopImmediatePropagation leaves the tool intact. Artifact code that suppresses tail delivery without canceling defaults can bypass mediation, retaining native CSP restrictions; that limit is documented, not claimed as full native event parity.

Both href and SVG namespaced xlink:href now resolve with href precedence, including fragments. Real painted SVG clicks, inline return false, handlers installed during dispatch, modifiers/untrusted/credentials, ordinary navigation and all prior attribution cases pass in both engines. Full dedicated suite: 59 pass / 1 existing WebKit Chromium-only clipboard-grant skip. Build/full renderer legacy pins/focused API/vet/28 Node tests pass; diff lint reports zero introduced issues against 8bcb662. Evidence and click-fix handoff: /tmp/exhibit-review/banner/navigation/. Screenshots remain waived. Parent owns two fresh final-head reviews; prior visual PASS at 8bcb662 is old-head evidence only. No merge/deploy/main changes.
