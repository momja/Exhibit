---
id: Exh-3r39
status: closed
deps: []
links: []
created: 2026-10-24T21:20:06Z
type: task
priority: 2
assignee: Max Omdal
tags: [docs, onboarding, self-host, agent]
---
# Add copy-paste agent guides for installing and deploying Exhibit

Add copy-paste setup guides that a person can hand to a coding agent to have it install, configure, and deploy Exhibit on their own machine. The agent should perform the setup and explain progress, rather than merely return installation instructions for the person to execute.

README.md and docs/deployment.md already document individual commands and configuration. Provide a clear entry point and complete prompts/runbooks that turn those references into a guided, end-to-end setup experience.

## Design

- Add a discoverable “Set up with an agent” entry in README.md, linking to the guides under docs/ and providing a ready-to-copy prompt. Make each guide usable when pasted into a fresh agent session, with the repository URL and enough context to start before a checkout exists.
- Have the agent inspect the machine's OS, architecture, shell, available tooling, occupied ports, and existing Exhibit installation. Ask only for choices or credentials it cannot infer; state supported platforms and explain any unsupported prerequisite.
- Default to Docker Compose for self-hosting, with a clearly separated source-build guide. Download/install missing dependencies with platform-appropriate commands and verify versions. Source setup must include the current Go, Node/npm, make, asset-build prerequisites, and the pinned Pi runtime; derive versions from repository sources rather than duplicating stale pins.
- Clone or safely reuse the checkout, install dependencies, build as needed, and write persistent environment configuration using the mechanism the chosen deployment actually loads. Explain required versus optional settings, distinct app/render origins and ports, persistent data storage, authentication/bootstrap, and agent provider credential setup. Collect secrets locally without committing or repeating them in the handoff.
- Start Exhibit on the person's machine and document how to stop, restart, and inspect logs. Keep local-only setup straightforward; explain the additional origin, authentication, and TLS configuration when the person chooses network access.
- Verify the gallery/API and separate render origin, import and run a small artifact, confirm persistence after restart, and check that the Pi runtime and agent credential path are ready. Exercise an agent interaction when credentials are supplied; identify any outstanding credential step explicitly.
- Finish with the actual reachable URLs, configuration/data locations, launch method, and any remaining steps. Include focused recovery guidance for missing tools, failed builds, port conflicts, origin mismatches, and missing Pi/provider credentials. Re-running setup must preserve existing configuration and data.
- Keep docs/deployment.md as the configuration reference and link to it; reconcile conflicting setup instructions encountered while writing the guides. This ticket is for the guides and copyable agent prompts, not a new installer or hosted provisioning service.

## Acceptance Criteria

1. README exposes copy-paste agent setup prompts and links to complete Docker Compose and source-build guides.
2. Starting from a machine without Exhibit or its prerequisites, an agent following each supported guide can install/download dependencies, obtain the repository, persist environment settings, and launch Exhibit with minimal user input.
3. The guides cover the full product, including the Pi runtime and agent credential setup, and preserve the separate app/render origins and persistent data.
4. Each guide contains concrete verification and troubleshooting steps, plus a final handoff with usable URLs and stop/restart/log commands. Missing credentials or unsupported platforms are reported clearly rather than described as a completed setup.
5. Walk through each documented path in a clean supported environment, record platform and results, and verify restart persistence and safe re-runs without overwriting an existing installation's data or configuration.


## Notes

**2026-10-24T22:04:36Z**

Implemented README prompts plus shared, Compose, and source setup runbooks. Validated fresh Compose data and a clean Ubuntu ARM64 source build, login and artifact/render/state flows, restart/re-run persistence, pinned Pi, environment loading, and Git/Docker exclusions. Results and untested platform/provider steps are recorded in docs/agent-setup-validation.md.
