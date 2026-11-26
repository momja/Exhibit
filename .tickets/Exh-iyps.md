---
id: Exh-iyps
status: closed
deps: []
links: []
created: 2026-11-26T05:56:28Z
type: bug
priority: 2
tags: [agent, preview]
---
# Agent artifact preview goes blank after an edit or tab reload

## Agent edit swaps in a blank preview

The user observed the artifact preview go blank when the agent saved a change and the new artifact version was swapped in. To investigate, open an artifact in the agent view, ask the agent to make a visible change, and wait for the preview swap.

Expected: The preview renders the saved version in the same agent view.

Reported result: The preview goes blank after the new version is swapped in. The conditions that trigger this and any relationship to the tab-reload case remain to be established.

## Tab reload clears a newly created artifact preview and conversation

When an artifact is created from a fresh agent view, the preview and conversation appear, but the browser URL stays at /agent. Reloading that page opens the empty agent view again.

1. Open Add to Gallery, select Build with agent, and enter “Build a second click counter.”
2. Click Start building. Wait for “Artifact created” in chat and for Click Counter to render in Preview.
3. Reload the browser tab.

Expected: The agent view opens with the saved artifact preview and the conversation that created it.

Actual: The reload returns HTTP 200. The agent shell and composer remain, but the previous messages and preview iframe are gone. The pane says “Your artifact will appear here.” The artifact is still saved.

## Verification

Reproduced on two fresh sessions on local main at 854ac35dab3eb792a5c37a7df1f57dcec10efd0b in Chromium 152.0.7977.169 with Pi 1.4.0 and the deterministic mock provider. No browser console error accompanied the successful save and reload.

As a control, opening /agent?artifact=<saved-id> and reloading preserved the preview. In the same fixture, an agent edit on that pre-bound route refreshed the preview via GET /partials/agent-preview (HTTP 200) without a top-level reload. The reported blank preview after a save remains unverified. Related work covers preview refresh after a save (av-6m3e) and persisted conversation history (av-y7td).

## Acceptance Criteria

- After the agent saves an edit, the preview swap displays the new artifact version and keeps the conversation visible without reloading the browser tab.
- After a new artifact has been saved from Build with agent, a browser reload restores its preview and the available conversation.
- Reloading neither creates another artifact nor resends the original prompt.
- Opening a fresh agent view and reloading the view for an existing artifact continue to work.
- Add browser regression coverage for the save-triggered preview swap and the create-then-reload path.

## Notes

**2026-11-26T06:54:14Z**

Reproduced both cases in T3 Chromium 152 with Pi 1.4 and the deterministic mock. With relay enabled, an ordinary edit returned a successful preview fragment and kept the same document and exact CSP, but htmx attribute settling removed the new iframe src after relay-host set it. The agent preview swap now skips attribute settling. Creation binds the current browser entry to the artifact and its saved session, so a reload renders the available conversation through History without starting or resuming a session. Reload before the first transcript is stored still shows the artifact. Both Chromium regression scenarios failed on the original implementation and passed after the fix. T3 also verified fresh and prebound reloads, the purple edit, and one artifact with no replay of the opening prompt.
