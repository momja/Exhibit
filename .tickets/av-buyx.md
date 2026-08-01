---
id: av-buyx
status: open
deps: []
links: [av-xba2]
created: 2026-07-05T00:20:33Z
type: feature
priority: 2
assignee: Max Omdal
tags: [security, backend]
---
# Encrypt / protect artifact state at rest and in transit

Deferred from av-xba2. Artifact state (localStorage/sessionStorage synced via the storage shim) is stored in plaintext in SQLite and served by the state API. Some artifacts store secrets there (e.g. the Ticket Visualizer persists a GitHub PAT). Currently acceptable: local-only dev on a trusted network. Later work: (1) encrypt state values at rest; (2) reconsider whether GET /api/artifacts/:id/state should be publicly reachable (it was opened for the token-less shim, but the inline-state hydration fix may let it return to authenticated); (3) guidance/policy for artifacts that store credentials in browser storage. Blocks any public/multi-user deployment.


## Notes

**2026-08-01T19:16:24Z**

Deferral (2) — "reconsider whether GET /api/artifacts/:id/state should be publicly
reachable" — is already resolved on main. afb1da1 (av-xba2, "bridge state writes via
postMessage, drop CORS/public endpoint") moved writes through the host frame and
returned the state endpoint to authenticated; its message reads "Closes the
public-endpoint exposure tracked in av-buyx." Reads are satisfied by render-time
inlining, so the sandbox never needs the endpoint.

Remaining scope for this ticket is therefore (1) encrypt state values at rest and
(3) guidance/policy for artifacts that store credentials in browser storage.
