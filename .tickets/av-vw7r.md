---
id: av-vw7r
status: closed
deps: [av-bavj, av-b4yh]
links: [av-bavj, av-b4yh, av-y7td]
created: 2026-10-03T14:00:00Z
type: feature
priority: 2
assignee: Max Omdal
tags: [versions, agent, security]
---
# View an earlier version without restoring it

Returning to an earlier version ([[av-bavj]]), or to a conversation tied to one ([[av-b4yh]]), changes the artifact's code and saved data at once. A person has to be able to see what they would get before choosing it. In that view nothing may be persisted, but the version's data has to be there: it is the code *and* the data that make it what they remember.

Follows [[av-b4yh]], whose notes listed "previewing the old version before choosing" as not done.

## Design

- **The same pair a restore puts back.** `Store.GetVersionView` returns a version's body blob and the state snapshot it left behind, in one owner-scoped query, and writes nothing. The head is `ErrAlreadyCurrent` (its data is the live rows, and the artifact's own page shows it). A test restores a viewed version and compares, so "what you viewed is what you get" is a property of the code.
- **A render document of its own**: `GET RENDER_ORIGIN/a/:id/versions/:seq`. Same security envelope as the artifact (the current allowlist, an opaque-origin sandbox, `no-store`, framed by the app origin alone), narrowed in what it writes.
- **History has its own credential.** A render token's MAC already mixes in the name of the document it is for; for a version that name is `rendertoken.VersionScope(id, seq)`. A token for the live document — which the artifact can read from `location.href` — does not open history, and a version's does not open the live document. No new claim, no wire change. Owner-only: minted only for the owner and refused if it names anyone else.
- **Nothing it does persists, three independent ways.** (1) The shim's `VERSION_VIEW` flag stops write-through and refuses a resync over the snapshot. (2) The host: the version frame is a different element from the page's artifact frame (no `#pv-frame`), and every bridge listens to that id alone, so a message the artifact forges by hand is not heard — this is the half the other two cannot cover, and the anonymous render's "the API's auth is the enforcement" does not apply because the framing page is the owner's own. (3) The response's own CSP `sandbox`, so even opened top-level it has an opaque origin with no IndexedDB or cookies.
- **One fragment, two pages.** `/partials/version-viewer` renders the `versionViewer` partial (a bar saying it is a preview and nothing is saved, over the frame) and the pages swap it in: the chat's preview pane from the History card ("View vN in the preview first", above the rollback choice), and a slot above the list in the edit page's Versions panel, where a Restore sits beside the view because that is where the decision is made. The chat's decision stays in its card, so its pane offers none.
- It ends on *Back to current*, when the conversation is continued, or when an agent save re-renders the pane.

## Acceptance Criteria

- A version shows the code as it was and the data it left, not the live data (`TestAVersionIsServedAsItWasWithTheDataItLeft`, `TestAVersionIsViewedWithTheDataItLeftBehind`).
- Its document cannot persist: shim flag, sandbox CSP, no devices, no cache (`TestAVersionDocumentCannotPersistAnything`; `render.shim.test.mjs` runs the shim). Each guarantee was broken on purpose and the tests failed.
- Tokens are per document: the live token opens no version and a version's opens nothing else (`TestATokenOpensItsOwnDocumentAndNoOther`, `TestATokenOpensTheDocumentItWasMintedForAndNoOther`), and only for the owner (`TestAVersionIsOnlyServedToItsOwner`).
- The URL the app mints is the one the real render handler serves (`TestTheMintedVersionURLIsServedByTheRenderSurface`).
- Another owner's artifact answers as one that is not there, in the fragment and on the render origin.
- In a real browser: the view shows 25 where the live data is 99, the artifact counts inside it, live data is unchanged, no write request leaves the page, a hand-forged state message does nothing while the same message from the artifact frame is persisted, a version URL opened top-level has no IndexedDB where the live document does, and rolling back afterwards gives exactly the 25 that was viewed.

## Notes

Not done, deliberately: a diff between the viewed version and the current one, and viewing a version's widget (a widget-only version looks identical to its predecessor in the artifact; its label says what changed). The version's bridges are present in the document but nothing hosts them for it, so downloads, clipboard and links do nothing in a view rather than raising prompts that would write per-artifact authority.

Registers one icon (`ph-eye`) in the Phosphor subset.
