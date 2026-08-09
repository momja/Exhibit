---
id: av-t21v
status: open
deps: [av-rzvf]
links: []
created: 2026-08-09T17:39:50Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-sz4e
tags: [security, auth]
---
# Rate-limit the login endpoint

There is no rate limiting anywhere in the service. With av-q30x's single credential that was defensible and documented: bcrypt's cost is the throttle, a guess costs the attacker roughly what it costs the server, and an internet-facing instance was pointed at proxy-level limits.

That argument weakens once Exhibit issues credentials for several people. Credential stuffing against N accounts is a different shape from brute-forcing one, and 'put a limit at your proxy' is a weaker answer when *we* are the thing minting credentials rather than delegating.

This is the one item from av-30rj's original objection list that the operator-provisioned model does not dissolve, so it is in scope rather than deferred.

