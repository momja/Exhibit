Reviewed `d7ac2662`, both round-one reports, the requested diff, and supplied screenshots—including all five required phone views. Read-only; no browser session or project changes.

| Category | Score | Assessment |
|---|---:|---|
| Necessity | **8/10** | Substantially less repetition; remaining help generally supports decisions. |
| Clarity | **7/10** | One sharing summary still overstates write access; some recovery instructions remain misleading or absent. |
| Hierarchy | **8/10** | Recognizable headings and help beside relevant controls, verified on phone and desktop. |

**Not all categories reach 8. This is not a qualifying round.**

The major corrections are effective: current-data erasure acknowledges historical copies; Connections clearing explicitly includes historical data and irreversibility; external-link approval discloses enduring access; manual tokens disclose library-wide scope; sharing has a visible **Give access** button; refetch correctly explains version restoration.

| Surface | Evidence and assessment |
|---|---|
| Library/search/empty states | “Library” naming and contextual empty-state actions are improved. Search/retry copy remains clear. The sharing receipt has the scope issue below. |
| Add artifact | Phone screenshot shows persistent description help and a clear primary action. Source removes the self-containment overpromise and explains possible network dependencies. |
| Viewer/edit/agent | Phone and desktop screenshots confirm readable Permissions headings and compact summaries. Agent API-key help is shorter. Connected-preview recovery still has a contradiction. |
| Share/download/duplicate | Sharing submission is discoverable; data-mode labels are clearer. Download exclusions are visible, and duplicate copy acknowledges saved connected-account results. |
| Saved-data transfers | Phone screenshot confirms export/import explanations directly accompany their controls. Erase wording preserves its narrower scope. |
| Settings/connected agents/connections | Token scope appears before issuance and remains readable on phone. Connections’ unavailable-service state still gives an impossible next step. |
| Login/OAuth/errors | OAuth disclosure remains precise. The rendered 404 provides clear recovery. External SSO callback failures remain unresolved. |

Remaining findings, in priority order:

1. **P1 — Sharing receipt still conflates personal-grant writers with public-link readers.**  
   [gallery.go:920](/home/maxomdal/Exhibit-Exh-iust/internal/api/gallery.go:920) says “everyone using it reads and writes” the shared data. When a public link also exists, that includes visitors who cannot save changes. This contradicts the corrected sharing dialog. Replace the grant sentence with:  
   **“{people} can open this artifact and view and update the owner’s saved data.”**  
   At [gallery.go:926](/home/maxomdal/Exhibit-Exh-iust/internal/api/gallery.go:926), use:  
   **“Anyone with its public link can open it and read the owner’s saved data, but cannot save changes.”**

2. **P2 — Connected-preview details still prescribe a disabled action.**  
   The headline correctly handles protected artifacts, but [detail.js:254](/home/maxomdal/Exhibit-Exh-iust/web/gallery/detail.js:254) and the other capability descriptions still instruct users to open a new tab. [detail.js:298](/home/maxomdal/Exhibit-Exh-iust/web/gallery/detail.js:298) inserts these unconditionally, while [detail.tmpl:62](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/detail.tmpl:62) disables that action. For protected artifacts, replace the new-tab instruction with:  
   **“This feature is unavailable while the artifact uses connected accounts or private saved data in Exhibit.”**  
   Preserve the separate redirected-request explanation.

3. **P2 — Browser-facing SSO failures expose diagnostics without recovery.**  
   [auth.go:198](/home/maxomdal/Exhibit-Exh-iust/internal/api/auth.go:198) still displays “login state mismatch”; [auth.go:208](/home/maxomdal/Exhibit-Exh-iust/internal/api/auth.go:208) displays “identity provider returned no code.” [auth.go:464](/home/maxomdal/Exhibit-Exh-iust/internal/api/auth.go:464) returns plain text without a sign-in action. For verification/expiry failures, use:  
   **“Your sign-in attempt expired or could not be verified. Start sign-in again.”**  
   Provide **“Return to sign in.”** Keep diagnostics in logs and preserve account-disabled refusal wording.

4. **P2 — Connections gives unavailable setup instructions.**  
   The phone screenshot says no services are available, then [connections.tmpl:33](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/connections.tmpl:33) instructs users to connect an available service; [connections.tmpl:61](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/connections.tmpl:61) directs them into artifact setup. When there are neither configured accounts nor setup providers, retain the existing availability explanation and **remove these two next-step instructions**.

5. **P3 — Widget terminology remains inconsistent after the heading rename.**  
   [edit.tmpl:220](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/edit.tmpl:220) and [edit.js:435](/home/maxomdal/Exhibit-Exh-iust/web/gallery/edit.js:435) still say “custom tile/default tile.” Replace with **“custom/default.”** At [edit.tmpl:230](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/edit.tmpl:230), move the disappearing placeholder instructions into persistent help: **“Enter a complete HTML document for the card widget. Leave it empty to use the default.”**

6. **P3 — Widget save feedback retains the old permissions vocabulary.**  
   [edit.js:467](/home/maxomdal/Exhibit-Exh-iust/web/gallery/edit.js:467): replace “not on the allowlist” with:  
   **“Saved. Network access to {sites} is blocked. Allow these sites under Permissions.”**

The remaining acceptance blocker is inaccurate audience/write scope in the sharing receipt. The contradictory recovery guidance and SSO failure experience also keep Clarity below 8.
