---
id: Exh-77m9
status: closed
deps: []
links: []
created: 2026-11-04T04:58:45Z
type: feature
priority: 2
assignee: Max Omdal
---
# Redesign gallery tabs as Starred, Search, Shared, and Activity

Replace Library with Starred (personal persistent stars through artifact action menus, ordered by last opened, no filters). Combine existing Search and Library into Search with browse-all and search across accessible artifacts. Combine sharing spaces into Shared. Add Activity for running agents and artifact/data changes with version, description, and links opening the corresponding conversation in the agent panel; use honest placeholders where the audit data is unavailable.

## Acceptance Criteria

Four consistent desktop/mobile tabs; Starred has no filters and tracks last opened per viewer; Search browses/searches all accessible artifacts; Shared includes incoming and outgoing shares with correct permissions; Activity uses available owner-scoped agent/version data and clearly identifies unavailable audit coverage. Verify persistence, authorization, navigation/history, responsive layouts, and update screenshots.


## Notes

**2026-11-04T05:30:51Z**

Implemented four persistent gallery destinations on feature/Exh-77m9/gallery-tabs. Personal SQLite-backed stars and opened history; browse-all Search retains organization filters; incoming/outgoing Shared; owner-scoped Activity from version metadata and running session snapshots. Saved conversation links render history without starting an agent; live links observe existing SSE and do not close the original session. Explicit Coming soon placeholder for missing saved-data/account audit events. Updated responsive/browser/security/account-deletion coverage and eight reproducible screenshots. Full-suite verification in progress.

**2026-11-04T05:39:05Z**

Completed. Validation: go test -short ./... passes across 24 tested packages; go vet ./... passes; full agent package passes. Focused final compatibility/migration/account-deletion tests pass for both authenticated viewers and no-login owner 1. Chromium coverage passes for search/browse/clear/paging, star/unstar and real viewer opening telemetry, activity saved-conversation deep links, desktop/mobile/dark/large text tabs, filter sheets, htmx navigation/history/races, preserved account/header lifecycle, and editor return context. Stabilized browser navigation waits; new redesign flow passed three consecutive runs at 320, 390, and 1280px. Eight screenshots refreshed with reproducible screenshot fixture; interactive preview checked at 360 and 728px. Activity uses real version metadata and running-session snapshots; detailed data-only/account audit history remains explicitly marked Coming soon. No merge or deployment.

**2026-11-04T05:53:31Z**

Read-only review against origin/main merge-base 44def51a found four P2 defects, all reproduced against built modules: (1) agent.js:1221 live-ticket loading races send/preparation and can prompt a newly created conversation; (2) agent.js:437 observer renders a locally echoed user message again on message_end; (3) agent.js:1219 observer only attaches at boot, so pagehide/pageshow history restoration leaves the stream closed and sends to a new conversation; (4) gallery.js:13 returns before registering pagination failure handlers for Activity, hiding load errors. Core ownership, migration/account deletion, navigation, and unified search/sharing paths otherwise reviewed without qualifying findings. Fixing these before PR.

**2026-11-04T06:01:27Z**

Review fixes complete: live Activity attachment and Send share the bounded session lifecycle; typing cannot prepare an unrelated session, late tickets cannot replace selected history, failed connections/replies retain drafts, observer SSE owns user messages, cached history restoration reconnects with a fresh ticket and rebuilds the replayed transcript, and Activity pagination retains error/retry/stale-request handling without a search form. Six regression cases failed before the changes; all now pass, plus rejection/navigation coverage. Validation: complete TestGalleryPageScriptSuite; GalleryRedesign, SpaceNavigation, SpaceNavigationRaces, Spaces, SearchTab, SharedGalleryRecipient browser tests; AgentComposer, AgentMarkdown, AgentPanes browser tests all pass. Prior full go test -short ./... and go vet ./... passed. Diff whitespace checks pass. Preparing PR against unchanged origin/main 44def51a.
