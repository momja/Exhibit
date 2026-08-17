---
id: av-jviu
status: closed
deps: []
links: []
created: 2026-08-09T22:04:46Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-sz4e
tags: [auth, self-host, onboarding]
---
# Bootstrap a default admin account on first boot

av-rzvf provisions accounts with `user add`, which means a fresh instance requires a CLI round-trip and a restart before anyone can log in. That is friction in the first five minutes, and it makes the docs harder to read than they need to be.

Instead: on first boot, when an instance has no local accounts, create `admin` with the documented default password `changeme`, flagged admin. The operator signs in and changes it. Everything after that is `docker compose exec`.

**The trade, chosen deliberately (2026-08-09):** a fixed default is guessable from the moment the instance boots until someone changes it, and unchanged defaults are among the most-scanned-for things on the internet. A generated password printed to the log, and a first-run browser setup page, were both considered and rejected in favour of the simplest thing to read and document. Recorded so the risk is a decision rather than an oversight.

Mitigation that does not touch the flow: log a warning on **every** startup while the default password is still in place, not only when it is created.

