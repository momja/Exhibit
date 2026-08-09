---
id: av-6xjd
status: open
deps: []
links: [av-0k5q, av-20xv, av-v991, av-7k7b, av-8ipt]
created: 2026-08-09T16:34:08Z
type: feature
priority: 2
assignee: Max Omdal
tags: [sharing, frontend, gallery, design]
---
# Sharing in the UI — design and test before building

Sharing is API-only today and stays that way for the multi-user epic — a deliberate decision (2026-08-09), not an oversight. There is no share button, no list of what you have shared, and no indication on a card that an artifact is shared at all.

The mechanism is sound: `POST /api/shares` mints a row, `GET /s/:id` on the app origin redirects to the render origin, and the render surface serves it to anyone with no credentials because the share row *is* the authorization. Verified end to end against a running instance.

What is missing is the product around it, and it needs design and testing first rather than a button bolted on:

- **No affordance to create one.** A share is currently a curl command.
- **No way to enumerate what is shared.** You cannot audit what you cannot list, and the failure mode is the share made months ago that nobody has thought about since.
- **No indication on a card.** `ListArtifacts` carries no share data at all, so the gallery could not show it even if the template wanted to.
- **A share publishes the owner's state.** `ServeShare` inlines it deliberately ('a share publishes the artifact as its owner sees it'). Fine for a stateless tool, not obviously fine otherwise — av-7k7b owns that question.

Blocked on nothing technically. Deliberately not started: the interaction design and the state question should be settled before any of it is built.

