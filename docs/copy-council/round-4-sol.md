Reviewed `0a2cb08e` against `5ba4f080`, prior findings, current source, and supplied screenshots. Read-only; no tests run.

| Category | Score | Evidence |
|---|---:|---|
| Necessity | **8/10** | Help supports decisions; sharing warnings remain consolidated. |
| Clarity | **8/10** | All four round-three findings are resolved; instructions match available actions. |
| Hierarchy | **8/10** | Persistent help accompanies fields; permissions and irreversible-action warnings remain beside relevant controls. |

Confirmed fixes:

- [edit.tmpl:232](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/edit.tmpl:232) directs users to **Remove** to restore the default widget, matching validation.
- [edit.js:445](/home/maxomdal/Exhibit-Exh-iust/web/gallery/edit.js:445) and [edit.js:480](/home/maxomdal/Exhibit-Exh-iust/web/gallery/edit.js:480) consistently say **widget** and explain removal’s result.
- [storage-import.js:54](/home/maxomdal/Exhibit-Exh-iust/web/gallery/storage-import.js:54) names **saved data/assets** instead of internal “state.”
- [partials.tmpl:396](/home/maxomdal/Exhibit-Exh-iust/internal/api/templates/partials.tmpl:396) uses **people you add** while preserving the explanation about retained separate data.

No regressions identified. Sharing still distinguishes public readers from invited writers and saved results from account access. Import/erase permanence, historical-copy caveats, token scope, connection limits, and SSO recovery remain intact. Phone and desktop screenshots support the previous hierarchy assessment.

**Remaining copy findings and acceptance blockers: none identified by Sol.**

Sol now scores at least 8 in all three categories for two consecutive reviews. **Combined council acceptance remains pending:** Opus supplied no round-three score, and this review supplies no Opus approval.
