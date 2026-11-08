**Astra, round 7 — Necessity 8/10 · Clarity 8/10 · Hierarchy 8/10.**

Reviewed UI revision `80c48ae6`, the `aa5a4ce8..80c48ae6` diff, prior reports, current source, and saved actual-app screenshots via `view_image`. No edits, tests, Git mutations, subagents, or live browsing.

All round 6 findings are resolved:

- **Connections:** [conditional instructions](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/connections.tmpl:36) now distinguish existing accounts, available setup, and unavailable accounts. New phone/desktop screenshots confirm the unavailable state.
- **Recipient recovery:** [redirect guidance](/home/maxomdal/Exhibit-Exh-iust/web/gallery/detail.js:274) asks the owner to review access, omits unavailable editor instructions, and preserves the no-bypass warning. The fixture screenshot confirms its presentation.
- **Creation:** [saved status](/home/maxomdal/Exhibit-Exh-iust/web/gallery/new.js:175) no longer points incorrectly “below.”
- **Agent:** [recorded transcripts](/home/maxomdal/Exhibit-Exh-iust/internal/agent/transcript.go:125) omit preparation noise; restoration feedback consistently says “restore.”
- **Terminology/errors:** Connections uses “artifact”; the 404 screenshot confirms removal of the decorative caption.

Across unchanged surfaces, Library/search guidance remains concise; creation and widget instructions are persistent; Permissions and data-transfer help accompany their controls. Sharing distinguishes named writers from public readers and exposes **Give access**. Duplicate/download disclosures, irreversible-action warnings, token scope, OAuth consent, SSO recovery, and render fallbacks retain their material meaning.

One **nonblocking hierarchy improvement** remains: the recipient redirect screenshot prominently offers **Open in new tab**, although the expanded explanation correctly says it cannot bypass permissions. Consider hiding that action specifically for `redirected-origin` in [the capability banner](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/detail.tmpl:54). This is a competing action cue, not an inaccurate permission disclosure.

**No substantive regressions or acceptance blockers found.** Scores remain 8 because some warning panels are dense and secondary actions still compete with recovery guidance; the interface nevertheless meets all three thresholds.

This supplies Astra’s second consecutive qualifying review. Round 7 completes the required **second joint passing round only if Sol also scores at least 8 in every category**.
