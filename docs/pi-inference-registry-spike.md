# Artifact inference through Pi's registry

Research for [Exh-urrg](../.tickets/Exh-urrg.md), under
[Exh-zue3](../.tickets/Exh-zue3.md). Investigated 2026-11-05 on Linux ARM64; mockLLM acceptance completed 2026-11-19.

## Decision and acceptance status

**Recommend one dedicated, warm Pi Bolt worker, using a small inference-only
extension and Go supervision. Reuse `registry.complete()` and
`registry.classify()`; do not build the parallel Go provider clients yet.**
The pinned native binary can host this without adding Node, npm, a listener,
an agent conversation per inference call, or a new production service. A worker
is separate from the per-conversation agent manager and its admission limits.

**The architecture spike is complete.** The user clarified the acceptance
scope: “Why do we need openrouter benchmarks? We should be able to test with
mockLLM?” Deterministic mock-provider calls through the **real pinned native
Pi registry and adapters**, measured local overhead/memory, isolation/failure
checks, and a written choice suffice to choose this runtime. No live provider
benchmark or credential is required to close [Exh-urrg](../.tickets/Exh-urrg.md)
or begin the dependent implementation.

The prototype now uses the shared [mockLLM handler](../internal/mockllm/inference.go)
for both operations. Go calls the real worker over RPC; the extension resolves
real OpenRouter catalog models and calls `registry.complete()` / `classify()`.
Only the provider HTTP destination is redirected to loopback. This retains the
pinned transport, bool/noul conversion, parsing and catalog accounting in the
tested path; it does not mock the registry at its boundary. The
[fixture wrapper](../scripts/pi-inference-spike/fixture.go) only instruments
credentials, overlap and cancellation and adds the 1 ms benchmark delay.
Shared mockLLM supplies SSE completion, typed answers, usage, slow cancellation,
HTTP errors and malformed billed answers. Normal agent tool-building and
legacy guard scripts remain covered by regression tests.

No live OpenRouter inference was performed. Mock success proves neither live
API compatibility, model accuracy, real spend nor a provider-facing latency
SLO. Before enabling an actual provider integration/release, the
[complete kind](../.tickets/Exh-gd3g.md) and
[decide kind](../.tickets/Exh-x16u.md) retain opt-in/manual compatibility checks.
In particular, the pinned classifier calls `/api/v1/systemone`, while the
2026-11-05 OpenRouter catalog advertised `/api/alpha/decisions`. Unauthenticated
401 probes establish neither route's compatibility nor incompatibility. Do not
patch transport speculatively; if authorized release validation fails, evaluate
an explicitly tested correction or the narrow Go fallback below. Evidence:
[pinned transport][systemone], [provider catalog][or-classifiers], and
[captured catalog/probe results](../scripts/pi-inference-spike/results/openrouter-catalog-2026-11-05.json).

## Exact version evidence

The [Dockerfile](../Dockerfile) pins Bolt `1.0.8` and its release-archive hashes.
The ARM64 archive downloaded from the [official release][release] matched
`f530486723c7ab9004aca51f6f9494b95861fba7287c05ae8804647d63077de1`.
The actual executable reported:

```text
1.4.0 (Pi-Bolt 1.0.8, linux-arm64, JIT off)
```

Source checkouts were Bolt tag `bolt-v1.0.8`, commit
`49245b143555dd14cd00d1617f5329d2b50fe676`, and upstream Pi tag `v1.4.0`,
commit `4a080252f06caf63a525d7873c5d94d3c23887e0`. All implementation citations
below refer to **Bolt's pinned source**, not a Pi 0.99 API or a moving branch.

Bolt is not byte-for-byte upstream Pi 1.4.0. Local source comparisons found
identical `model-registry.ts`, auth resolution, and System One classifier
implementation; `model-runtime.ts`, RPC output handling, the chat adapter,
and generated catalog differ. Notably, Bolt's chat adapter includes
OpenRouter-reported-cost handling. The installed Node `pi` also reports 1.4.0,
but was **not** used for these measurements. Do not substitute it in the
commands and assume identical behavior. Sources: [registry][registry],
[runtime][runtime], [Bolt chat adapter][chat],
[upstream Pi chat adapter][upstream-chat], [cost normalization][or-cost].

## Smallest viable host and protocol

The stock RPC switch has no arbitrary host-to-extension method registration.
It has `prompt`, session-wide `abort`, state/model commands, and replies to
extension UI requests. Notifications are fire-and-forget extension-to-host
messages, not return values for a new RPC method. [RPC implementation][rpc]

The viable supported combination is:

1. Start **one dedicated** `pi --mode rpc` with the inference extension only.
2. Put an inference envelope inside the RPC `prompt.message`: ID, operation,
   resolved model, request credential, and context. This is private pipe
   traffic between trusted Go and its child, never a browser/agent prompt.
3. The extension's `input` handler starts an async job and immediately returns
   `{ action: "handled" }`. Handled input returns before model/auth checks and
   history insertion; no agent turn starts. The RPC response's
   `disposition: "handled"` only acknowledges dispatch, **not** inference success.
4. Send the completed result through `ctx.ui.notify`, using an
   `exhibit_inference:` JSON envelope correlated by the request ID.
5. A separate handled input cancels exactly that ID's `AbortController`.
   RPC `abort` is session-wide and does not manage these detached extension jobs.

This follows the pinned [RPC dispatcher][rpc],
[input-handler ordering][session], and [extension contract][extensions].
The existing [guardrail](../internal/agent/ext/guard.ts) already demonstrates
per-call `apiKey`/`signal` and registry lookup of classifier models. The new
[extension prototype](../scripts/pi-inference-spike/extension.ts) and
[Go supervisor](../scripts/pi-inference-spike/worker.go) demonstrate dispatch,
result correlation and cancellation together on the release executable.

All inputs are consumed, including invalid ones: a malformed envelope must
never fall through to an ordinary agent turn. The worker has no saved session,
tools, MCP, extension discovery, context files, prompt templates, user HOME or
provider environment. Startup is offline. A readiness ping exercises the
extension before accepting work. Tests verify zero conversation messages and
no request/key material written under its private HOME after mixed calls.
Credentials go through stdin only, never argv or per-worker environment.
Raw provider errors/stderr are not forwarded; trusted results still include
model/provider identifiers and must go through the core's browser redaction.

An even smaller host is a standalone JS process with
`createModels()`, `openrouterProvider()`, `getModel()` /
`getModelOfType()`, and `complete()` / `classify()`. This is the current API,
not the old global-registry contract; see the [pinned provider-factory docs][ai-doc].
It avoids the coding-agent session scaffolding. It would, however, add a JS
runtime/package installation or a separately compiled worker to our native
runtime image and require maintaining equivalent catalog/cost behavior across
Bolt and npm Pi. No standalone-shim performance claim was measured. Because
the existing pinned Bolt host works locally, that packaging change is not
justified by the low-millisecond local classifier difference alone.

## Concurrency, credentials and cancellation

`ModelRegistry` delegates each call to `ModelRuntime`. `prepareRequest()`
constructs request-specific options, resolves auth with the supplied key and
signal, and passes the resolved options to the provider. Explicit keys take
the request-override branch **before** reading stored credentials. They are
not installed into a process environment or mutated via a shared
`setRuntimeApiKey()`. Sources: [registry][registry], [prepareRequest][runtime],
[auth override][auth], [classifier HTTP headers/signals][classifier-shared].

Use a fresh `AbortController` and options object per call, and require an
explicit, nonempty resolved key at the Go/extension boundary. Missing keys
must fail before provider dispatch, not fall back to the platform credential.
Do not assign a shared conversation/session ID or enable immutable-message
request caching for unrelated viewers. The prototype supplies neither; Bolt's
[request cache][request-cache] only activates when both are explicitly set.
The offline catalog is shared; key-specific OpenRouter catalog personalization
is intentionally not run or cached for all viewers. Resolving a model from
the catalog does not guarantee the caller's key may use it; API rejection still
fails that call. [OpenRouter provider/personalization][or-provider]

Real-Bolt tests against shared mockLLM through the loopback fixture establish:

- Twelve concurrent mixed chat/classifier calls reached twelve overlapping
  HTTP handlers. Alternating fake platform/user keys were checked against
  independent tenant labels in the payload: **zero mismatches**. Chat results
  also carried the correct tenant label, checking result correlation.
- Canceling a chat request and, separately, a classifier request canceled its
  actual HTTP transport. The concurrent sibling completed and the same worker
  answered a later ping. This is not just cancellation of a Go waiter.
- Sixteen in-flight requests are admitted; the next receives `busy` without a
  provider call. Requests over 64 KiB are refused. These are prototype bounds,
  not proposed product rate limits.
- The pinned adapters return `stopReason: "error"` / `"aborted"` rather than
  relying on promise rejection. A Go caller must inspect that result, meter
  any usage, and then map the error. The benchmark refuses a non-`stop` result.

The tests use fake keys, not two live accounts. Source inspection plus header
assertions substantiate request isolation; they do not establish account
permissions or billing with real platform/BYOK credentials. JavaScript heap
memory is shared within a worker, not a per-viewer security sandbox. Never
load third-party/user extensions there. [Tests](../scripts/pi-inference-spike/worker_test.go)

## Latency and memory measurements

**Shared mockLLM over local HTTP only; no provider latency or HTTPS/TLS is included.**
One ARM64 run on 2026-11-19 on this shared server, Go 1.25.7, checksum-verified Bolt 1.0.8,
JIT off. Each path/kind had one excluded warmup and **20 measured samples**.
Calls used the same small payload and model ID, alternating direct/worker
order, reusable connections, eight maximum chat output tokens, and zero
provider retries. Chat uses SSE internally even though `complete()` returns
only the finished message; the direct Go control likewise consumes SSE and
usage to completion. Classification asks one bool, one choice and one score.
The fixture injects a 1 ms delay and deterministic answers/usage; this is not
an accuracy test. Both classifier paths deliberately use the pinned
`/api/v1/systemone` URL shape, so a direct control cannot hide a worker endpoint
compatibility problem. [Harness](../scripts/pi-inference-spike/main.go),
[instrumented mockLLM](../scripts/pi-inference-spike/fixture.go),
[raw samples/results](../scripts/pi-inference-spike/results/mockllm-bolt-1.0.8-arm64.json)

- Chat: warm worker median **3.816 ms**, range 3.268–11.556 ms; direct Go HTTP
  median **1.638 ms**, range 1.493–3.825 ms.
- Classifier: warm worker median **2.533 ms**, range 2.369–2.800 ms; direct Go
  HTTP median **1.583 ms**, range 1.463–1.857 ms.
- Difference of medians: **2.178 ms** chat, **0.950 ms** classifier. These are
  whole-adapter/IPC differences, not pure serialization measurements. Go vs
  Bun HTTP stacks and fixture timer scheduling also contribute.
- Worker launch to extension-ready ping: **135.777 ms** (one observation).
  Never launch one per artifact request.
- `/proc/<pid>/status` RSS: worker ready **84,888 KiB**, warm **94,804 KiB**
  (about 92.6 MiB). Go driver/fixture process ready **7,964 KiB**, warm
  **12,036 KiB**. Direct Go adds no child process. The Go values include both
  paths, the fixture and measurement scaffolding, so they are not an isolated
  production direct-client memory benchmark. RSS includes shared mappings;
  PSS, peak RSS, long-run growth and loaded production behavior were not measured.

The local classifier overhead supports a warm-worker design, with a real
roughly 93 MiB steady child-process cost on this host. It does **not** prove a
provider-facing latency SLO. There is one process/run, small inputs, no TLS,
no production load distribution, and no meaningful tail percentile estimate.
The earlier isolated-fixture baseline is retained as
[historical local evidence](../scripts/pi-inference-spike/results/local-bolt-1.0.8-arm64.json).
Neither run is a live-provider benchmark; local overhead suffices for this
architecture choice. Concurrent compilation on a shared server may also affect
individual samples. The harness reports the upper middle observation for an
even sample count, rather than interpolating between the two middle values.

## Usage, catalog prices and answer contract

The same normalized `Usage` shape is available for both operations when the
provider supplies usage: input/output/cache counts and a USD `cost` breakdown.
Classifier usage is optional, not a guaranteed zero-cost success. The classifier
adapter prices `input_tokens` / `output_tokens` with `calculateCost()`; chat
also calculates from the catalog, and Bolt can replace its total with validated
OpenRouter accounting (`totalFromProvider: true`). OpenRouter's upstream-key
BYOK accounting is different from Exhibit choosing a viewer's OpenRouter key:
the former can include an upstream provider charge in addition to the OpenRouter
fee. Sources: [classifier usage parser][classifier-shared],
[cost calculation][models], [Bolt OpenRouter cost handling][or-cost],
[OpenRouter usage-accounting docs][or-accounting].

For the pinned catalog, `openai/gpt-4.1-nano` costs $0.10/$0.40 per million
input/output tokens; Jev 1.14 costs $0.042 per million input tokens and zero
output-token cost. The fake classifier's 100 input tokens therefore produce
**$0.0000042**; the fake chat produces 100 input + 1 output and a reported
**$0.0000104**. These are fixture accounting checks, **not real spend**.
The retained raw results mark chat `totalFromProvider: true` and classifier
`false`. [Pinned catalog][catalog], [raw local results](../scripts/pi-inference-spike/results/mockllm-bolt-1.0.8-arm64.json)

The public OpenRouter catalogs were read without a key. The 2026-11-05 snapshots identify
chat snapshot `openai/gpt-4.1-nano-2025-04-14` and classifier snapshot
`typesafe/jev-1.14-20261014`, with the same base input/output prices. These
are catalog snapshot identifiers, **not observed models served by live calls**.
Catalog cache-read pricing can drift (snapshot chat API $0.03/M versus pinned
$0.025/M). Keep classifier cost explicitly estimated, do not claim it is an
invoice, and test/catalog-review it when upgrading the pin.
[Current chat catalog][or-models], [current classifier catalog][or-classifiers]

Usage is attached **before** classifier answers are parsed; malformed billed
answers retain usage and return `stopReason: "error"`. Tests cover this.
Usage missing from a canceled response or lost on worker death is unknown
spend, not evidence of zero charge. Provider cancellation cannot guarantee
refunds or recover accounting already lost with the process.
[System One result parsing][systemone-shared]

The normalized classifier contract maps `bool` to wire-level `noul` and back
to `{ type: "bool", probability }`; choice includes probabilities/confidence,
score includes score/confidence. Pi's normalized score answer omits any extra
provider fields such as score probabilities. The artifact API should promise
Pi's normalized shape, not an unchanged provider response. Pi checks finite
numbers and required fields but does not supply all the product's input bounds,
probability-range/membership checks, or JSON Schema validation/repair for chat;
those remain kind-adapter responsibilities. The prototype demonstrates the
three question types, not a full validation/structured-output service.
[Pinned conversion/parser][systemone-shared], [TypeSafe question semantics][typesafe]

## Failure model and production boundary

The Go prototype requires a wall-clock deadline, multiplexes requests with
bounded input/output frames, and fails pending calls on EOF/crash. On request
cancellation it sends a per-ID cancel envelope and allows **250 ms** for a
terminal result (retaining any usage). If the worker cannot acknowledge, Go
kills that generation and fails all its calls. This bounds a JS infinite loop
or ignored abort; an otherwise idle wedged worker is detected on its next call.
The stdout reader drains/reaps the child and removes its scratch HOME.
[Supervisor](../scripts/pi-inference-spike/worker.go)

With real Bolt and local in-flight classifier HTTP calls, fault injection
measured **8.621 ms** to fail pending calls after exit(42), and **281.784 ms**
after an intentional synchronous infinite loop (30 ms deadline + grace).
Explicit fresh-worker readiness took **176.040 ms** and **191.204 ms** respectively;
the subsequent classifier call succeeded. One sample each, not restart SLOs;
fault commands only exist when the test fixture is configured.
[Fault-injection tests](../scripts/pi-inference-spike/worker_test.go),
[captured test output](../scripts/pi-inference-spike/results/mockllm-runtime-tests-bolt-1.0.8-arm64.txt).

Production work belongs to [Exh-hwjg](../.tickets/Exh-hwjg.md): an independent
supervisor owned by inference, eager readiness while inference is enabled,
bounded admission, a restart backoff/circuit breaker, and clean shutdown.
The prototype proves an explicit replacement generation works; it does not
implement a restart daemon/backoff. Do not call `agent.Manager.Create()` or
`reserveProcess()`; those count conversational processes and carry agent-scoped
credentials. [Existing agent process limits](../internal/agent/agent.go)

Never replay a failed/unknown-outcome inference call on restart: it may already
have reached the provider and been billed. The next admitted call may use the
new generation. A timeout can kill siblings if the worker is truly wedged;
ordinary cooperative cancellation does not. Meter every usage-bearing result
before validating/redacting it; map unknown worker/provider errors to the
core's stable error vocabulary. No production endpoints, bridges, DB migrations,
guardrail changes or agent process limits are implemented by this spike.

## Go fallback and provider release validation

If later provider validation shows pinned-registry calls cannot work reliably, or the measured
production memory/latency budget rules out the worker, implement the two narrow
Go adapters behind the existing inference core. The direct control in this
prototype is deliberately OpenRouter-only, not the beginning of a generic SDK.

Chat needs provider-native schema options, streaming-to-final parsing,
token/cache/usage normalization and retry/cancel behavior even for a nonstreaming
artifact API. Decisions need a separate endpoint, bool/noul conversion, typed
answer/error validation, optional usage and catalog estimation. Preserve the
decision model contract; never substitute a chat model's invented probabilities.
Direct Go removes the child process and pin/IPC failure surface, but loses the
registry's existing multi-provider adapters, model-kind catalog and prices unless
we maintain equivalents. It may gain earlier access to a newly advertised
provider endpoint. A direct request's reported cost does not replace a catalog
for providers/operations without reported prices. [Pinned provider registration][or-provider],
[chat implementation][chat], [classifier implementation][systemone-shared]

The runtime choice is ready for implementation. Provider compatibility is a
separate release check in the two kind tickets, not an unresolved dependency
of this spike. It may be run manually or with explicit opt-in when authorized
configuration is available. Record served model IDs if exposed, response
usage/cost provenance and confirmation of the pinned classifier URL. A live
benchmark is optional; it is not necessary to repeat the local runtime-overhead
comparison. Keep a failing integration disabled until corrected and verified.
The ZIP export ticket requires a compatible local worker in addition to the Go
relay and local keys.

## Reproduction and verification

This round: checksum/version verification; shared mockLLM and real-Bolt
prototype tests under Go's race detector; selected existing agent/guardrail
pipeline regressions using the same native binary; `go vet ./internal/mockllm
./scripts/pi-inference-spike`; local mock benchmark; Markdown target/result-JSON
validation; `git diff --check`; and ticket dependency-cycle check. Captured
[shared mock/native tests](../scripts/pi-inference-spike/results/mockllm-runtime-tests-bolt-1.0.8-arm64.txt)
and [agent regressions](../scripts/pi-inference-spike/results/agent-mock-regression-tests-bolt-1.0.8-arm64.txt).
The prior round also completed an asset build and repository-wide compile-only
checks (`go test -run '^$' ./...`).
The prior broader `go test ./...` run terminated with exit 143 before completion;
`internal/agent` had passed (49.895 s), and no failing assertion was emitted
before termination. This is **not** a full-suite pass. The latest prototype
race run includes malformed/scalar-envelope handling and verifies such input
cannot fall through to an ordinary agent prompt.

Run from the repository root. The checks below use the same ARM64 archive and
hash as the Dockerfile. For amd64, use its `linux-x64-baseline` archive and
Dockerfile hash instead. Keep adjacent archive files. Downloads/scratch data
stay outside the checkout.

```sh
mkdir -p /tmp/Exh-urrg-bolt
curl -fsSL --retry 2 \
  https://github.com/opensec-git/Pi-Bolt/releases/download/bolt-v1.0.8/pi-bolt-linux-arm64.tar.xz \
  -o /tmp/Exh-urrg-bolt.tar.xz
echo 'f530486723c7ab9004aca51f6f9494b95861fba7287c05ae8804647d63077de1  /tmp/Exh-urrg-bolt.tar.xz' | sha256sum --check --strict
tar -xJf /tmp/Exh-urrg-bolt.tar.xz -C /tmp/Exh-urrg-bolt --strip-components=1
/tmp/Exh-urrg-bolt/pi --version

PI_SPIKE_BIN=/tmp/Exh-urrg-bolt/pi go test -race -count=1 -v ./internal/mockllm ./scripts/pi-inference-spike
go vet ./internal/mockllm ./scripts/pi-inference-spike
PATH=/tmp/Exh-urrg-bolt:$PATH go test -count=1 -v ./internal/api \
  -run '^(TestAgentSessionReadsTheArtifactItself|TestPlatformModeRunsASessionWithNoStoredKey|TestGuardrailBlocksAPromptBeforeTheAgentRuns|TestGuardrailFailsClosed|TestGuardrailAllowedPromptShowsNoRefusal)$'
go run ./scripts/pi-inference-spike -pi /tmp/Exh-urrg-bolt/pi -n 20 \
  > /tmp/Exh-urrg-local.json

# Optional provider release check, NOT required for the spike.
# OPENROUTER_API_KEY must already be supplied by authorized configuration.
# Never put a real key in argv, source files, command output or git.
# Each kind/path makes one warmup + 5 measured calls: 24 total provider calls.
go run ./scripts/pi-inference-spike -pi /tmp/Exh-urrg-bolt/pi -live -n 5 \
  > /tmp/Exh-urrg-live.json
```

The tiny configured models/output bound are intended to keep a live run cheap;
no live spend was incurred here. Benchmark output is written only after all
paths succeed; a failure returns a generic stage/status error without dumping
upstream bodies or credentials. With no `PI_SPIKE_BIN`, integration tests skip
explicitly; that is not validation of the pinned runtime. By default,
the CLI uses shared mockLLM even when a live key exists. Only `-live` permits
provider requests, and it refuses to run without explicit live configuration.
The benchmark also refuses an executable whose version string is not the
pinned Bolt/Pi pair (archive checksum verification remains the operator's step).

Source checks can be reproduced independently:

```sh
git clone --depth 1 --branch bolt-v1.0.8 https://github.com/opensec-git/Pi-Bolt.git /tmp/Exh-urrg-pi-bolt-1.0.8
git clone --depth 1 --branch v1.4.0 https://github.com/earendil-works/pi.git /tmp/Exh-urrg-pi-1.4.0
git -C /tmp/Exh-urrg-pi-bolt-1.0.8 rev-parse HEAD
git -C /tmp/Exh-urrg-pi-1.4.0 rev-parse HEAD
curl -fsSL 'https://openrouter.ai/api/v1/models?output_modalities=decisions' \
  -o /tmp/Exh-urrg-current-classifiers.json
```

[release]: https://github.com/opensec-git/Pi-Bolt/releases/tag/bolt-v1.0.8
[registry]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/coding-agent/src/core/model-registry.ts
[runtime]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/coding-agent/src/core/model-runtime.ts
[rpc]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/coding-agent/src/modes/rpc/rpc-mode.ts
[session]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/coding-agent/src/core/agent-session.ts
[extensions]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/coding-agent/docs/extensions.md
[ai-doc]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/README.md#provider-factories
[auth]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/auth/resolve.ts
[systemone]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/api/typesafe-system-one.ts
[systemone-shared]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/api/system-one-shared.ts
[classifier-shared]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/api/classifier-shared.ts
[models]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/models.ts
[or-provider]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/providers/openrouter.ts
[chat]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/api/openai-completions.ts
[upstream-chat]: https://github.com/earendil-works/pi/blob/v1.4.0/packages/ai/src/api/openai-completions.ts
[or-cost]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/utils/openrouter-cost.ts
[request-cache]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/utils/request-cache.ts
[catalog]: https://github.com/opensec-git/Pi-Bolt/blob/bolt-v1.0.8/packages/ai/src/providers/data/openrouter.json
[or-models]: https://openrouter.ai/api/v1/models
[or-classifiers]: https://openrouter.ai/api/v1/models?output_modalities=decisions
[or-accounting]: https://github.com/OpenRouterTeam/docs/blob/main/cookbook/administration/usage-accounting.mdx
[typesafe]: https://docs.typesafe.ai/introduction
