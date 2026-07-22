---
id: av-td4y
status: closed
deps: []
links: []
created: 2026-07-22T04:11:25Z
type: feature
priority: 2
assignee: Max Omdal
---
# Agent mobile: segmented Chat/Preview toggle (no sidecar)

The agent surface (internal/api/agentui.go) is a .layout flex row with .chat{width:460px} beside .preview{flex:1}. On narrow screens the two-pane sidecar doesn't fit, so the artifact preview is effectively unreachable while chatting. Redesign for mobile (<=640px): a segmented Chat / Preview toggle so each pane gets the full screen, one visible at a time, with a 'preview updated' nudge indicator so an agent save isn't missed while on the Chat pane.

## Design

Single file: internal/api/agentui.go (HTML+CSS+JS are one Go string; no build.mjs step). Add a 2-button segmented control (display:none on desktop, shown under @media (max-width:640px)). Drive which pane shows with a body state class: default Chat visible, .preview{display:none}; body.pv shows preview and hides chat. Under the media query .chat,.preview{width:100%;flex:1}. JS (~10 lines): segment click handlers flip body.classList; in handleAgentEvent's existing exhibit_artifact_saved case (agentui.go ~437) add a dot/badge class to the Preview segment when currently on Chat (the 'preview updated' nudge), cleared when Preview is shown; in the __exSnippet==='captured' handler (~590, already focuses input) switch back to the Chat pane. Model chip (key-btn) gets max-width + ellipsis on mobile. Switch height:100vh -> 100dvh in the mobile query. Desktop layout unchanged.

## Acceptance Criteria

At <=640px: a Chat/Preview segmented control is visible; exactly one pane shows at a time and toggling swaps them full-screen; an agent artifact-save while on Chat lights an indicator on the Preview segment that clears when Preview is opened; capturing a snippet returns to Chat with the element attached. Desktop (>640px) renders identically to today (segmented control hidden, side-by-side sidecar intact). Agent page still serves (TestAgentPageServes) and the code compiles. Manual mobile-viewport verification screenshot attached to the PR.

