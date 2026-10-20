---
id: Exh-i0ll
status: in_progress
deps: []
links: [Exh-yvhp, Exh-099b, av-rcwx, av-4bzn, Exh-p4zz, Exh-nueb]
created: 2026-07-12T03:30:17Z
type: epic
priority: 2
assignee: Max Omdal
---
# Standalone agent workers on Fly Sprites: horizontal scaling and idle compute release

Run agent execution independently of the Exhibit web/database host. Concurrent
conversations should use separate, on-demand Fly Sprites rather than require a
larger shared VM with every Pi subprocess resident on it. Retain a standalone
Go runtime, independently packaged in `exhibit-agent`, that accesses Exhibit
only through authenticated HTTP APIs.

## Replan — 2026-10-20

This replaces the July extraction plan in this ticket, its open children, and
`docs/agent.md` §Extraction. That plan assumed moving the whole agent UI,
BYOK storage, and handlers to another origin. The scaling boundary is now
execution: keep the chat UI and browser-facing routes in Exhibit and extract
the worker, as confirmed by the owner on 2026-10-20. A separate UI origin is
not required for horizontal scaling.

The agent has acquired dependencies the old plan does not cover:

- Multi-user login and owner-scoped sessions; per-artifact tool credentials
  must still prevent one conversation from reaching another artifact.
- Platform credentials as well as encrypted BYOK, usage-policy screening,
  usage accounting, owner/session/instance spend caps, and the ten-session
  owner limit. These cannot become independent counters on every Sprite.
- Resumable Pi session files, conversation identity and lifetime spend,
  artifact-version provenance, History, and explicit rollback/continue flows.
- Widgets, state, image/snippet context, SSE replay/reconnect, and the current
  server-rendered preview/History fragments.
- Active readiness/source-cache/concurrency work (Exh-099b) and resource-bound
  fixes (av-4bzn, Exh-p4zz, Exh-nueb). Integrate the resulting contracts rather
  than extracting a stale July snapshot. In particular, conversation preparation
  must not create unbounded speculative Sprite compute while someone types.

Current code still reads and writes the Store for transcripts, usage recording,
resume baselines, and spend checks, and mints grants through an in-process
registry. Transcripts are no longer the only extraction seam.

## Target design

1. **Exhibit remains the durable authority.** It owns login, authorization,
   encrypted BYOK storage, platform configuration, artifact APIs, transcripts,
   usage ledger, budgets, and session admission/assignment. The browser keeps
   using Exhibit's origin, existing chat routes, SSE authentication, template
   partials, and render-origin bridges. Workers do not mount its SQLite file,
   blob directory, or secret key. Scaling the SQLite-backed core itself is a
   separate problem; this epic removes agent execution from that host.
2. **The standalone worker owns execution.** Its Go supervisor runs the pinned
   Pi/Node runtime, extensions, and per-execution scratch directory; accepts
   prompt/steer/abort/close commands; and reports events, usage, and session
   snapshots over the service API. No Store imports, direct database access,
   login UI, or persistent worker-owned key database. Support the same runtime
   as a local standalone process for development/self-hosting; keep Sprites
   provisioning behind an execution-backend interface.
3. **One Sprite per active conversation assignment initially.** Admission and
   assignment happen centrally before provisioning; the worker starts on demand
   when execution is needed. Keep the assignment while useful, close its
   connections when idle, and reclaim it after a configured idle retention
   period. A later prompt may wake it or use a fresh Sprite restored from the
   centrally saved conversation. Conversation IDs outlive Sprite/process IDs.
   No cross-owner reuse of a filesystem containing conversation data or keys.
4. **Durability is external to the Sprite.** A Sprite is replaceable even though
   its disk persists. Save complete Pi records after completed tool calls and
   settled turns, flush on orderly shutdown, and acknowledge durable snapshots
   before reclaiming a worker. Resume on another worker retains conversation
   ID, title, version association, current instructions, and recorded spend.
   Surface interrupted/uncertain runs; do not automatically replay an unfinished
   prompt or a write whose outcome is unknown. Existing successful API saves
   remain saved even if subsequent execution dies. Coordinate with av-rcwx.
5. **One writer and one set of limits across the fleet.** Persist assignment,
   execution-attempt identity, and lease state centrally. Fence superseded
   workers at every artifact-write and transcript-write boundary, including
   server-side create-mode binding. Idempotent commands/checkpoints/usage must
   tolerate retries without duplicate runs, stale snapshots, or double counting.
   Preserve current budget policy and bound overshoot fleet-wide; propagate
   instance hard stops and turn deadlines to the responsible workers. Bound
   provisioning, active workers, backlog, and subscriber memory; refuse capacity
   with a useful response rather than accumulate an unbounded queue.
6. **Separate infrastructure, supervisor, and tool authority.** Only the trusted
   coordinator holds the Sprites management credential. The worker supervisor
   receives narrowly scoped, expiring authority for its assigned execution;
   Pi tools receive only their existing owner/artifact-scoped grant. Provider
   keys needed for that execution may reach Pi's minimal environment; neither
   the operator's Exhibit token, management tokens, nor other users' keys may.
   Transcript persistence and usage reporting remain supervisor-only operations.

## Fly Sprites lifecycle and cost requirements

Fly describes Sprites as persistent environments with idle compute suspension,
not disposable filesystems: disk survives sleep, warm suspension may preserve
memory, cold starts lose processes, and connections must be re-established.
Our worker lifecycle deliberately treats them as disposable.
[Lifecycle and persistence](https://docs.fly.io/sprites/concepts/lifecycle).

Use a registered Sprite Service for cold-start bootstrap, authenticated worker
access, and a bounded Tasks API hold while a run must remain active. Release
the hold and all worker streams/keep-alive connections after settlement or
abort. A browser left on an idle chat must not keep its Sprite billable; idle
health polling must not repeatedly wake it.
[Services](https://docs.fly.io/sprites/concepts/services),
[keeping a Sprite running](https://docs.fly.io/sprites/keeping-sprites-running).

No always-running worker floor. Retained disk still costs money, so include
automatic deletion of expired/orphaned Sprites, scratch/credential cleanup,
and a measured active-versus-idle cost check. Record actual platform creation
and concurrency limits at rollout; horizontal capacity is bounded by those
limits and the operator's configured ceiling.
[Sprites pricing and limits](https://fly.io/sprites/).

## Delivery order

1. Exh-v6v4 — trusted snapshot persistence and resume reads through the API.
2. Exh-hz3g — core/worker API, credential, usage, and policy boundary.
3. Exh-k75k — extract/package the standalone worker and exercise it locally.
4. Exh-xw7h — central assignment, fencing, routing, and fleet-wide limits
   (can proceed alongside extraction once the API boundary is defined).
5. Exh-fuxp — on-demand Sprites backend, lifecycle cleanup, and rollout proof.

The former plugin-ecosystem subtasks remain closed. No plugin registry,
arbitrary tool/plugin ecosystem, new billing product, or database-platform
migration is part of this epic. Update architecture/deployment documentation
with the implementation; this ticket is the planning authority until then.

## Acceptance Criteria

1. The Exhibit host runs no Pi subprocesses in remote-worker mode and needs no
   Node/Pi install for chat availability. The standalone worker runs locally
   and on Sprites without direct access to Exhibit's storage.
2. At least two conversations execute concurrently on distinct Sprites with
   owner isolation and centrally enforced owner/instance capacity and budgets;
   adding execution capacity does not require resizing the Exhibit VM.
3. An idle open chat permits its worker to suspend without compute charges;
   the next prompt works after warm wake, cold restart, or worker replacement.
   The configured retention policy removes expired/orphaned Sprites.
4. Concurrent resume, retries, network partitions, core/worker restart, and
   lease expiry cannot create two effective writers, duplicate a command, reset
   conversation spend, or overwrite a newer checkpoint with a stale one.
5. Worker loss preserves committed artifact writes and acknowledged snapshots;
   recovery reports interruption and resumes from durable history without
   silently rerunning unfinished work. Usage reporting retries do not double
   count, including valid late usage from a terminated execution.
6. Current create/modify/widget/state, guardrail, BYOK/platform, snippet/image,
   SSE, History/resume/rollback, version, and conflict behavior remains covered.
   Tool grants cannot read raw session files, write transcripts, mint workers,
   approve capabilities, or reach another owner's data.
7. Deployment verification records cold/warm readiness, concurrent execution,
   idle compute release, retained-storage cost, and bounded capacity failure.
   Operators have documented rollout, rollback, cleanup, and failure diagnosis.


## Notes

**2026-10-20T07:30:55Z**

Implementation delivered as a four-layer PR stack: durable authority, Sprites backend, standalone runtime, and production core integration. Chat/login/BYOK/History/budgets remain in Exhibit. Review-agent findings were fixed and re-reviewed. Code children are complete; epic remains in_progress with Exh-fuxp for credentialed live Sprite rollout, telemetry and cost/suspension evidence. No deployment or paid resources were created in this environment.
