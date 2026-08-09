---
id: av-utap
status: open
deps: [av-rzvf]
links: []
created: 2026-08-09T17:39:50Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-sz4e
tags: [auth, frontend, backend]
---
# Admin: create, disable and reset other users

The administration surface Exhibit needs once it issues its own credentials: create an account, set or reset its password, disable it.

Password reset by an admin rather than by email is the specific choice that keeps SMTP out of the product. It is what Immich does, and for a household or small-team instance the admin is reachable by other means anyway.

Deliberately paired with av-g2dx's settings work rather than folded into it: that epic is a person acting on their own account, this is an admin acting on the instance. They will likely share page furniture; they must not share authority.

Note disabling must invalidate live sessions, not just prevent future logins — av-30rj made sessions server-side rows precisely so that is possible.

