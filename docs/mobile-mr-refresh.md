# Mobile MR refresh — 2026-11-02

The four remaining mobile changes are refreshed onto main after the navigation stack merged. Each remains independently reviewable. Their heads merge cleanly in MR order, and the resulting tree matches the combined source tested below.

Base: `9b8f08df4f351275cc8596705ccdf7b015120b24`. Combined verified source: `91d97b168256a184952f91a74f3f8f2f1c0929ec`.

| MR | Reviewed head | Individual evidence |
| --- | --- | --- |
| [#258](https://github.com/momja/Exhibit/pull/258) — Creation chooser | `ed79fceb4c1227ef7d131c5c1307bf67ae161e50` | 22 creation tests/subtests |
| [#260](https://github.com/momja/Exhibit/pull/260) — Header and touch targets | `24de34a844fc0d1d04a226fe736884aaba63377b` | 113 initial focused checks; 54 final account/header checks |
| [#261](https://github.com/momja/Exhibit/pull/261) — Readability | `c05d19fc786d5b9dd50d0faadec2b51b73122c79` | 35 final typography/notice tests/subtests |
| [#263](https://github.com/momja/Exhibit/pull/263) — Chat/Preview tabs | `11ba1265b7eaead57dd9037a73e188101b7f8b2b` | 10 pane/script/preview checks; 3 additional relay-consent checks |

Individual runs overlap; their counts should not be added. All listed passing runs had zero failures or skips. The artifact-pill design remains deferred.

## Changes from the refresh

- Header conflicts retain the newer reserved account slot, anchored error feedback, Connections and AgentGrants. The mobile slot remains 44px through loading and readiness. Error feedback now clears the full 56px header; the newer lifecycle tests retain their exact geometry, timeout, retry and reconnect assertions with 44px phone/32px desktop expectations.
- Enlarged composer text exposed a real regression with populated connection notices. At 320×568 and 200% root text, a minimized fixture placed Send’s bottom at 561.3px on main, 623.3px on the original readability branch and 682.3px combined. Both existing htmx regions now share a bounded scroll area. The input, Send, warning content and both connection actions remain reachable; empty notices reserve no space.
- A suspected relay-consent trap was disproved in Chromium. A new actual-controller regression verifies that the nested dialog is visible, focusable and clickable while Preview is inactive, before and after pane initialization; Decline restores composer focus. No consent UI or permission change was needed.

[Notice overflow before](screenshots/Exh-h9m4/composer-notices-before-320-200.png) · [Notice layout after](screenshots/Exh-h9m4/composer-notices-after-320-200.png). These fresh captures show the isolated readability change at 320×568/200%; the combined layout is covered by the final suite.

## Review and validation

Fresh independent Standards and Spec agents reviewed the original refreshed branches, then the notice/relay and account amendments. A separate coordinator reviewed their overlap and preserved prior findings. Standards: **0 remaining important findings**. Spec: **0 remaining important findings**. The enlarged-composer finding is resolved. The previously documented optional Details-link resize-focus follow-up remains outside the pane/tab-strip/divider guarantee.

All final checks passed on `91d97b168256a184952f91a74f3f8f2f1c0929ec`:

- Asset/server build.
- **319 browser/API/script tests and subtests**, zero failures or skips.
- Required browser execution gate, including both new regressions.
- **6 Playwright journeys**.
- **0 new lint issues** against the pinned main base.

The combined Go selection was:

```sh
go test -p 1 -count=1 -json ./internal/api -run '(Browser$|^TestGalleryPageScriptSuite$|^TestAgentPreview|^TestProfile|^TestNewPage|^TestGallery(Space|Header))'
```

Builds/checks ran serially with `GOMAXPROCS=1`, `GOMEMLIMIT=384MiB`, `GOFLAGS=-p=1`, a real Chromium binary and the repository browser workflow. An initial disk-full build and the earlier account failures were retained as failed attempts; they are not counted as final passing evidence.

Earlier story screenshots remain historical isolated captures. Chromium viewport, text and SDK-stub checks do not establish physical iPhone, VoiceOver, live Clerk or actual mobile-keyboard behavior. No merge or deployment was performed.
