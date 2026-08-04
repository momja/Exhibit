---
id: av-rgp1
status: in_progress
deps: []
links: []
created: 2026-08-03T05:02:10Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, agent, api]
---
# SSE stream puts the API bearer token in the URL query string

GET /api/agent/sessions/{id}/events accepts the service bearer token as ?token= (internal/api/agent.go:281-291), and the chat page sends it that way (web/gallery/agent.js:209). The reason is real — EventSource cannot set an Authorization header — but the consequence is that the instance's single master credential travels in a URL, and URLs are logged and stored in places request headers are not:

- This service's own request log writes r.URL.RawQuery whenever DEBUG/LOG_LEVEL=debug is on (internal/logging/logging.go:138) — the token lands in the log in cleartext, and debug is exactly what an operator turns on when the agent surface misbehaves.
- Any reverse proxy in front logs the query string by default; nginx's $request and Caddy's access logs both include it. deployment guidance explicitly leaves the proxy to the operator, so this is out of our control once emitted.
- Browser history and session restore keep the URL.

Because the token is per-instance and never rotated, a single stale debug log or proxy log is full API access — every mutating route in internal/api/api.go:78-135.

Two smaller defects in the same handler: the comparison is a plain != on the token (agent.go:287, same in internal/api/middleware.go:22), so it is not constant-time; and there is no owner check on the resolved session (agentSession, agent.go:265-276, ignores Session.OwnerID) — harmless while owner_id is pinned to 1, but this is the multi-user seam the architecture claims is additive, and a session id alone should not be a capability.

## Design

Keep the master token out of URLs entirely. Options, cheapest first:

1. Session-scoped SSE ticket. POST /api/agent/sessions already returns the session id over an authenticated request; return a short-lived (seconds), single-use, session-bound random ticket with it and have the events route accept only that. Compromising a log line then yields at most a replay window on one session's event stream, not the library.
2. Drop EventSource for fetch() + ReadableStream, which can set an Authorization header. Costs the automatic reconnect EventSource gives (agent.js:215) — the backlog replay in Session.Subscribe (internal/agent/agent.go:295-311) already makes reconnect cheap to reimplement, so this is a modest amount of code and removes the special case at the routing layer (api.go:65-69).

Either way, use subtle.ConstantTimeCompare for whatever secret the route does compare, and apply it to authMiddleware too. While in this handler, compare Session.OwnerID against the request's owner in agentSession() so the fix is already in place when real identities arrive.

## Acceptance Criteria

1. No request the chat page makes carries the service token in a URL. Verified by asserting the SSE request URL contains no token-shaped value.
2. With LOG_LEVEL=debug, no log line contains the service token.
3. If a ticket/handle scheme is used: it is single-use, expires in seconds, is bound to one session, and a replayed or foreign ticket is rejected — each covered by a test.
4. Token/ticket comparison is constant-time here and in authMiddleware.
5. agentSession() rejects a session whose OwnerID does not match the request's owner.

