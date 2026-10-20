# Standalone execution runtime

`go build -o bin/exhibit-agent ./cmd/exhibit-agent` builds only the storage-free
supervisor; it does not build/embed core browser assets. Source invocation needs
Pi **1.3.0** on PATH. The release pins Node **22.22.0** and Pi **1.3.0**, includes
the committed npm dependency lock, and embeds the single shared extension set
from `internal/agentassets`. It needs no database, core volume, encryption key,
operator credential, or browser cookies.

Build a relocatable Linux release (amd64 or arm64):

```sh
sh scripts/package-agent.sh /tmp/exhibit-agent-release amd64
```

The builder verifies Node's published SHA256, runs `npm ci --ignore-scripts
--engine-strict`, and emits the static Go binary, exact Node installation,
locked Pi installation, launcher, version manifest, and checksums. Copy that
directory to a host or Sprite. Register `launch` as the Sprite Service command;
Docker is not required. Roll back the complete directory together, keeping
protocol version compatibility with core. Core image/config changes and Sprite
provisioning are owned by the parent integration.

Provide an assignment-specific, random bearer credential of at least 32 bytes in
a private regular file (mode 0600), provisioned by the trusted coordinator. The
worker does not mint grants and does not accept an operator Exhibit token.
Provider and artifact-scoped credentials arrive only in the authenticated start
body. Put TLS/authentication at the trusted hosting gateway for non-loopback HTTP.

For a Sprite, fix both identities at launch:

```sh
/opt/exhibit-agent/launch --addr 0.0.0.0:8090 \
  --execution-id ATTEMPT --session-id CONVERSATION \
  --token-file /run/worker-token --work-root /tmp/exhibit-agent
```

For a static local standalone endpoint, explicitly opt into authenticated dynamic
assignments. No session UUIDs need to be known in advance:

```sh
bin/exhibit-agent --local-assignments --addr 127.0.0.1:8090 \
  --token-file /run/worker-token --work-root /tmp/exhibit-agent
```

Core uses `agentexec.Remote{Endpoint: agentexec.Endpoint{URL: workerURL, Token:
workerToken, AllowUnassigned: true}}` for that local service. The local service
holds at most 32 isolated assignments, counting closed executions awaiting final
snapshot acknowledgment. Remote `Cleanup` closes and releases an assignment
after core's final checkpoint. Released execution IDs cannot be reused. After
4096 released IDs, restart the local development daemon after draining it.
Production fixed-assignment mode never permits dynamic identity assignment or
reuse across owners. Its authentication file is provisioner-owned and must be
removed when that assignment is destroyed.

Every request includes `Authorization: Bearer …` and `Exhibit-Agent-Version: 1`.
`GET /v1/ready` reports protocol/runtime pins, capabilities, and assignment.
Operations under `/v1/executions/{executionID}` are `POST /start`, `POST /commands`
(prompt, steer, abort, question reply, stats, state, compaction), `GET /events?after=N`,
`GET /snapshot`, and `POST /close`. `/ready` under that execution checks warm/cold
state. The local service also has `POST /release`. DTOs and limits are defined in
`internal/agentexec/protocol.go`; typed error codes distinguish auth/version,
capacity, expired events, rejected commands, and uncertain outcomes.

Command identities are consumed before delivery and retained for the execution;
retries never run them twice. HTTP/event failures terminate the core transport
with an uncertain outcome and never replay an unfinished prompt. Event delivery
uses bounded cursor batches and fails explicitly if retention was exceeded.
Idle closes the event request and all keep-alives, with no health polling or
background reconnection. `Endpoint.Active`, `Idle(ctx, Snapshot)`, and `Release`
let a provisioner manage bounded compute holds and checkpoint acknowledgment.
An explicit new prompt after confirmed idle revalidates `Provisioner.Acquire`.
After a cold restart it requires `Endpoint.LoadSnapshot` to read the acknowledged
central session file and current artifact binding before starting the new prompt.
It never resumes automatically after an uncertain active run.

Pi has a private HOME and TMPDIR, runs from `/`, receives a minimal environment,
and loads only the shipped extensions. Context discovery and built-in tools are
disabled. Scratch has a 96 MiB/1024-entry watchdog, a 64 MiB session-file ceiling,
and is removed after process exit/final snapshot. Daemon startup takes an exclusive
scratch-root lock and removes abandoned execution directories. A worker must have
an execution expiry within 24 hours; each outstanding command has a deadline of
at most two minutes. The default worker runaway-turn backstop is 15 minutes and
may be tightened by core. The worker pauses that timer while waiting on a person.

Manager integration: set `agent.Config.Runner` and `ReserveExecution`. Reservation
runs before resume snapshot/usage reads, and may return a replacement fenced
`*agentscope.Grant`. All session scope/provenance uses that grant. Assigned runs
persist only through `Checkpoint(ctx, ExecutionCheckpoint)` with file, title,
kind (`tool`, `settled`, `interrupted`) and monotonic sequence. Final drain gates
`Release` on successful checkpoint acknowledgment. `Manager.Shutdown(ctx)` stops
admission, drains processes/output, checkpoints, releases, and cleans scratch.
