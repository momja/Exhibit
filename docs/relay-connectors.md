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

For managed OAuth, set `CONNECTORS_VENDOR_NAMESPACE` to a stable, instance-specific
name and configure `CONNECTORS_VENDOR_PROVIDERS` as a reviewed JSON object:

```json
{
  "slack": {
    "name": "Slack",
    "engine": "nango",
    "origins": ["https://slack.com"],
    "daily_limit": 100,
    "cache_minutes": 5
  },
  "linear": {
    "name": "Linear",
    "engine": "composio",
    "auth_config_id": "ac_operator_config",
    "toolkit": "linear",
    "origins": ["https://api.linear.app"],
    "allow_writes": false
  }
}
```

Use the provider configuration key registered in Nango; Composio additionally
requires its auth config ID and toolkit slug. `CONNECTORS_NANGO_SECRET_KEY` and
`CONNECTORS_COMPOSIO_API_KEY` configure their respective control-plane clients.
Their base origins default to `https://api.nango.dev` and
`https://backend.composio.dev`; `CONNECTORS_NANGO_BASE_URL` and
`CONNECTORS_COMPOSIO_BASE_URL` override them with HTTPS origins. Optional
self-hosted Nango uses the same public-address boundary, with no private-network
exception. `CONNECTORS_VENDOR_IDLE_DAYS` defaults to 30.

The legacy `CONNECTORS_NANGO_PROVIDERS`, `CONNECTORS_NANGO_NAMESPACE` and
`CONNECTORS_NANGO_IDLE_DAYS` remain accepted when their canonical counterpart is
absent. A legacy provider map selects Nango; a canonical map must explicitly
select an engine. Invalid engines, origins, credential positions or delivery
modes fail startup without printing configuration values. Native GitHub OAuth,
Nango providers and Composio providers can operate together.

The vendor interface covers start, completion, status, injection or forwarding,
and revocation. Exhibit retains grants, entitlement checks, the allowlist,
credential-origin binding, quota reservations, encrypted caching, stand-ins and logs.
Vendor errors become `needs_reconnect`, `rate_limited`, `refused` or
`upstream_error`, with safe messages shared by the relay and trusted UI.

### Delivery modes and migration

Nango uses **inject** mode: Exhibit retrieves managed credentials and authorizes
each checked hop through its own egress gate. Provider response bodies do not
pass through Nango. Composio uses **forward** mode: Exhibit checks the initial
exact origin, operator policy, public DNS answers, method, headers and body
before handing a sanitized request to Composio. The same deadline and decoded
response limits apply. Composio performs the provider dial and redirects, so
Exhibit cannot pin that dial, check its redirect hops or scrub reflected
credentials it never receives. The grant preview and saved connection identify
the engine, delivery mode and this limitation. There is no silent fallback from
inject to forward mode.

Connections persist their engine, external reference and reviewed provider
settings. Changing a provider's configured engine affects new connections and
new flows only. Existing connections keep their original engine and policy;
the UI recommends reconnection. Unusable credentials report `needs_reconnect`
and show a reconnect action. Reconnection starts a new consent flow under the
current configuration; revoke the old connection when ready. The original
engine handles revocation and idle pruning even after the provider is switched.
If an entitlement cap prevents keeping both, revoke the old connection first.
Keep an old engine's operator key configured until its connections are removed.
Migration 041 pins earlier managed connections to Nango and resolves their
legacy encrypted reference without transferring credentials between vendors.

Operators can inspect **Vendor usage** on the Connections page or
`GET /api/admin/connector-usage`. It reports the current UTC month's management,
injected-provider and forwarded-provider attempts per engine/provider, plus
currently stored connections. Counts persist across restarts and failed
attempts, exclude cache hits, and survive deletion as anonymous aggregates.
They are cost inputs, not an invoice or a dollar estimate. The existing
entitlement system limits native, OAuth and managed connections separately.

### Composio verification (2026-10-23)

The implementation uses current v3.1 hosted Connect Links and connected-account
polling, not the retired managed-OAuth initiation flow. Browser binding and the
returned account's owner, auth config and toolkit are checked before saving.
[Connect Link reference](https://docs.composio.dev/reference/api-reference/connected-accounts/postConnectedAccountsLink),
[connected-account reference](https://docs.composio.dev/reference/api-reference/connected-accounts/getConnectedAccountsByNanoid).

Current credential APIs redact both managed and custom-app secrets and do not
provide a supported token-export path; this requires forward mode.
[Token custody](https://docs.composio.dev/docs/security/token-custody).
Proxy Execute supports HTTP endpoints beyond predefined tools, but restricts
absolute targets to the connected account's scheme and registrable domain;
Exhibit applies a stricter exact-origin policy. The adapter uses documented
methods and header/query parameters, preserves upstream status and safe headers,
and reconstructs the parsed data response. Binary download URLs are refused;
no vendor-supplied download host is fetched. Encoded upload envelopes must also
fit the control-plane request cap, which can make their effective raw-body cap
smaller than the relay's one MiB.
[Proxy contract](https://docs.composio.dev/reference/api-reference/tools/postToolsExecuteProxy),
[proxy behavior](https://docs.composio.dev/docs/extending-sessions/proxy-execute).

Composio processes proxied request and response data. Its published standard
retention stores payload logs for up to one year. Zero Data Retention is a
separate project/plan control for covered calls, with metadata still retained;
managed OAuth apps require Enterprise for that coverage. Exhibit neither
configures nor verifies this vendor setting. Review it before granting access.
[Privacy policy](https://composio.dev/privacy),
[ZDR scope and exclusions](https://docs.composio.dev/docs/security/zero-data-retention).
Programmatic provider revocation is best effort; local/vendor deletion can stop
further use without proving that an upstream token was revoked. Unsupported
providers may require cleanup in the provider's own settings.
[Revocation reference](https://docs.composio.dev/reference/api-reference/connected-accounts/postConnectedAccountsByNanoidRevoke).

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
and saved-data provenance. Existing artifacts continue using their public
paths; no reconnect is required.

Grant the saved connection to the artifact, review its capabilities and credential
destinations, approve its API origin, then enable relay reads. API changes require
the separate write grant and a connector recipe that permits writes.

**Connection grants are additive.** The artifact's approved origins remain its
network boundary. A connection supplies credentials only to its matching API
origin; attaching it does not narrow unrelated traffic. An artifact may use two
connections at different origins alongside an approved public API, stylesheet,
font or CDN. Each matching API receives only its own credential; approved
standalone requests carry no connector credential. Unapproved origins still
require the ordinary network approval, and operator policy remains enforced.
The viewer and agent preview use the same origin-approval flow.

Credential custody protects the secret. Once an API response reaches artifact
code, that code can process it, save it and send it to other approved origins.
Exhibit cannot guarantee that returned data stays at the source API. Review the
artifact and its network permissions before giving it sensitive account access.

Removing a grant or revoking a connection stops future credential use without
changing other origin approvals or deleting saved responses. Disabling connectors
does not narrow approved standalone network access. Optional **Clear saved data**
removes stored state; it is never required to approve an origin or use another
connection. Legacy `permitted_egress`, `data_origins` and retained policy records
remain compatibility/provenance metadata only, with no network authority.

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
grant network permission and should never be persisted in source, state,
widgets or transcripts. Native attach-by-origin remains the simpler path.

## Request boundary

The frame uses a document-bound host bridge; the host pins the artifact and
viewer. Its app-owned authority is absent from artifact source and state.
Responses return as bytes to the opaque frame, where the shim constructs a
`Response`; host code never inserts provider bytes into its DOM. Current artifact
origin approvals, viewer relay consent and matching connection grants are checked
on requests. A connector never changes the artifact's other approved origins.
The same checks apply to direct relay API calls and scheduled fetches.

External links retain their ordinary artifact approval flow. Connector grants
add no response-data destination ceiling to links, scripts, styles, fonts, image
loads or other browser requests. Source and document authority checks continue
to protect the trusted host bridge and state synchronization.

Private connected documents open only inside Exhibit, with an opaque sandbox
and an exact parent frame policy. Their URLs and signed scopes remain bound to
the authorized document; artifact frames cannot use app-owned fresh-token routes
to escape that binding. Gallery widgets show saved data inside a script-free
app-owned wrapper pinned to the widget's render path. Explicit owner consent is
still required to share connected saved data, including data retained after
revocation. These privacy and sharing checks do not limit the artifact's approved
network origins. HTTPS previews require the browser's iframe Fetch Metadata
header. On plain HTTP development origins, where browsers omit it, the referrer
must instead name the exact trusted app origin. The trusted widget wrapper sends
only that origin; render documents still send no referrer.

`internal/egress` enforces all of the following for native and inject-mode
connections, ordinary relay requests, acquisition flows and refreshes. Forward
mode shares the initial request and response boundary described above, with
the disclosed exception for the vendor’s provider dial and redirects:

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

Relay token buckets allow bursts of 120 requests per owner, 60 per artifact and
300 per target origin, refilling at those rates per minute. A local throttle
returns `429`, `code: relay_rate_limited` and a `Retry-After` delay. The trusted
browser bridge waits and retries GET/HEAD requests for up to one minute, within
the frame's existing request deadline. Each attempt checks current consent and
policy. Abort and navigation cancel queued retries; writes, daily quota failures,
permission failures and provider responses are never automatically replayed.
Large read-only imports therefore slow down while retaining the relay limits.
Provider 429
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
entitlements, origin approval, relay grants and matching connector authorization at execution.
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
rotate the render authority before new data can be returned. The manifest’s
approved origins determine the frame CSP; local connections add credentials
only at matching origins. Multiple connections and approved standalone requests
work together after disconnects, credential replacement and runner restarts.
Legacy policy sidecars do not narrow network access. Response bytes never enter
the host DOM. The listener defaults to loopback. An explicitly exposed listener can configure
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
Shared native/Nango/Composio fixtures cover grants, credential custody, allowlists,
quota/cache and feature flags. Managed fixtures also cover bound consent,
engine changes with existing connections and pending flows, normalized errors,
reconnect state, revocation, response caps and persistent cost counters. The
full normal Go suite passes. A broader store race run reached the default
ten-minute timeout in the large account-deletion fixture; focused affected
store races pass.

Additive-permission regressions exercise two distinct connections alongside a
standalone API/CDN, both managed engines together, origin approval while
connections are attached, schedules, legacy saved policies and independent
response reuse. Go and Node runners use the same parity fixture. Chromium loads
an approved stylesheet in both the viewer and agent preview without an access
prompt. The running ticket visualizer also loads its previously approved CDN
icons while GitHub relay requests return successfully, without clearing state
or changing the saved approvals. Updated interface evidence is in
[screenshots/Exh-s47o](screenshots/Exh-s47o).

Live generic Basic account access, registered GitHub OAuth, Nango and Composio
staging flows/costs and one live model generation require external credentials; those checks are tracked in
[Exh-3kvx](../.tickets/Exh-3kvx.md).
