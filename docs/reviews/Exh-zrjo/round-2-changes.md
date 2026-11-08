# Response to baseline council

The same six categories and core-app scope from [brief.md](brief.md) remain in force. Round 1 did not pass (Sol: 7/7/7/6/6/7; Opus: 6/6/7/5/6/7). The target has not been lowered.

## Changes to inspect

- **Hierarchy and consistency:** 56px shared app header, clearer control boundaries in both themes, shared keyboard focus ring, relative type sizes, primary labelled desktop Add artifact. Gallery now has a visible h1 and short introduction. Library cards use the shared rounded surface, readable titles and a stable visible menu target. Mobile menu targets remain 44px.
- **Navigation:** app return links consistently say Library. The viewer shows the tool name, offers a direct desktop Edit control, and retains its full-canvas floating placement. On narrow phones editing remains in the overflow so the name and 44px controls fit. Settings uses a complementary account heading and shared link-button styling.
- **Creation:** native ES module with explicit handlers/imports. All three methods use field-associated inline errors, aria-invalid and focus. CodeMirror inherits the textarea's accessible name/description; editor source and widget source have distinct names. Ingest has an immediate repeated-submit guard, disables source/method changes while pending, restores the working copy after failure, and locks a completed creation during the network approval step. Progress/server failures use a live status. The existing snapshot-report rendering is unchanged.
- **Empty/search feedback:** full-width contextual empty states with actions, htmx reset links, first-page pending feedback and a stable polite results announcement refreshed by the server partial.
- **Editor:** permission summary has a section name and concise posture summary. Save is visually quiet when clean and primary when dirty. Version rows wrap, dates get their own phone row, and history actions have 44px targets. Delete is visually separated from return navigation and uses a restrained danger treatment.
- **Viewer:** 36px desktop and 44px phone/coarse controls, 44px restore tab, named tool, keyboard arrow/Home/End menu navigation and focus-exit dismissal. Existing floating-header controller still owns positioning and cleanup.

## Evidence and fixture corrections

The first baseline screenshot attempt mistakenly captured login screens (`urllib` scoped the single-label hostname cookie to `t3code.local`). Those images were replaced with authenticated baseline captures after reviewers had already inspected the originals. The baseline reports are preserved as originally written; their screenshot warning describes that initial attempt. The baseline service also stopped briefly and was restarted independently. Neither limitation is hidden or counted as an interface failure.

The new capture script `e2e/capture-ui.mjs` signs in through the ordinary login, asserts the route and main UI marker, checks horizontal overflow, records titles/console errors in a manifest, and captures both themes. Screenshots for this build are in `docs/screenshots/Exh-zrjo/round-2/`.

Scratch fixture now includes Health/Reading/Utilities tags, a Daily tools collection, an outgoing public link and person grant, plus an incoming Shared packing list from council-guest. Source/widget saves give Run Log populated history. Gallery search/filter misses remain available. Normal local settings and sharing can now be inspected live. No production data is used.

## Validation

Validation commands and final results are recorded in `validation.md`. New browser regressions check field naming/focus, repeated submits during a gated request, refusal recovery, narrow/enlarged history and viewer keyboard/restore behavior. Earlier runs exposed and fixed a 32px phone card-menu flex basis. A disk-full incident caused unrelated SQLite/trace-write errors; disposable Go cache was cleared before rerunning.

## Remaining review decisions

Standard navigation headers and the floating viewer are intentionally different layout variants: the viewer must leave the artifact canvas available. Their colors, control semantics, names and keyboard/touch contract should still be judged for consistency. Accounts stays as a secondary desktop shortcut and remains available in Settings on phones. Agent provider execution and experimental connectors remain outside the fixed scope because the scratch instance has no provider credentials. Do not inflate scores to reach the target; identify any substantial unresolved defect.
