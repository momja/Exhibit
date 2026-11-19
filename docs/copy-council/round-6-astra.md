**Astra, round 6: Necessity 8/10 · Clarity 8/10 · Hierarchy 8/10.**

Reviewed `b2f8def2`, prior findings, the change inventory, current source, and the requested actual-app screenshots via `view_image`. No edits, tests, Git mutations, or live browsing.

Evidence by surface:

- **Library/search/creation:** contextual empty states give useful next steps; Library naming is consistent. Phone/desktop creation screenshots show persistent description help and a clear primary action.
- **Viewer/editor/agent:** permission scope and refetch restoration are explicit. Data transfer help sits beside its controls; current-data erasure distinguishes historical copies. Final widget screenshots correctly direct users to **Remove**. Recorded/live activity labels agree.
- **Share/duplicate/download:** sharing distinguishes invited writers from public readers. Screenshots confirm discoverable **Give access** and consolidated disclosures. Duplicate and download descriptions explain saved-data inclusion/exclusion.
- **Settings/connections:** token scope precedes issuance; connection limits remain discoverable. The revised general Connections empty state removes unavailable setup instructions. One artifact-specific variant remains below.
- **Login/OAuth/errors:** OAuth scope is precise; SSO failures provide recovery without raw diagnostics. The 404 and protected-render fallback offer clear return actions.

Two substantive, localized findings remain:

1. **Unavailable setup instruction in artifact-specific Connections.**  
   [connections.tmpl:43](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/connections.tmpl:43) says “Connect your own account first…” whenever there are no connections, even when no setup providers exist. The introduction at line 36 also instructs choosing an account when none exists. Condition both on availability; otherwise say **“No accounts are available for this artifact.”** Keep the existing service-availability explanation in the setup section.

2. **Redirected-request recovery assumes owner permissions.**  
   [detail.js:272](/home/maxomdal/Exhibit-Exh-iust/web/gallery/detail.js:272) tells every viewer to add the destination under **Edit → Permissions**, including shared recipients. For recipients, use **“Only the owner can allow this destination. Ask them to review this request.”** Preserve the explanation that opening another tab does not bypass permissions.

These are real recovery-copy defects, not word preferences, but sufficiently localized that **all three categories still reach 8**. Optional polish: remove the decorative “NOT ON VIEW” caption from [notfound.tmpl:32](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/notfound.tmpl:32); it adds little beside “Page not found.”

**No acceptance-blocking copy issue identified.** This supplies Astra’s qualifying review only. If Sol also scores ≥8 throughout this round, count **one qualifying joint round** for Sol + Astra; another consecutive independent joint passing round is required.
