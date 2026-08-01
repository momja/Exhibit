---
id: av-hg5f
status: open
deps: []
links: []
created: 2026-08-01T16:50:57Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-p4hm
tags: [ui, state, api]
---
# State inspector on the edit page: typed form editing of artifact state

The edit page should show the artifact's server-persisted state (what the artifact wrote through localStorage/sessionStorage) as an inspectable, editable panel — a corrective tool for when a user needs fine-grained control of their own data, e.g. an artifact wedged itself on a bad value, or a key needs seeding by hand.

State is a flat key/value map (artifact_state, one row per (artifact, key), values are strings). Artifacts almost always store JSON, so the panel INFERS a type per value and renders the matching form control rather than exposing raw text: the user edits a list as a list and a number as a number. Raw-text/JSON editing of a value is deliberately NOT offered — a hand-typed blob is exactly the corruption vector this tool exists to undo.

Three actions: Save (write the pending edits), Cancel (discard them), and Erase all data (critical, warning-styled, confirmed) which drops every state row for the artifact.

Later the agent sidecar will read and edit state through the same API surface, so the routes this adds are the contract for that (av-lvi1 — follow-up, not in scope here).

## Design

**Type inference (display + control choice).** Per value: JSON.parse the string; on failure treat it as a plain string. Then dispatch on the parsed shape —
- string / number / boolean -> text input, number input, checkbox/toggle
- array of primitives -> ordered list editor: one row per item, add / remove / reorder
- array of objects (uniform keys) -> table-ish repeater, one labelled field per key
- object -> labelled field per property, recursing to a bounded depth
- anything deeper/ragged than the form model handles -> render READ-ONLY (pretty-printed, non-editable) with delete still available. Never fall back to a free-text JSON editor.
Re-serialize on save the way it was parsed (JSON.stringify for parsed values, verbatim for plain strings) so a round-trip with no edits is a no-op.

**Rows.** Each key gets: key name, inferred type badge, the control, and a per-key delete. Add-key flow asks for key + type up front, then renders that type's empty control.

**API (new — the single write path).** The state surface today is GET /api/artifacts/:id/state and PUT (one key at a time, from the storage bridge). This needs:
- DELETE /api/artifacts/:id/state/:key — remove one row
- DELETE /api/artifacts/:id/state — erase all rows for the artifact
and matching Store methods (DeleteState / ClearState) beside GetState/SetState. Save can reuse the existing per-key PUT; batch it only if the request count is visibly bad.

**Page wiring.** A collapsible panel on edit.tmpl beside the existing security panel, following the established pattern: server-rendered markup + the page's static asset JS in web/gallery/edit.{css,js}, per-request values (artifact id, token) via the inline bootstrap. Phosphor icons. State is fetched client-side on panel open rather than inlined, since it is cold data the page does not otherwise need.

**Semantics note (do not silently change).** The shim's removeItem currently write-throughs an EMPTY STRING rather than deleting the row, so a removed key reads back as '' instead of null. The edit page's delete should genuinely drop the row (the correct semantic). Reconciling the shim’s removeItem with row deletion is av-ms3r, not this ticket; the clear() write-through is av-st7c. Both consume the DELETE routes this ticket introduces.

**Erase all** is destructive and irreversible (no version history for state): warning-styled button, explicit confirm naming the artifact, and it must not touch the artifact body or allowlist.

## Acceptance Criteria

1. The edit page shows every stored state key for the artifact with its value rendered through a type-appropriate control (list as list, number as number, boolean as toggle, object as labelled fields).
2. No control anywhere in the panel accepts a raw JSON/plain-text blob as the way to edit a value.
3. A value too complex for the form model renders read-only and can still be deleted.
4. Editing values + Save persists through the API and is visible to the artifact on next render (state is inlined at render time).
5. Cancel discards pending edits with nothing written.
6. A new key can be added by choosing key + type and filling the rendered control.
7. A single key can be deleted, and its row is actually removed (not blanked).
8. Erase all data is warning-styled, requires explicit confirmation, removes every state row for that artifact, and leaves body/allowlist/capabilities untouched.
9. DELETE routes for one key and for all state exist on the API and are covered by tests; the panel is covered by an api-level render test.
10. An artifact with no stored state shows an explicit empty state, not a broken panel.

