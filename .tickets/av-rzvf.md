---
id: av-rzvf
status: in_progress
deps: []
links: []
created: 2026-08-09T17:39:50Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-sz4e
tags: [auth, schema, backend]
---
# Credentials and roles on the users table, with a bootstrap admin

The schema half. `users(id, external_id UNIQUE, email, created_at)` gains a nullable `password_hash` and a role/admin marker.

Nullable matters: an OIDC identity has no password and must stay valid in the same table and the same `owner_id` space, so the two kinds of user differ by which columns are populated rather than by living apart.

Also: the first user on an instance becomes admin. That is continuous with existing behaviour — deployment.md §3.4 already tells operators to complete the first login themselves because that identity adopts owner 1's library — so 'first in is the admin' is the same rule stated once more rather than a new one.

`LOGIN_USERNAME`/`LOGIN_PASSWORD_HASH` (av-q30x) is retained as bootstrap and break-glass rather than replaced: it seeds the first admin on an empty instance, and it is the way back in after locking yourself out. Decide explicitly whether it stays live permanently or only while `users` is empty, and record why.

