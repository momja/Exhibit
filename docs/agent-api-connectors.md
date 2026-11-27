# Agent-proposed advanced API connections

An agent can call `propose_api_connector` after saving its artifact. This is an
advanced option, gated by connectors, custom connections and relay support.
It supplies a name, exact HTTPS API origin, authentication type (`api-key` or
`bearer`), reviewed header and reason. It cannot supply a key, injection
recipe, artifact grant or executable setup instructions.

The proposal is immutable and durable. The typed `api-connector` interaction
stores only its ID. The server renders its canonical destination and displays
an explicit warning about malicious agent suggestions. The person enters a
key in a password field and checks **I trust [exact origin] with my key** before
choosing **Approve connection**. The server checks the exact trusted origin,
validates the credential and seals it using the existing credential vault.
Creation and the durable approval commit together. Only `approved` or `denied`
returns to Pi. Keys never enter Pi RPC, tool results, events, interaction state,
transcripts, browser drafts or URLs. Failed submissions clear the field and
require the key again. This promise applies to this private field; ordinary
chat is still chat, so the agent must never ask for credentials there.

A connector is an account, not permission for an artifact. The person still
attaches it and approves network access in Connections. There is no network
probe during setup. The existing relay validates DNS/public IPs and pins its
connection on every outbound request; redirects and credential destinations
remain governed by that relay. Keys grant whatever permissions the API provider
assigns, including writes.

Repeated proposals for the same owner, artifact, origin, auth and header reuse
one record, including a denied decision. Concurrent approvals and ordinary
native creates cannot create a second active connector for the same owner,
origin and header. An existing credential is reused, never overwritten. A
failed tool reply can retry the durable outcome without repeating creation.
Stop expires the session interaction; a stale approval cannot create anything.
A later session can request the same pending proposal. Stopped proposals remain pending and count toward the eight-proposal cap; re-requesting the same destination allows it to be declined. A proposal whose approved connection was removed reports disconnected and never silently recreates it. Decline returns a normal
negative outcome and instructs the agent to keep the app disconnected or offer
an alternative without asking again or changing destinations.

## integrations.sh investigation (2026-11-27)

The [registry's own README](https://github.com/UsefulSoftwareCo/integrations#for-agents-api--mcp)
documents public `GET /api/{domain}/surface`, `/openapi.json`, `/api.json` and MCP
discovery surfaces. A direct request to
`https://integrations.sh/api/stripe.com/surface` returned HTTP 200 and a v3
structured domain record with detected/discovered surfaces and documentation.
The [website](https://integrations.sh/) groups API, GraphQL, MCP and CLI entries.
This can help locate official API origins, authentication bindings and OpenAPI
request schemas rather than guessing them.

It is discovery data, not a destination trust authority: facts can be inferred,
LLM-discovered or stale, and schema URLs can contain untrusted text. Follow the
linked provider-owned documentation/OpenAPI document and verify its server URLs
and security schemes. Never send a credential to the registry, auto-approve its
origins, execute schema-provided code, or fetch a schema with a user's key.
The initial implementation teaches this workflow in tool/system guidance; it
does not add a runtime dependency or automatically fetch external schemas.
