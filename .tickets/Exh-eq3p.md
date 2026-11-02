---
id: Exh-eq3p
status: open
deps: []
links: []
created: 2026-11-02T15:59:31Z
type: feature
priority: 1
assignee: Max Omdal
parent: Exh-qkl7
tags: [connectors, security, experimental]
---
# Suspend connection grants when an artifact's source changes

Granting a connection to a tool is a decision about that tool's code, but artifact_connection_grants has no link to the source. Any later body change keeps the grant: a Pi or MCP agent edit, a refetch from a site that has since been compromised, a manual edit or a restore. A realistic chain: an external agent reads a poisoned artifact (cross-artifact prompt injection is an accepted risk, docs/security.md §5) and then edits a different tool that already holds the user's GitHub grant, adding code that misuses it.

## Design

- Record the source revision (and widget revision, if widgets can reach the relay) on each connection grant and on relay consent.
- A body change of any origin (edit, agent, refetch, restore) suspends the tool's connection grants for every viewer until that viewer re-approves in trusted app chrome. Suspended grants send no credential; the request fails with a clear, recoverable error in the host chrome.
- The re-approval shows what changed: new network origins from the footprint and a link to the source diff between the approved and current versions.
- Agents (Pi and MCP library grants) cannot re-approve; their routes stay outside the allowlist.
- An unchanged save (same bytes) does not suspend anything.

## Acceptance Criteria

- Tests: an agent write, a refetch, a manual edit and a restore each suspend existing grants; re-approval restores them; an identical save does not.
- A suspended grant never attaches the credential, including for scheduled fetches.
- Browser test of the suspended state and the re-approval prompt; screenshots updated.
- docs/relay-connectors.md and docs/security.md describe the rule.

