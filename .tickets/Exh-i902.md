---
id: Exh-i902
status: closed
deps: [Exh-zbwk]
links: []
created: 2026-11-04T05:37:11Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, export, experimental]
---
# Exports: disable AI inference and preserve editable source

Both standalone HTML and ZIP exports disable Exhibit-provided AI inference, including complete() and decide(). Export preserves editable artifact source so a person can hand the bundle to another agent and arrange their own replacement. Exhibit does not provision, reconnect or rewire inference for exported artifacts.

Product decision (2026-11-19): inference portability is out of scope. This supersedes the earlier ZIP local-inference proposal and any research notes requiring a local Pi runtime for exports. Inference remains disabled in exports even when they are online.

## Design

- Both formats install a minimal window.exhibit.ai stand-in before artifact code runs: available() reports no available kinds, and complete()/decide() reject with the same stable unavailable error as the hosted API. The stand-in makes no network or relay requests and inserts no markup into the artifact's DOM.
- Artifact code handles unavailable explicitly: AI-dependent actions are disabled with an explanation, while other functionality remains usable where possible. The agent guidance and example artifact cover this behavior ([Exh-15tf](Exh-15tf.md)).
- The export dialog says AI features are disabled in exported artifacts. ZIP documentation repeats that limitation and points to the editable source for users who want to implement their own inference integration. It does not present an inference setup wizard or promise automatic portability.
- Preserve the artifact's inference call sites and editable source in the source bundle. Export adds the unavailable stand-in rather than attempting to translate prompts, models, providers or API calls. User-directed rewiring is outside Exhibit's supported export behavior.
- No inference route in the ZIP relay, provider SDK/client, Pi runtime requirement, credential-entry flow or authenticated connection back to Exhibit is added for inference. Existing connector export behavior is unaffected.
- No credential, instance inference configuration or live Exhibit inference endpoint is written into either export.

## Acceptance Criteria

- Both HTML and ZIP exports of the example artifact load, report no available AI kinds, and reject complete()/decide() with unavailable. AI-dependent controls explain the limitation, with no uncaught rejection; non-AI features remain usable where possible.
- Browser tests with mock inference endpoints assert zero inference requests for either export, including when online and when unrelated connector relay configuration exists.
- The export dialog and ZIP documentation explicitly state that AI features are disabled in exports.
- The source bundle retains the editable artifact source and its inference call sites so another agent can rewire them independently.
- Tests assert neither format embeds credentials, instance inference configuration or a live Exhibit inference endpoint, and the ZIP has no inference relay route or runtime requirement.

## Notes

**2026-11-20T06:19:13Z**

Done: every HTML and ZIP export installs the unavailable stand-in first in <head> (ZIP: before the bundle shim freezes window.exhibit); available() reports nothing; calls reject unavailable with zero network requests; source and call sites unchanged; no credentials, config or endpoint exported; Download HTML, ZIP panel and README say AI is off. Connector export behavior unchanged (bundle tests).
