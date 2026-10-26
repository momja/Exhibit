# Standalone workers on Fly Sprites

Implementation scope: [Exh-fuxp](../.tickets/Exh-fuxp.md), under
[Exh-i0ll](../.tickets/Exh-i0ll.md). `internal/sprites` is an infrastructure
backend, independent of `internal/agent`, HTTP handlers, and Store. The core
Manager still owns coordination. `cmd/server/agent_workers.go` connects it to production mode selection, durable
admission, encrypted request journaling, lease renewal and scheduled cleanup.

## Platform contracts checked 2026-10-20

The implementation uses the current official
[OpenAPI 0.1.12](https://docs.fly.io/sprites/api/openapi.json),
[API overview](https://docs.fly.io/sprites/api), and
[Go SDK](https://github.com/superfly/sprites-go). It makes authenticated REST
requests to `https://api.sprites.dev`; it does not assume Docker image deployment.

- [Lifecycle](https://docs.fly.io/sprites/concepts/lifecycle): persistent disk,
  disposable process state, warm/cold wake, reconnect after suspension. Treat
  snapshots acknowledged by the core as authoritative.
- [Services](https://docs.fly.io/sprites/concepts/services): register the worker
  once; the platform starts it on cold boot and restarts crashes. A quiet service
  does not itself require an active hold.
- [Tasks](https://docs.fly.io/sprites/keeping-sprites-running): active holds use
  HTTP over `/.sprite/api.sock`, host `sprite`, with expiry up to one hour.
  This API is **inside the Sprite**, not a guessed remote management endpoint.
- [TCP proxy](https://docs.fly.io/sprites/api/websockets/tcp-proxy): authenticated
  WebSocket at `/v1/sprites/{name}/proxy`, JSON `{host,port}` initialization,
  `status: connected` acknowledgement, then binary TCP data. This keeps the
  management credential out of worker HTTP requests as well as its environment.
- [Org limits](https://fly.io/sprites/): concurrency and creation limits depend
  on the org/plan. Read returned running/warm limits and observe real refusals;
  do not treat the local configured ceiling as purchased capacity.

The Go SDK and OpenAPI currently disagree on parts of the schema. Filesystem
writes send both OpenAPI's `mkdir=true` and the SDK's `mkdirParents=true`.
Inventory accepts top-level OpenAPI limits and nested SDK `org` limits. Service
creation accepts NDJSON events (documented service behavior and SDK) and a JSON
service object (OpenAPI). These compatibility paths have fake-server coverage;
none is claimed to have been exercised against a live org in this implementation.

## Coordinator integration contract

Construct `sprites.New(sprites.Config{...})`. Required values are `Token`, an
exclusive deployment `Prefix`, and `Release{Version, URL, SHA256}`. The URL is a
complete pinned runtime **archive**, even if the outer configuration calls it
`AGENT_WORKER_RELEASE_URL`. Require an immutable release version and a separate
SHA256 setting; a bare Go binary does not satisfy the archive contract.

Suggested configuration mapping, owned by the core integration:

| Core setting | Package field |
| --- | --- |
| `SPRITE_TOKEN` | `Config.Token`, coordinator only |
| `AGENT_SPRITE_PREFIX` | `Config.Prefix` |
| `AGENT_WORKER_RELEASE_URL` | `Config.Release.URL` (tar.gz runtime bundle) |
| `AGENT_WORKER_RELEASE_VERSION` | `Config.Release.Version` |
| `AGENT_WORKER_RELEASE_SHA256` | `Config.Release.SHA256` |
| `AGENT_MAX_WORKERS` | `Config.MaxWorkers` and central active admission |
| `AGENT_WORKER_STARTUP_TIMEOUT` | `Config.StartupTimeout` |
| `AGENT_WORKER_IDLE_RETENTION` | `Config.IdleRetention` |

Other constructor fields control the Service port, static supervisor environment,
extra daemon arguments, startup concurrency, creation spacing, request/cleanup
deadlines, orphan grace, retries, and optional readiness adaptation. The backend
does not read environment variables. Root mode selection (`local`, `remote`,
`sprites`, `off`) and static remote-worker settings live outside this package.

`Identity{OwnerID, SessionID, ExecutionID}` is an immutable **assignment**.
OwnerID is a string here; adapt an integer owner ID with `strconv.FormatInt`.
ExecutionID is the central fencing/assignment generation, not the prompt ID.
Use a fresh ExecutionID after central lease loss or reassignment; never reuse a
retired generation. If only an idle Sprite disappears while its central lease
remains valid, the coordinator may provision the same immutable assignment on
the next explicit input and restore its acknowledged snapshot. It does not
replay an uncertain active turn.
The deterministic resource name is `<prefix>-<128-bit identity hash>`. Raw owner
and conversation IDs do not appear in the name, and a different owner produces a
different Sprite. Prefixes must be exclusive to one coordinator deployment.

After central admission and reservation, generate `NewEndpointToken()` and save
`Request{Identity, EndpointToken}` durably **before** calling `Provision`. Retries
must present that same request. The backend deliberately does not generate a new
credential when an uncertain create may already have succeeded.

```go
backend, err := sprites.New(config)
// Handle configuration errors before accepting remote work.
worker, err := backend.Provision(ctx, persistedRequest)
// Adapt only after handling provisioning errors:
//   agentexec.Endpoint.URL       <- worker.URL
//   agentexec.Endpoint.Token     <- worker.EndpointToken
//   agentexec.Endpoint.Transport <- worker.Transport
// The same transport is available on worker.Client.Transport.
```

`Endpoint` returns `URL`, `EndpointToken`, `Transport`, `Client`, immutable identity,
name, and `Destroy(ctx)`. Its client and transport are coordinator-only objects.
Always pass the returned transport to the worker HTTP adapter: URL and application
token alone cannot reach this private Sprite. Do not replace it with the default
transport. No localhost listener or control-connection pool is opened. The
transport pins the endpoint origin, refuses redirects, adds application bearer
auth and `Exhibit-Agent-Version: 1`, and opens one bounded authenticated proxy
connection per request. The outer management token is used only on the Sprites
API handshake; the tunneled HTTP request carries only the application token.

`Backend` exposes:

- `Provision(ctx, Request)`: create if missing, recover a partial bootstrap,
  register Service, wait for readiness. An identical existing assignment is
  validated without rewriting its credential or restarting its Service.
- `Wake(ctx, Request)`: explicitly reconnect and validate the stored manifest,
  then check readiness. Never creates a missing Sprite or replays a prompt.
- `Query(ctx, Identity)`: management metadata only; no filesystem read, tunnel,
  worker health request, or background wake. `cold`/`warm` describes platform
  state, not application health.
- `Destroy(ctx, Identity)` / `Endpoint.Destroy`: idempotent infrastructure delete.
  The **caller first stops/fences execution, flushes acknowledged state, and
  revokes grants**. Do not wire idle settlement straight to destruction if
  retention is desired.
- `Reconcile(ctx, []Assignment, BeforeDestroy)`: delete expired idle assignments
  and old untracked orphans, while preserving active, recently idle, recently
  created, and locally busy resources. A mandatory hook flushes/revokes before
  deletion; hook failure retains the resource. Returns deleted/failed names and
  observed org counts/limits. Serialize central snapshot capture/reconciliation
  with assignment admission so an outdated snapshot cannot delete new work.

Only the exact generated prefix/name pattern is eligible for cleanup. A similarly
prefixed manually created Sprite is left alone. Account deletion should revoke
that owner's executions first, invoke `Destroy` for known assignments, and leave
failed deletions to a later authoritative reconciliation sweep. Startup failures
use a separate cleanup deadline after the startup context expires. Cleanup
failure is returned; an unknown create outcome that cannot be queried remains a
possible orphan for reconciliation. Never silently treat it as a completed run.

`ErrCapacity` is actionable admission refusal; release capacity or retry later.
`ErrNotFound` requires deliberate snapshot restoration: keep an existing valid
idle assignment, or acquire a fresh central assignment after lease loss.
`ErrConflict` requires investigation/new assignment, not automatic token rotation
or prompt replay. Upstream bodies, service output, and hook errors are excluded
from returned errors because they may contain secrets. Do not log Requests,
Config fields, HTTP headers, token files, or conversation payloads yourself.

## Release/bootstrap

Prepare a complete trusted Linux runtime tree for the Sprite architecture:

```text
runtime/
  bin/exhibit-agent
  bin/node
  bin/pi
  lib/...             # everything bin/pi needs, including node_modules
```

The worker embeds or includes its required extensions. Relative executable
symlinks must resolve inside this tree. Pin Node **22.22.0** and Pi **1.3.0**,
matching the current standalone daemon contract. `Release.NodeVersion` and
`PiVersion` can explicitly change those exact pins alongside a compatible daemon
release; do not change them independently during a rolling assignment.

Build the worker from the intended commit for the target Linux architecture
with its dependencies already installed in the prepared tree. Package it with:

```sh
scripts/agent-workers-sprites/package-release.sh COMMIT_VERSION runtime /tmp/exhibit-agent-COMMIT_VERSION.tar.gz
```

The script performs no networking/package install. It packages the whole tree,
adds `EXHIBIT_RELEASE`, normalizes tar metadata, writes a `.sha256` sidecar, and
refuses an existing destination. Publish the archive to a trusted immutable HTTPS
release location; configure its version, URL, and verified sidecar digest in the
coordinator. Archive hosting is an operator release step. No release has been
published as part of this implementation.

Provisioning creates a private URL (`url_settings.auth = sprite`), writes private
bootstrap/token/manifest files via the filesystem REST API, and registers
Service `exhibit-worker` with `cmd: /bin/sh`, an uploaded script path, and
`http_port: 8080` by default. The script:

1. Downloads only over HTTPS with bounded curl connect/total deadlines.
2. Checks SHA256 **before extraction**, then checks `EXHIBIT_RELEASE` and the
   included Node/Pi executable versions under a clean environment.
3. Atomically moves the verified runtime into an assignment-local release path.
   Later cold bootstraps reuse that exact installed release.
4. Starts the daemon with a clean environment and explicit CLI arguments:
   `--addr 127.0.0.1:PORT --work-root .../scratch --pi .../bin/pi
   --execution-id ID --session-id ID --token-file .../endpoint-token`.

Scratch and bootstrap directories are private. The endpoint token file is 0600;
bootstrap is 0700. The manifest records assignment identity, bootstrap hash and a
token hash, never execution grants/provider credentials. A changed release,
launch configuration, owner or endpoint token cannot take over an existing
manifest. A compatible daemon's scratch-root lock clears stale execution
scratch on process restart; Sprite deletion removes its assignment disk entirely.

Default readiness is authenticated `GET /v1/ready`, protocol header `1`, checking
protocol version `1`, pinned Pi version, execution ID and session ID. Owner is
validated through the immutable manifest; it is not worker tool authority.
`CheckReady` may adapt another daemon protocol but must check assignment and
protocol/pins, remain read-only, respect its context, and return `ErrConflict`
for terminal identity mismatches. Startup/wake retries stop at the startup
deadline. There is no periodic idle readiness poll.

## Active holds and the daemon integration requirement

`sprites.NewHolds()` is a small **in-Sprite** helper. `Acquire`/`Renew` upsert
`PUT http://sprite/v1/tasks/NAME` over `/.sprite/api.sock` with a whole-second
expiry of 1 second–1 hour; `Release` deletes and tolerates an already-missing
task. It needs no credential and starts no goroutine or automatic renewer.

The standalone runtime must call it as part of its execution lifecycle; adding
this infrastructure package alone does not activate holds:

1. Before acknowledging active work, acquire a task named for the execution.
   If acquisition fails, fail/abort rather than continue unprotected work.
2. During that execution only, renew a short hold, e.g. five minutes at a shorter
   interval. Cap each renewal at the remaining central execution/turn deadline.
   Stop renewal on settlement, abort, lease expiry, or coordinator loss. A
   timer tied only to daemon lifetime would keep an idle worker active forever.
3. After the core acknowledges the durable snapshot and settlement, release
   the task using a separate short cleanup context. Failure must be surfaced;
   the finite expiry still bounds a crashed process's hold.
4. Close worker event response bodies, active streams, and idle HTTP transports.
   `Endpoint.CloseIdleConnections` closes pooled idle state but cannot cancel an
   active SSE response; the adapter must close/cancel it. Keep browser SSE at
   the core rather than forwarding a permanently open idle worker connection.

Equivalent local commands for diagnosis (run only inside the Sprite):

```sh
curl --unix-socket /.sprite/api.sock -H 'Content-Type: application/json' -X PUT http://sprite/v1/tasks/EXECUTION -d '{"expire":300}'
curl --unix-socket /.sprite/api.sock -X DELETE http://sprite/v1/tasks/EXECUTION
```

Provider keys and tool/snapshot/usage authority are per-execution input to the
authenticated daemon API. The endpoint token authenticates only coordinator to
supervisor. Keep it out of Pi's minimal environment, along with the Sprites token.
The coordinator retains all BYOK/platform selection, admission, policy, snapshot
persistence, and fencing. The worker reaches the configured Exhibit API and model
provider over authenticated HTTPS; it mounts no core database/blob/secret volume.
An operator may separately apply an outbound network policy for the release host,
Exhibit and selected providers. No egress policy is silently guessed here.

## Bounds, rollout, rollback and evidence

Defaults: at most 20 retained Sprites under the prefix, two concurrent startups,
two-minute startup deadline, 30-second REST/proxy handshake deadlines, 15-second
cleanup deadline, 15-minute idle retention, five-minute orphan grace, and six
seconds between create attempts. Management retries are capped at four attempts
with exponential full-jitter backoff (250 ms initial, five-second local cap).
`Retry-After` is a lower bound; a value beyond the remaining deadline causes a
prompt refusal. Readiness retries are limited by the startup deadline, not the
management retry count. All operations are additionally bounded by caller context.

The package counts running Sprites across paginated **whole-org** inventory before
creation and respects reported running limits, so another deployment's workers
count too. Prefix retention counts include idle disks. These checks are local
admission guards and observations, not a distributed reservation against other
org clients. API quota refusals are still authoritative; the core enforces its
own durable global/owner active-run limits before any Sprite is started. Use one
coordinator per prefix. Configure creation spacing for the actual org plan;
other tools/deployments can consume its creation allowance too.

Roll out with a new immutable release, small explicit limits, and a deployment
prefix whose reconciliation ownership is unambiguous. Drain/fence old assignments
before changing their configured release/launch fingerprint. Reconcile the old
inventory using its original backend configuration and central assignment map;
restore new explicit work from acknowledged snapshots with fresh execution IDs.
For rollback, stop new Sprites admission, settle or interrupt affected runs
explicitly, revoke their grants, clean their Sprites, then select the previous
archive+digest or the supported local/static remote mode. An unfinished prompt is
never automatically replayed. The SQLite-backed core remains a single writer;
this backend scales execution, not core storage.

The integration should record startup/wake latency, active duration, idle age,
worker and org counts/limits, quota refusal/retry counts, and cleanup failures.
Use resource/assignment identifiers and aggregate durations; avoid tokens,
provider keys, raw service logs and conversation bodies. Derive billing and
storage evidence from the real org dashboard/invoice and usage, not estimates.
There is no always-running floor in this package, and there is also no assertion
that retained Sprite disks are free.

Observed validation in this checkout:

- Fake `httptest` REST + WebSocket servers: private create, authenticated writes,
  service startup/failure, readiness, warm/cold metadata, deletion, no management
  token in worker traffic, no idle keep-alives/pool, retries, quotas, auth failure,
  pagination, org/prefix/startup ceilings, deadline/cleanup, idempotent and
  uncertain create, mismatched identity/credential, orphan/retention hooks.
- Actual local Unix-socket HTTP server: bounded Tasks acquire/renew/release,
  no bearer credential, cancellation, idempotent missing release, validation.
- Executed bootstrap with fixture archives: digest/release/runtime-pin rejection,
  clean supervisor environment, daemon CLI mapping and reuse after cold bootstrap.
  Reproducible release packaging and overwrite/incomplete-runtime refusal.
- `go test -race ./internal/sprites` and `go vet ./internal/sprites` pass.

No `SPRITE_TOKEN`, `SPRITES_TOKEN`, `FLY_API_TOKEN`, or `FLY_ACCESS_TOKEN` is present
in the implementation environment. No paid Sprite was launched. Live installation of the verified runtime
archive, service bootstrap, two concurrent Sprites, forced platform loss,
actual org quota/rate behavior, warm/cold timings, idle compute suspension,
retained-storage usage/billing, and a production rollback remain unvalidated.
Core integration now wires holds, shared durable authority, encrypted request
journaling, snapshots, per-Sprite transports, retention scheduling and mode
selection. Local validation additionally drives real pinned Pi through standalone
HTTP services with a fake Sprite fleet: simultaneous distinct-owner turns, central
admission, source/state changes, cold service replacement, resume and priced-usage
retry continuity, guardrail and aggregate instance-budget refusal, idle renewal
without worker HTTP, and account cleanup. Handler tests cover login/body-limit
separation, atomic concurrent create, stale denial and exact source/widget version
association. Blocking guards and management cleanup cannot stall other workers.
These checks do not establish the live platform or billing evidence above.

Final assembled-stack verification (2026-10-20):

- `make assets` and `go test ./... -count=1 -timeout=15m` passed, including the
  complete API and SQLite suites.
- Vet passed for the server, worker, agent, authority, transport, API, Sprites,
  and Store packages. All 46 browser history/import tests passed after rebasing
  onto the shipped asset-import changes; the full Go suite also passed again.
- The native arm64 release archive passed
  `TestPackagedReleaseMatchesSpriteBootstrap` with bundled Node/Pi. The amd64
  archive passed layout/checksum/ELF checks; amd64 execution was not tested.
- `docker build --target core` passed. A local container using that image had
  no Node/Pi executables, rendered chat after login, and drove a separate
  packaged worker over HTTP to stream saves, persist History, resume the same
  conversation, and commit another source version. The container and fixture
  processes were stopped after verification.
- Targeted race tests covered concurrent workers, restart cleanup, erasure,
  central admission/fencing, and late-receipt instance hard-stop propagation.
- Self-hosting upgrade verification used the server from main with schema 034,
  then the default local image with worker migration 035 on the same data volume.
  Login, encrypted BYOK credentials, artifacts and History survived; the same
  conversation resumed and committed another source version using bundled Pi.
  No execution-mode override, worker service, Sprite credentials, platform key,
  or spend-cap configuration was supplied. Default Compose still starts only
  the app. Upgrade regressions also cover schema 033.

Review-agent review covered every implementation layer and each follow-up fix;
all findings were fixed and re-reviewed. These are local validation results,
not a live Sprite rollout.
