---
id: av-esa1
status: closed
deps: []
links: []
created: 2026-09-07T16:08:09Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-7k7b
tags: [sharing, gallery, ui]
---
# Share panel is a modal, not a page-bottom details panel

av-6xjd shipped the owner's share panel as a collapsed <details> below the
artifact frame. It should be a modal opened from the toolbar.

The reasoning av-6xjd gave for the panel argues the other way: it noted the
detail page's full-height flex layout means a panel takes height from the frame
rather than covering it. That is the objection, not the justification. Sharing
is an occasional action on a page whose subject is the running tool, so it
should hold no layout at all until asked for.

Use the modal vocabulary the page already has. detail.tmpl carries four
capability dialogs as `<div class="modal-overlay" hidden><div class="modal"
role="dialog" aria-modal="true">`, with backdrop-click and Escape handled in
detail.js. Reuse it rather than introducing <dialog>: the styles exist, the
close plumbing exists, and a second modal idiom on one page is worse than a
slightly older one.

Two things must survive the move unchanged:

- `#share-body` keeps its htmx attributes. The swap is
  `hx-get="/partials/share-panel"` on `exhibit:shares-changed`, and htmx does
  not care whether the target sits in a modal.
- `#share-panel` stays the stable id share.js delegates listeners from, since
  htmx replaces the body underneath it.

## Acceptance Criteria

1. The detail page renders no share chrome below the frame. The panel holds no
   layout until opened.
2. A toolbar control opens it, owner-only, behind the same {{if not .ReadOnly}}
   gate every other owner control uses — not a second gate.
3. It closes on the close button, on backdrop click, and on Escape, matching
   the four capability dialogs on the same page.
4. A grant, a revoke and a link change still swap the panel body through htmx
   while the modal is open.
5. The page-script suite still passes and covers open and close.

