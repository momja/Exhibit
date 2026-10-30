---
id: Exh-qr6e
status: closed
deps: [Exh-spg4]
links: []
created: 2026-10-22T06:50:51Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, security]
---
# [Experimental] Phase 5: stand-in tokens for clients that hold credentials

EXPERIMENTAL; part of Exh-g2vv.

For recipes whose ordinary client code expects to hold a credential value (an access URL with embedded credentials, SDKs that require a key). This is the Meta Muse pattern: the runtime sees only stand-ins, and an egress process substitutes the real secret per request.

- **Binding:** each stand-in is bound to (viewer, connection, allowed origins). Substitution happens only in recipe-designated positions (the Authorization header including Basic-decoded user:pass, or named query parameters), never anywhere in a URL or body. Otherwise there's a reflection attack: a stand-in placed in a field the API echoes back would come back as the real secret.
- **Scrubbing:** if the real secret appears in a response, replace it with the stand-in. Refresh-token rotation happens inside the recipe.
- **Misuse:** a stand-in seen going to a non-recipe origin blocks the request and alerts the owner, since it's a strong exfiltration signal.
- **Out of scope:** stand-ins in exported state are inert by design (reconnect locally). Signing APIs (SigV4, HMAC) can't use stand-ins; their recipes sign in Exhibit as first-party code.

## Acceptance Criteria

- Ordinary client code that parses an access URL with embedded credentials works unmodified with a stand-in URL.
- The echo-reflection test proves no real secret is returned.
- The exfiltration test blocks and alerts.


## Notes

**2026-10-22T16:01:53Z**

Implemented expiring viewer/connection/origin-bound stand-ins, reviewed header/Basic substitution, misuse refusal and activity alert, inert exports, and reflection scrubbing with preallocation size bounds. Stand-in, echo, exfiltration and resource-bound race tests pass.
