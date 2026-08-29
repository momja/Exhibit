---
id: av-kmwj
status: open
deps: []
links: [exhibit-fr7]
created: 2026-08-29T17:55:52Z
type: feature
priority: 2
assignee: Max Omdal
tags: [security, ui, render]
---
# Revive exhibit-fr7: runtime network-permission prompt orphaned off main

exhibit-fr7 ("prompt for CSP-blocked origins in trusted app chrome") was built
and merged in PR #64 -- but stacked on feature/exhibit-x87/network-origins,
which had already been merged to main one minute earlier via PR #63. Nobody
opened a follow-up PR bringing the combined x87+fr7 state back into main, so
fr7 actual code never reached main. The ticket was marked closed because the
PR merged, which was true of the wrong branch. Full history: bbe4c2a
(x87, in main) vs. a11cca2 (x87+fr7, only on the orphaned branch,
git diff bbe4c2a a11cca2 recovers the isolated fr7 change, 15 files, 653
insertions).

What is still true on main: the data model fr7 needs is already there via x87
directly -- artifact_network_origins, DecisionAllow/DecisionBlock,
Store.ListOriginDecisions/SetOriginDecision/DeleteOriginDecision all exist and
are unused for this purpose. docs/security.md still says "a runtime approval
prompt is tracked by exhibit-fr7" in future tense, which is accurate today.

What is missing, concretely:
- internal/api/origins.go: a per-origin POST/DELETE /api/artifacts/:id/origins
  route (main only has whole-allowlist replacement via PATCH
  /api/artifacts/:id; that path deliberately never touches block rows it
  did not display, per TestPatchAllowlistPreservesBlockDecisions).
- A securitypolicyviolation listener in the render preamble's shim, reporting
  the blocked origin and violated directive to the host frame via postMessage.
  Main's shim has every other bridge (state, download, clipboard, link) using
  exactly this ARTIFACT_ID/API_ORIGIN/postMessage(..., API_ORIGIN) idiom, just
  not this one.
- The modal itself: Allow / Block once / Do not ask again, in trusted app
  chrome (detail.js/detail.tmpl, which still owns the live iframe today --
  agaf-02xs removed the raw-source pre-tag source dump from the detail page,
  not the iframe).
- A Forget control on a blocked origin (edit.js), so "do not ask again" has a
  way out.

Not a clean cherry-pick: internal/render/render.go injectShim was renamed to
injectPreamble and gained parameters (widget, anonymous) since July. The port
needs hand-adapting against the current signature and call sites, the same
way av-hrtv was just hand-ported after av-e0yj changed what it touched.

## Design

Start from the recovered diff (git diff bbe4c2a2b24bbc6d5b12bdd62455a1fbb5b87b09
a11cca2472811fb3fbf88df794ef6c5497dad276) as a reference, not a patch to
apply -- main has moved through av-x01o (worker-src CSP fix), av-mdc5 (link
bridge), av-i7hd (origin normalization + owner-scoped store), agaf-02xs
(detail page restructuring), and the multi-user/owner-scoping work since. Read
the recovered diff five pieces (origins.go route, render.go shim listener,
detail.tmpl+detail.js modal, edit.js Forget control, docs) and re-derive each
against current main shape rather than merging mechanically:

- internal/render/render.go: add the securitypolicyviolation listener block
  to the current shimTemplate; thread a "blocked origins" list through
  injectPreamble current signature (body, artifactID, appOrigin, state,
  widget, anonymous) so an already-refused origin is not re-reported.
- internal/api/origins.go (new): POST/DELETE /api/artifacts/:id/origins,
  writing through Store.SetOriginDecision/DeleteOriginDecision (already
  owner-scoped on main -- the old route predates that and will need the
  ownerID plumbing every other handler already has).
- web/gallery/detail.js + detail.tmpl: the modal, gated on
  e.source === frame.contentWindow like every other bridge in that file.
- web/gallery/edit.js: list block rows with a Forget action.
- docs/security.md, docs/architecture.md, docs/api.md: fr7 original diff
  touched these; reconcile against current text rather than reapplying blind
  (docs/security.md "is tracked by exhibit-fr7" line is what needs updating
  once this actually ships).

Explicitly out of scope for this ticket: the agent chat page preview pane
(agent.js) has no capability bridges at all yet (download/clipboard/link),
only the state bridge and snippet capture -- so it has no runtime network
prompt either, but giving it one is a separate, larger gap than reviving fr7
on the page it was built for. Note it, do not fix it here.

## Acceptance Criteria

1. A rendered artifact's request to an origin not on its allowlist is
   detected client-side (securitypolicyviolation) and reported to the host
   frame -- not silently dropped, as it is on main today.
2. The detail page prompts in trusted app chrome (never inside the artifact's
   own frame) with Allow / Block once / Do not ask again.
3. Allow writes an allow decision and transparently reloads the iframe so the
   widened CSP takes effect and the request retries, with no manual refresh.
4. Do not ask again writes a block decision; it is inlined into the next
   render's preamble so the same origin is not re-prompted, and it never
   widens the CSP.
5. A block decision is revocable (Forget) from the edit page's origins
   editor.
6. Each origin is reported at most once per page load, so a retry loop cannot
   spam the prompt.
7. A test pins that this reaches main this time: an in-repo test (not a
   manually-verified branch) exercises the report, prompt, allow, reload
   path, since that is exactly the step that was skipped last time.

