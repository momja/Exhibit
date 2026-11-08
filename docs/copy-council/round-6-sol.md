**Sol, round 6 — N: 8/10 · C: 8/10 · H: 8/10.**

Reviewed `b2f8def2`, prior findings, and the requested saved app screenshots. Read-only; no tests or live browser.

Evidence by surface:

- **Library/search:** consistent Library naming, contextual empty states, clear retry actions.
- **Creation:** persistent description help; URL imports accurately disclose remaining network dependencies.
- **Viewer/editor/agent:** recognizable Permissions heading; accurate refetch, restore, erase, and lasting-permission explanations.
- **Share/duplicate/download:** named writers and public readers are distinguished; Give access is visible; download exclusions and copied-data consequences are explicit.
- **Settings/connections:** token scope precedes issuance; transfer help accompanies controls; the general Connections empty state is corrected.
- **Login/OAuth/fallbacks:** clear consent scope, SSO recovery, and useful unavailable-link guidance.

Remaining findings:

1. **P2 — Artifact-specific Connections still gives unavailable instructions.**  
   [connections.tmpl:36](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/connections.tmpl:36) says “Choose an account,” and [line 43](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/connections.tmpl:43) says “Connect your own account first,” even when both accounts and setup providers are absent. Gate these instructions on available accounts/setup options; otherwise retain the existing availability explanation. The corrected general-page screenshot does not exercise this branch.

2. **P3 — Creation guidance points in the wrong direction.**  
   [new.js:175](/home/maxomdal/Exhibit-Exh-iust/web/gallery/new.js:175) says “review network access below,” but [new.tmpl:114](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/new.tmpl:114) places the approval region before the status. Use **“Saved. Review network access before continuing.”**

3. **P3 — Internal preparation noise remains in recorded conversations.**  
   [transcript.go:146](/home/maxomdal/Exhibit-Exh-iust/internal/agent/transcript.go:146) still displays **“Prepared artifact source”**, although the live event is suppressed. Omit this projected history entry too.

Minor consistency polish: [Connections notices/actions](/home/maxomdal/Exhibit-Exh-iust/internal/api/connections_page.go:95) retain “tool,” and [agent restoration feedback](/home/maxomdal/Exhibit-Exh-iust/web/gallery/agent.js:955) retains “roll back.” Align them with **artifact** and **restore**. These are not substantive blockers.

**No acceptance blockers identified.** The remaining issues are localized; the principal decisions and consequences are clear.

This is one qualifying Sol review. If Astra also qualifies, round 6 counts as **the first joint passing round** for Sol + Astra; another joint passing round remains required.
