---
id: av-4xao
status: in_progress
deps: []
links: [av-rcwx]
created: 2026-08-30T01:28:50Z
type: chore
priority: 2
assignee: Max Omdal
tags: [deployment, hosted, infra]
---
# Stop the Fly machine when idle

The single sjc machine runs 24/7 on an instance with very little traffic.

Verified 2026-08-29 against app exhibit, machine 847312b225eed8: the event log
holds three entries total (pending/launch, created/launch, started/start), all
from 2026-08-19T20:03 PDT, and /proc/uptime reads 857561s. That is 9.9 days
continuous since creation, with no stop and no suspend. The deployed service
config agrees with the file ("autostop": false).

It has never stopped because nothing asks it to. Idleness alone does not stop a
Fly machine; the proxy acts only when auto_stop_machines is "stop" or "suspend".

fly.toml:55-58 records why it was set to "off": share links go to people with no
account here and no reason to wait, so a stopped machine turns somebody else's
link into a cold start. Accepting that cold start is the decision this ticket
makes. auto_start_machines is already true, so the proxy holds the request and
boots the machine rather than failing it. The cost is first-hit latency, a few
seconds for a Go binary plus volume attach.

## Design

Two lines in fly.toml, not one.

  auto_stop_machines = "stop"      # was "off"
  min_machines_running = 0         # was 1

min_machines_running is a floor on running machines in the primary region while
autostop is enabled. Left at 1 with exactly one machine in the app, that machine
is exactly the floor, so flipping auto_stop_machines alone would change the
config and stop nothing. Both lines or neither.

"stop" over "suspend" deliberately. suspend snapshots memory and restores it,
which sounds like the better fit for the in-memory agent session registry, but
the sockets do not survive the gap: the SSE stream is broken on resume and Pi's
provider connection is dead, leaving a session that exists and is wedged. A
clean boot is the more honest failure.

Known loss on stop, accepted here and tracked separately:
- internal/agent/agent.go:85 keeps sessions in an in-memory map by design
  (architecture.md 3.7), so live session ids are unknown after a restart.
- The pi subprocess is a child of the service and dies with it mid-turn.
- persistTranscript only runs on agent_settled (agent.go:562), so a turn killed
  before it settles leaves no transcript at all.
- cmd/server/main.go:251 is a bare ListenAndServe with no signal handling, and
  no kill_timeout is set, so Fly sends SIGINT and SIGKILLs 5s later.

The exposure is narrower than it reads. The proxy stops a machine only when it
sees no active connections, and an open SSE stream is one, so a user watching a
chat holds the machine up. Two ways in: widget generation, which returns before
the turn finishes by design (architecture.md 3.7), and anyone who closes the tab
mid-run. Failure mode is a lost transcript or a half-finished artifact,
recoverable by rerunning, not corruption. SQLite is in WAL mode and recovers a
hard kill on next open.

Nothing in the app changes. This is a deploy-config change only.

## Acceptance Criteria

fly.toml sets auto_stop_machines = "stop" and min_machines_running = 0.
The fly.toml comment explains the cold-start tradeoff being taken, replacing the
comment that argues for the opposite.
After deploy, fly machine status shows autostop true in the service config.
After an idle period, the machine event log shows a stop event from flyd.
A request to a share URL after that stop returns the artifact rather than an
error, having triggered an autostart.

