# Experimental API relay and connections

Artifacts keep executing in the browser. An optional relay performs bounded
HTTPS requests, and reviewed connector engines inject credentials outside the
artifact. The runnable examples are in
[examples/connectors](../examples/connectors/README.md). This is experimental
functionality with normal isolation, persistence and configuration behavior.

## Operator setup

Every phase is off by default. The relay requires a signed-in browser session
or the instance's service token; anonymous pass-through use, public-link
visitors, top-level render pages and widgets cannot spend relay authority.
Use the existing app/render origins, database, blob store and server secret.
For a separate development instance on T3 Code:

```bash
DATA_DIR=/tmp/exhibit-connectors \
ADDR=:18080 RENDER_ADDR=:18081 \
APP_ORIGIN=http://t3code:18080 RENDER_ORIGIN=http://t3code:18081 \
AUTH_TOKEN=your-dev-service-token \
RELAY_ENABLED=true CONNECTORS_ENABLED=true RELAY_SCHEDULES_ENABLED=true \
go run ./cmd/server
```

Production uses the normal two HTTPS origins and authentication configuration.
`EXHIBIT_SECRET`, or the generated `secret.key` beside the database, encrypts
credentials and connector response caches with the same AES-GCM facility used
for agent keys. Keep the secret with backups; changing or losing it prevents
decryption of existing connections.

| Variable | Purpose |
| --- | --- |
| `RELAY_ENABLED` | Enable the authenticated API relay and its permission UI. |
| `CONNECTORS_ENABLED` | Enable generic native API key, Bearer and Basic connections. |
| `CONNECTORS_OAUTH_ENABLED` | Enable the reviewed native GitHub OAuth flow. |
| `CONNECTORS_VENDOR_ENABLED` | Enable operator-reviewed managed OAuth providers. |
| `CONNECTORS_STANDINS_ENABLED` | Enable short-lived placeholder credentials for clients that require a token value. |
| `RELAY_SCHEDULES_ENABLED` | Enable declarative fetch-to-state refreshes. |
| `RELAY_ALLOW_ORIGINS` | Optional operator ceiling, as comma/space-separated exact origins. |
| `RELAY_DENY_ORIGINS` | Operator origin denylist; denial wins over artifact approval. |

Connector subphases require `CONNECTORS_ENABLED`; schedules require the relay.
Origins and viewer grants remain necessary after enabling any flag. No
pre-existing allow row silently becomes a relay or connection grant.

For GitHub, set `CONNECTORS_GITHUB_CLIENT_ID` and
`CONNECTORS_GITHUB_CLIENT_SECRET`, then register
`APP_ORIGIN/api/connections/oauth/github/callback` with the OAuth application.
The flow binds expiring, one-shot state and PKCE to the initiating browser;
tokens and refreshes remain server-side. `CONNECTORS_GITHUB_SCOPES` selects the
operator's reviewed scopes. Endpoint overrides exist for fixture and enterprise
configuration and remain HTTPS-only.

For managed OAuth, set `CONNECTORS_NANGO_SECRET_KEY`, an instance-specific
`CONNECTORS_NANGO_NAMESPACE`, and a reviewed `CONNECTORS_NANGO_PROVIDERS` JSON
object, for example:

```json
{"slack":{"name":"Slack","origins":["https://slack.com"]}}
```

`CONNECTORS_NANGO_BASE_URL` defaults to `https://api.nango.dev`.
`CONNECTORS_NANGO_IDLE_DAYS` defaults to 30. The provider list is operator policy,
never agent-authored recipe code. Completed managed connections are checked
against the current user before storing their identifier. Idle managed
connections are pruned. Exhibit's entitlement system limits native, OAuth and
managed connections separately; the UI reports availability and remaining
allowances. No external vendor account or live consent is assumed by tests.

Nango manages credential acquisition and refresh. Exhibit retrieves credentials
and sends requests through its own gate; it deliberately does not use a vendor
proxy, because redirect and dial-time checks must apply to every provider hop.

## Using a connection

Open **Connections & relay** from an artifact, or visit `/connections`. Add a
named connection in Exhibit's app chrome. Generic connections choose an exact
HTTPS origin and a reviewed credential position; API key headers cannot control
Host, cookies, browser identity or transport framing. Basic accepts either an
origin, username and password or a pasted HTTPS URL with embedded credentials.
The latter is split into origin, credentials and a private base path. Credentials
and the base path stay encrypted; artifact paths are relative to the saved base
URL. A private base ending in `/private/api` and an artifact request for
`/accounts` becomes `/private/api/accounts` at the provider. Safe metadata and
stand-in access URLs expose only the origin and a virtual `/` root. Exhibit
does not redeem setup tokens or make a claim request. Redeem any token outside
Exhibit and paste the resulting URL only into the trusted connect UI.

Each generic connection can set a maximum number of provider requests per UTC
day and a GET response reuse interval in minutes. Zero or blank means no daily
limit or no reuse. The relay reserves quota before contacting the provider and
stores reused responses encrypted. Both controls are shared by every artifact
and device using the connection; cache hits spend no additional quota. Redirect
hops each reserve a request. Cache keys distinguish forwarded representation
headers; partial and uncacheable responses do not replace a full response.

The generic-connection migration converts earlier experimental native records
with a Basic authorization recipe by their stored shape. It retains their IDs,
encrypted credentials, private endpoint mapping, quota/cache settings, grants
and saved-data restrictions. Existing artifacts continue using their public
paths; no reconnect is required.

Grant the saved connection to the artifact, review the generated capability and
data-destination text, then enable relay reads. API changes require the separate
write grant and a connector recipe that permits writes. Every connection defaults
to its own origin and nowhere else, widened only by explicit approval. Its
effective network policy also
restricts other browser egress paths, not just `fetch` through the relay.
Changing a grant invalidates the app's previous render authority; reopen the
frame under the current policy before receiving connected data.
Removing an artifact grant or revoking a connection stops credential use while
retaining its saved-data destination policy. Clear the saved data and release
that policy explicitly in Connections before widening destinations again.

Connections belong to the current viewer. A recipient uses their own connection
unless the owner explicitly shares a connection and its data. Public links do
not use the relay. Shared-state/public-link artifacts require explicit owner
sharing consent before attaching a connection because saved response data is
part of the artifact's state. Sharing or exporting saved data is distinct from
sharing a credential.

Artifact code uses ordinary HTTPS `fetch` with no credentials. Safe granted
metadata is available from `await window.exhibit.connections.list()`, including
provider, origins and any public endpoint. Models can call `list_connections`
and `request_connection`; requests appear in Exhibit and grant nothing until a
person completes them. Agents cannot acquire credentials, submit recipes, edit
grants or retrieve secrets.

When enabled, `window.exhibit.connections.get(connectionId)` obtains an inert,
expiring stand-in. The relay accepts it only for its viewer, connection,
designated header and reviewed origin; misuse is refused. Stand-ins do not
replace the destination policy and should never be persisted in source, state,
widgets or transcripts. Native attach-by-origin remains the simpler path.

## Request boundary

The frame uses a document-bound host bridge; the host pins the artifact and
viewer. Its app-owned authority for the render's current policy is absent from
artifact source and state. Responses are returned as bytes to the opaque frame,
where the shim constructs a `Response`; host code never inserts provider bytes
into its DOM. A new grant cannot empower a document rendered under an older,
broader policy. Viewer authentication and the same policy checks apply to direct
relay API calls.

The host also checks external-link destinations against the current data policy,
including after first-use approval, and binds state resyncs to that policy.
Private connected documents open only inside Exhibit, with an opaque sandbox
and an exact parent frame policy. Their URLs and signed token scopes include an
opaque policy fingerprint, so a cleared or changed policy invalidates old URLs
before source or saved data is served. Artifact frames cannot use the app's
fresh-token routes to escape that binding. Gallery widgets keep showing their saved data
inside a script-free app-owned wrapper that pins navigation to that widget's
render path. An anonymous public view without a connection data policy keeps
its existing approved-link behavior. Explicit public shares remain public only
after the owner approves sharing for every connection data policy.
HTTPS previews require the browser's iframe Fetch Metadata header. On plain
HTTP development origins, where browsers omit that header, the browser's
referrer must instead name the exact trusted app origin. The trusted widget
wrapper sends only that origin; render documents still send no referrer.

`internal/egress` enforces all of the following for native and managed
connections, ordinary relay requests, acquisition flows and refreshes:

- HTTPS only; exact normalized origins on the initial URL and every redirect.
- Public IPs only. Legacy numeric IP spellings, loopback, private, link-local,
  metadata, shared-address, reserved and transition ranges are refused without
  an approval option. Every DNS answer is checked; dialing uses a checked
  literal and cannot resolve the hostname again during the connection.
- No environment proxy, cookie jar, browser cookies, guest Authorization,
  Origin, Referer, Host, forwarded identity or arbitrary custom request headers.
  A small allowlist covers negotiation, content type, conditional requests and
  ranges. Trusted credential injection runs afresh for each checked hop.
- One MiB request bodies, four MiB decoded responses, 64 KiB guest headers,
  five redirects and a fifteen-second request deadline. Gzip and deflate are
  decoded before the response byte limit is applied.
- Safe response headers only. Set-Cookie, hop-by-hop fields and upstream CORS
  policy are not delivered. The User-Agent identifies `Exhibit-Relay/1` and the
  instance URL.

Relay token buckets apply per owner, artifact and target origin. Provider 429
responses impose instance-wide origin back-off using `Retry-After`, including
redirect destinations. Credential-free GET responses are cached only with
explicit public `Cache-Control` freshness, scoped to viewer, artifact and policy;
the cache caps TTL at five minutes and memory at 16 MiB. Generic connections
add their user's persistent daily limit and encrypted response reuse policy,
so repeated page opens need not spend provider quota. Connector responses scrub
reflected credential material before delivery. Activity and relay logs contain
only owner/artifact, origin, method, status, bytes and latency: no bodies, access
URLs, request paths or query strings.

## Scheduled snapshots

The artifact's connection page can schedule a credential-free GET URL, a saved
state key and an interval of one hour to one year. A stable random initial
offset spreads provider traffic; workers claim due rows and recheck access,
entitlements, origin approval, relay grants and connector policy at execution.
Only a successful UTF-8 text/JSON response of at most one MiB replaces the key;
errors preserve the previous value. A worker never interprets JavaScript,
templates or provider responses as code. Widgets only read saved state.

The Connection Check example can save a scheduled HTTPbin response to its shared
app/widget state key. Use a credential-free URL at the granted public origin;
the relay adds any granted connection's credential when the schedule runs.

## Portable exports

**Download app** exports a ZIP with `index.html`, `state.json`, `exhibit.json`,
the browser bridge, `serve.mjs` and a README. Its manifest contains allowed
origins and nonsecret connection requirements, with no original connection IDs,
access paths or credentials. The single-file HTML export lists origins that
still require a relay. Neither form silently carries an Exhibit grant.

Unpack and run `node serve.mjs`, or build `go build ./cmd/exhibit-run` and run
`exhibit-run app.zip`. The Go runner also accepts a directory. Node is covered
by the parity tests; Bun and Deno may use their Node compatibility support.
Saved data persists locally. ZIP-based Go runs write a `.state.json` file beside
the ZIP. Use the local app's **Connections** page to re-enter API keys, bearer
tokens, Basic credentials or a pasted Basic credential URL. Local secrets live
in runner memory until it stops; `EXHIBIT_CONNECTIONS` can supply local generic
credentials as JSON instead. OAuth/managed exports reconnect as local bearer
requirements rather than exporting an existing provider token or vendor binding.
Local stand-in and OAuth acquisition flows are not exported.

Both runners use the same origin, redirect, header and private-address rules.
The Go runner reuses `internal/egress`; the zero-dependency Node runner pins
checked DNS literals and caps decompression. Both render artifact code in an
opaque frame, including direct visits to the render URL. Exact Host checks,
Origin checks and a per-session CSRF token protect local mutation and relay
endpoints. The host permits only local frame navigation, and connection changes
rotate the render policy token before new data can be returned. Connection
requirements narrow the frame CSP, and a nonsecret `state.json.policy.json`
sidecar retains local data restrictions after disconnect, credential
replacement and runner restarts. Response bytes never enter the host DOM.
The listener defaults to loopback. An explicitly exposed listener can configure
an additional exact authority using `EXHIBIT_PUBLIC_HOST` or `-public-host`.

## Backend decision and validation

This implementation supersedes the synchronous artifact backend POC in
[Exh-co2m](../.tickets/Exh-co2m.md) for external API and account access. No
QuickJS/WASM runtime, artifact server script or Workers-style handler is shipped
here. Browser execution remains the model. Declarative schedules cover fetch and
cache refreshes; browser automation, inbound webhooks and shared server authority
remain separate future design questions rather than reasons to execute arbitrary
artifact code now. This also records the reconciliation direction for
[Exh-h5va](../.tickets/Exh-h5va.md).

Validation includes hostile headers, redirect credential isolation, numeric and
mapped IP spellings, private/mixed DNS answers, rebinding-resistant literal
dialing, decoded-size limits, quotas, revoked grants, account deletion and
encrypted secrets. Standalone tests exercise a real Node process and Go HTTP
handler against TLS fixtures, including generic Basic URL parsing, local reconnects,
cache behavior and stale render policies. Real provider accounts were not
available; a passing fixture test is not a claim of live account access.

The integrated example was also exercised in the browser: a bearer connection
created through the app's form authenticated against HTTPbin, with its reflected
token scrubbed before delivery and state persistence. Fieldnotes returned live
arXiv results. A due scheduled HTTPbin fetch saved an authenticated, scrubbed
snapshot with no artifact page open, and the connected gallery widget rendered
that saved state. The actual downloaded Node bundle reconnected through its
local form and authenticated independently against HTTPbin. Desktop, mobile and
dark configuration views were checked.

Validation commands: `make assets`, `go test ./...`, and `go vet ./...`.
Security checks include the real Chromium bridge/widget tests and race tests
for the egress gate, connector engine and standalone runners. Review images are
in [screenshots/Exh-g2vv](screenshots/Exh-g2vv).
Live generic Basic account access, registered GitHub OAuth, Nango staging costs and one live model
generation require external credentials; those checks are tracked in
[Exh-3kvx](../.tickets/Exh-3kvx.md).
