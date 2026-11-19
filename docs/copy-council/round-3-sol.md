Reviewed `5ba4f080`, the diff from `d7ac2662`, both prior rounds, and supplied screenshots. Read-only; no browser or project changes.

| Category | Score | Assessment |
|---|---:|---|
| Necessity | **8/10** | Repeated sharing warnings are consolidated; remaining help generally supports decisions. |
| Clarity | **8/10** | Permission audiences and recovery instructions are accurate. A localized widget instruction still needs correction. |
| Hierarchy | **8/10** | Clear section headings, persistent field help, and explanations beside relevant controls. |

**This is Sol’s first qualifying round.** Completion still requires both reviewers to qualify in two consecutive rounds.

| Surface | Evidence and assessment |
|---|---|
| Library/search/empty states | Contextual search/filter messages remain clear. The sharing receipt now distinguishes named-person writers from public-link readers. |
| Add artifact | Persistent description help and concise route instructions remain effective in the phone screenshot. Network dependency limitations remain disclosed. |
| Viewer/edit/agent | Connected-preview explanations no longer prescribe a disabled action; redirected requests retain their separate explanation. Recorded and live agent activity labels match. |
| Share/download/duplicate | Revised phone and desktop screenshots confirm one prominent account-access warning, discoverable sharing submission, and clear data-mode descriptions. Download and duplication disclosures remain intact. |
| Saved-data transfers | Import confirmation explicitly warns of permanent replacement and possible historical copies. Export/import help remains beside its controls. |
| Settings/connected agents | Token scope remains explicit before issuance. Restored connection allowances sit appropriately inside “Connection limits.” |
| Connections | Revised phone screenshot no longer directs users to unavailable setup steps. Existing account-access and saved-results distinctions remain clear. |
| Login/OAuth/errors | SSO failures now provide plain explanations and “Return to sign in”; diagnostics remain in logs. OAuth scope and error-page recovery remain clear. |

All six original P1 findings remain resolved. Round-two objections concerning sharing scope, unavailable recovery/setup instructions, SSO diagnostics, import permanence, repeated warnings, and common-path jargon are resolved.

Remaining findings:

1. **P2, localized — Widget help promises behavior that saving rejects.**  
   [edit.tmpl:232](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/edit.tmpl:232) says “Leave it empty to use the default.” When a custom widget exists, [edit.js:443](/home/maxomdal/Exhibit-Exh-iust/web/gallery/edit.js:443) rejects an empty source instead. Replace the help with:  
   **“Enter a complete HTML document for the card widget. To replace a custom widget with the default, choose Remove.”**

2. **P3 — Two widget messages retain “tile.”**  
   [edit.js:445](/home/maxomdal/Exhibit-Exh-iust/web/gallery/edit.js:445):  
   **“Widget HTML cannot be empty. Choose Remove to use the default widget.”**  
   [edit.js:480](/home/maxomdal/Exhibit-Exh-iust/web/gallery/edit.js:480):  
   **“Remove this artifact’s custom widget? Its library card will use the default widget.”**

3. **P3 — Import failure exposes the internal name “state.”**  
   [storage-import.js:54](/home/maxomdal/Exhibit-Exh-iust/web/gallery/storage-import.js:54): use **“Could not import saved data: {message}”** for data imports and **“Could not import assets: {message}”** for asset imports.

4. **P3 — Switching help retains “personal grant.”**  
   [partials.tmpl:396](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/partials.tmpl:396): replace the opening with **“Switch to shared data: people you add use your saved data.”** Keep the explanation that their separate data is retained.

**Acceptance blockers: none identified.** The remaining issues are localized and recoverable; they warrant cleanup but do not keep these three scores below 8. Test-suite completion remains with the parent; this review does not claim it passed.
