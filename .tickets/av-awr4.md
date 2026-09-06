---
id: av-awr4
status: in_progress
deps: [av-lrae, av-6axy]
links: []
created: 2026-09-06T01:33:31Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-7k7b
tags: [sharing, gallery, security, render]
---
# Recipient's read-only artifact view

Slice 3 of sharing v1: what a granted non-owner actually sees. Depends on the
grant accessor (av-lrae) and the viewer principal (av-6axy).

The recipient opens the ORDINARY artifact URL, /artifacts/:id. A grant is not a
link, so there is no second address and no second template — this is detail.tmpl
in a read-only mode, the way public mode already suppresses edit controls from
request context.

## What is removed, and why it is not a matter of hiding buttons

Only the owner modifies CSP, allowlists and capability approvals (av-7k7b,
av-6xjd). The server already enforces that through owner-scoped queries
(av-ep8k), so nothing here is a security fix. It is an honesty fix: three host
frame prompts write per-artifact authority, all three would render for a
recipient today, and all three would 404 on submit.

    network permission prompt (av-kmwj)     POST /api/artifacts/:id/origins
    download / clipboard / link first-use   PATCH /api/artifacts/:id
    camera / microphone gate                PATCH /api/artifacts/:id

Click Allow, nothing happens, and the recipient learns the tool is flaky rather
than that it is not theirs to grant.

**A recipient's session renders none of those prompts.** It gets the path
av-kmwj already built for a violation approval could not fix: explain, do not
ask. 'This tool tried to reach api.example.com. Its owner has not allowed that.'
No button.

Also absent: the edit link, the source, share controls, delete, tag and
collection controls, refetch, export decisions — everything that mutates or that
reveals the owner's library.

## The failure mode to design against

A shared artifact is frozen at whatever its owner approved. If the owner's
allowlist is missing an origin the tool needs, the recipient cannot fix it and
has to go ask. That is correct — the alternative is a recipient widening
somebody else's CSP — but it fails invisibly unless the explanation is actually
shown. A silent blank tool is the thing this ticket exists to prevent.

## Note on device capabilities

Camera and microphone still hit the browser's own native permission prompt on
the recipient's machine, so an owner's approval never hands over a stranger's
camera. It stops Exhibit blocking it; the browser is the second gate. Do not
present the owner's approval as a grant over the recipient's hardware.

## Acceptance Criteria

1. A granted recipient opening /artifacts/:id gets the artifact running in its
   frame, with their own state inlined (share_state_mode 'own') or the owner's
   ('shared').
2. A user with no grant gets the 404 page, indistinguishable from a nonexistent
   artifact.
3. The recipient's page renders no edit link, no source, no share controls, no
   delete, no tag or collection controls.
4. A CSP violation in a recipient's session raises an explanation naming the
   origin and the fact that its owner has not allowed it. No Allow, no Block
   once, no Don't ask again.
5. A download, clipboard or external-link attempt the owner has not approved
   fails the way it always has, with no prompt offered.
6. Nothing in the recipient's session issues POST /origins or PATCH
   /api/artifacts/:id. Assert it at the page-script level (web/gallery/testdom
   suite), not only by reading markup.
7. The owner's own view is unchanged in every respect.

