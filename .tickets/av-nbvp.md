---
id: av-nbvp
status: open
deps: []
links: []
created: 2026-08-11T06:21:13Z
type: task
priority: 2
assignee: Max Omdal
tags: [docs, architecture, deployment]
---
# Document the per-process assumptions the multi-user epic depends on

Three load-bearing mechanisms introduced or relied on by the multi-user epic are in-process-only, and each is currently a design conclusion baked into the code rather than a constraint documented anywhere central:

- Login rate limiting (av-t21v) -- token buckets held in process memory. The moment the service runs as more than one replica (or behind a round-robin proxy, which deployment.md already describes as a supported shape), the throttle silently becomes no throttle at all: no error, no log line, just a wider window than the operator thinks they configured. The residual distributed-spray gap is already acknowledged and punted to fail2ban, but that punt isn't written down next to the deployment docs where an operator scaling out would see it.
- Agent grant registry (internal/agentscope) -- live per-session credentials and the session registry itself are held in process memory and die with the process; an SSE reconnect that lands on a different replica can never resolve.
- Render token signer fallback -- NewRandomSigner() is used when no server secret is configured, which mints tokens that stop verifying across a restart (or across replicas). Fail-closed and safe, but a third assumption of the same shape.

None of these are bugs today -- the documented deployment model (architecture.md, technical_stack.md 12) is one Go process + one SQLite file, and single-process is a legitimate default. But this is precisely the epic that makes the product multi-user, and these three are the exact seams that break first the moment someone scales past one process. Right now the constraint lives only in scattered code comments; nothing tells an operator or a future contributor 'this deployment model assumes exactly one replica, and here is why, in one place.'

## Acceptance Criteria

- A single doc section (deployment.md or a new 'scaling limits' section referenced from it) names all three per-process assumptions, what breaks under multiple replicas, and the accepted mitigation/workaround for each (e.g. fail2ban for the rate-limit gap).
- Each of the three code locations (rate limiter, agentscope registry, render signer fallback) gets a short comment pointing at that doc section, so the constraint is discoverable from the code, not just from docs.
- No code changes required to close this ticket -- it is a documentation/traceability fix. Actually engineering any of the three out (e.g. a shared rate-limit store) is out of scope and would be its own future ticket if ever needed.

