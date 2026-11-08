---
id: Exh-dabd
status: closed
deps: []
links: []
created: 2026-11-08T16:15:53Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent, mcp, state, privacy, security]
---
# Let owners keep saved data away from agents

Agents run on an inference provider: the built-in Pi agent sends tool results to whichever provider is configured (BYO key or the platform key), and a connected MCP agent sends them to the provider behind the user's own client. Today the Pi agent has get_state/set_state/delete_state on its artifact and a library grant has read-only get_state across the owner's library, so anything a person typed into their tools (a journal, a budget, health numbers) can be read into a provider's context without the person having decided that.

Make access to saved data an explicit owner decision, denied by default to match the deny-by-default security model: an account-wide default plus a per-artifact override.

## Design

- Account default: new owner_settings(owner_id PRIMARY KEY, agent_state_access 0|1 DEFAULT 0, updated_at). Keyed by owner_id with no users FK, because a no-login owner has no users row (same reasoning as agent_grants). Missing row = off. Removed by account deletion.
- Per-artifact override: artifacts.agent_state_access TEXT NOT NULL DEFAULT 'inherit' CHECK IN ('inherit','allow','deny'). Not versioned (like approvals). Duplication copies it, so a copy of a 'deny' artifact cannot inherit an 'allow' default.
- One resolver: Artifact.AgentStateAllowed(accountDefault) — inherit uses the account default; allow/deny override it. Every caller goes through it.
- Enforcement is server-side on the state routes' agent branch (state.go artifactExists): any agent principal (Pi grant or library grant) reading or writing state on an artifact whose effective access is off gets 403 with a message saying the owner has not allowed it. Prompt text and tool registration are UX, not the boundary.
- An agent cannot widen this: PATCH agent_state_access from an agent principal is refused (like share_state_mode); the account route is outside agentScopeAllows.
- Pi: the session's effective access is computed at creation (bound artifact's, or the account default in create mode) and passed as EXHIBIT_STATE_TOOLS=1; state tools are only registered, and only listed in the system prompt, when allowed. Otherwise the prompt says saved data is unavailable and where the person can change that. A change mid-session takes effect on the server immediately and on tool registration at the next conversation.
- MCP: get_state stays listed (the tool list is per connection, not per artifact); a 403 becomes a tool error telling the agent the owner has not shared this artifact's saved data, not to retry, and that the person can allow it in Exhibit.
- UI: Settings gets an 'Agents and saved data' card with the account default applied immediately (PUT /api/settings/agent-state-access). The edit page's Stored state panel gets a per-artifact select (Use account default (currently Off/On) / Allow / Don't allow), applied immediately through PATCH, owner-only.
- Copy is honest about the remainder: agents still see the artifact's source, and anything the person selects in the preview or pastes into the chat.

## Acceptance Criteria

1. With no settings changed, a Pi session and an MCP library grant both receive 403 on GET/PUT/DELETE /api/artifacts/:id/state, and Pi registers no state tools.
2. Turning the account default on lets both read state on artifacts set to 'inherit'; an artifact set to 'deny' stays refused; an artifact set to 'allow' is readable with the default off.
3. An agent principal cannot set agent_state_access via PATCH, and cannot reach the account setting route.
4. The edit page and Settings controls persist the choice and say what the default currently is.
5. Duplicating an artifact preserves its override.
6. Account deletion removes owner_settings rows (TestEveryTableIsAccountedForInAccountDeletion).
7. Docs updated: PRD §4.4/§8.7, architecture §3.1/§3.3/§3.7, security.md §5, agent.md, api.md.


## Notes

**2026-11-08T16:44:53Z**

Implemented as designed. Migration 054 adds owner_settings and artifacts.agent_state_access. Enforcement is the agent branch of state.go artifactExists (403 for Pi and library grants). Pi registers state tools only with EXHIBIT_STATE_TOOLS=1 (CreateOpts.StateTools, computed at session start); the prompt gets state_tools.tmpl or state_unavailable.tmpl. MCP get_state maps the 403 to a do-not-retry tool error. UI: Settings card (checkbox, PUT /api/settings/agent-state-access) and a select in the edit page's Stored state panel (PATCH agent_state_access), both through the agent-state-access Stimulus controller and applied immediately. Not covered, and documented in security.md 5.6: source, preview selections and chat text.
