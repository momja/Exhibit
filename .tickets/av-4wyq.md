---
id: av-4wyq
status: open
deps: [av-7jcq, av-qwld]
links: []
created: 2026-08-09T17:26:41Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-g2dx
tags: [multi-user, account, privacy]
---
# Delete my account and library

A person signing in through an identity provider has no shell on the host and no relationship with the operator. Deleting their own library has to be something the product does.

Blocked on two things that are genuinely prior: the settings surface to invoke it from, and `Blob.Delete` — without the latter, 'delete' leaves every artifact body on disk.

