---
id: av-qwld
status: open
deps: []
links: [av-4wyq, av-utap, av-qo05]
created: 2026-08-09T17:26:41Z
type: feature
priority: 2
assignee: Max Owner
parent: av-g2dx
tags: [frontend, gallery, account]
---
# User settings page — the surface itself

A server-rendered settings page in the shape of the existing gallery pages: stdlib html/template in internal/api/templates/, Phosphor icons, static CSS under web/gallery/, no CDN and no framework.

This story is the surface and its route, not the things on it — each of those is its own story so the page can ship with one section and grow. First occupant is account deletion; the BYO agent key is the obvious second, since it is user-level settings currently living in a modal on the agent page.

Note it inherits two invariants that now have tests: whatever credential the page embeds is decided by pagecredential.go (av-5imk), and any GET route it adds must be declared a read in csrf_test.go (av-ke2m). Both walks fail on an undeclared route, so a new page cannot ship unexamined.

