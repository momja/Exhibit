---
id: Exh-zue3
status: closed
deps: []
links: []
created: 2026-11-04T05:36:21Z
type: epic
priority: 2
assignee: Max Omdal
tags: [inference, agent, security, experimental]
---
# Platform inference for artifacts: LLM and decision model calls without managing keys

An artifact can call a language model, or a decision model, from ordinary JavaScript. The person running it doesn't need a key, a provider account or any API knowledge. Exhibit makes the call on the server and charges it to the person viewing the artifact.

This is a capability bridge (PRD §6.3), not a connector. The frame never reaches the network. The render preamble installs window.exhibit.ai, the call is posted to the host frame like a state write, and the host makes the authenticated request to the single write path. The CSP and the artifact's origin allowlist are untouched.

Product decisions (2026-11-04):

- Two kinds of call, each its own ticket. complete() is a general LLM call. decide() calls a decision model (a "System One" model such as TypeSafe's Jev): it answers typed questions (yes/no, choice, rating, probability) with a confidence, and never generates text.
- One shared core and one bridge: who pays, whether they may, metering and limits live in one layer (Exh-hwjg), and the person is asked once whether a tool may use AI, never which kind of model.
- No inference on public links or for anonymous visitors, ever. No owner-sponsored links.
- Who pays: the viewer. On an artifact shared with named people, each recipient's calls spend that recipient's own allowance (or their own key), never the owner's.
- Providers follow the agent pattern: platform mode uses an instance credential, self-hosters may configure any supported provider, and BYOK uses the viewer's own stored key. Both kinds use the agent's providers (OpenRouter on the hosted version, which serves Jev as the guardrail already does): INFERENCE_MODEL names a chat model and INFERENCE_DECISION_MODEL a classifier model. Whether calls run through Pi's model registry or new Go clients is decided by the spike Exh-urrg.
- Accounting is separate from the agent's (its own ledger tag, reported separately), but it counts toward the same total usage limits.
- Export decision (2026-11-19): both standalone HTML and ZIP exports disable complete() and decide(), even when online. Preserve editable source for user-directed rewiring; Exhibit supplies an unavailable stand-in and a clear export notice. Local inference, provider/credential setup, Pi packaging and reconnecting an export to the hosted service are out of scope. See [Exh-i902](Exh-i902.md).

Out of scope here: streaming, scheduled inference into saved state for widgets, tool use or multi-step agents inside artifacts, and in-browser local models.


## Delivery plan (2026-11-19, implementation orchestrator)

One production PR on `feature/Exh-zue3/artifact-inference` delivers the epic. The
spike branch (Exh-urrg, PR #306) is evidence only; production reuses its findings and
the shared mockLLM inference script, not its harness.

Ticket ownership in this PR: Exh-hwjg (core), Exh-zbwk (bridge, endpoint, approvals,
UI), Exh-gd3g (complete), Exh-x16u (decide), Exh-i902 (exports), Exh-15tf (guidance,
example, docs). Exh-urrg closes with the recorded decision. Each closes on the branch
only when its acceptance criteria are met; anything left open is stated in the PR.

Runtime: one warm, checksum-pinned Pi Bolt worker supervised by `internal/inference`
(restart backoff, circuit breaker, bounded admission, wall-clock deadlines, no replay of
unknown outcomes), separate from `agent.Manager`. Usage is metered before any
error/validation mapping. Live provider compatibility (notably the classifier route)
is a separate opt-in release check; mock round trips through the real adapters are
the automated evidence.

UI decisions (filling gaps the tickets left open):

- **Name**: the capability is "AI" in every surface ("Use AI"), never a provider or
  model. Kinds are described only to builders (docs, agent guidance).
- **Owner first use**: one dialog on the artifact page names the artifact, says that
  text it sends, possibly including saved data, goes to an AI service, and says who
  pays ("your Exhibit AI allowance" in platform mode, "your own <provider> key" in
  BYOK). Allow sets `inference_approved` and counts as the owner's own consent.
- **Recipient**: approved artifact → one consent dialog about spending their own
  allowance/key. Unapproved artifact → an explanatory notice in chrome, no button.
- **Unavailable states in chrome** (never in the artifact DOM): no key (BYOK) links
  to the agent key setup; limit reached names the limit and reset; AI off on the
  instance is silent (the artifact gets `unavailable`).
- **Security settings**: an "AI" row beside Downloads…Location in the edit page's
  Permissions/Features list (Ask first / Allowed), and an AI line in the posture
  receipt and gallery-card cluster.
- **Settings**: an "AI in artifacts" card: whether AI is available here and who pays,
  this month's use of the shared allowance (platform) as a share of the limit with its
  reset date, call counts per artifact and kind, and a list of artifacts allowed to
  spend your allowance, each revocable. No provider, model or dollar figure in
  platform mode; BYOK says calls bill the person's own provider account.
- **Exports**: the Download HTML help text and the app-bundle ZIP README state that AI
  features are turned off in exported copies.

## Notes

**2026-11-20T06:19:13Z**

Implemented on feature/Exh-zue3/artifact-inference (see docs/inference.md). Limitations: live provider compatibility untested (no paid calls; classifier route /api/v1/systemone vs /api/alpha/decisions unverified — run one manual complete and decide with a real key before enabling in production); window.claude.complete only maps prompt→string; allowance shown as a share, never money; BYOK users see tokens, not cost; the demo's answers come from the deterministic mock.
