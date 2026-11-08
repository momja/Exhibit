# Round 2 copy review (Claude Opus) of revision `d7ac2662`

**Evidence used:** the diff `a6682df4..d7ac2662` on `internal/api`, `internal/render` and `web/gallery`, and targeted greps of that revision for remaining stray terms. I looked at the round-2 PNGs for new-phone, share-phone, edit-phone, data-phone, token-phone and connections-phone. I did not open a browser.

## Scores

| Category | R1 | **R2** | Main reasons |
|---|---|---|---|
| Necessity | 5 | **7** | The long text is much shorter: permission dialogs, device notes, the editor intros, versions, 404 and Settings. Some repetition is left in the share modal and on the Connections empty state. |
| Clarity | 5 | **7** | All six P1s are fixed accurately, and Library and Saved data are now used consistently. "tool", "origin", "allowlist" and "Error:" still appear in visible JS status and banner text, including on common paths. |
| Hierarchy | 6 | **8** | Permissions now has a fixed heading with a short summary, and transfer controls sit beside their help. The share form has a label, help text and a submit button. Token scope comes before Issue token, and Add page help is no longer in placeholders. |

**Result: not all scores are 8 or higher, so this round does not qualify.**

## Prior P1s: all resolved
1. **Editor erase:** the button reads "Erase current data". The confirmation says older versions and open pages may keep copies (`state.js`), which matches the store's behaviour.
2. **Connections clear:** the confirmation now says "Permanently… cannot be undone". It separates the owner case (saved versions are cleared too) from the recipient case ("your saved data") (`connections-controller.js`). The scope is correct.
3. **Link consent:** discloses "open external links from now on, including links to other sites" (`detail.tmpl`).
4. **Token scope:** the full library-wide scope appears before the button, and the dialog adds "keep it private". Confirmed in token-phone.
5. **Sharing submit:** the form has the "Add people" label, help text and a "Give access" button. Confirmed in share-phone.
6. **Data modes:** the cards read "Separate data" / "Shared data", and the Connections page uses the same names. The note that public links show the owner's data now sits in the link section.

The refetch confirmation was also fixed: it now says a new version is created and the current one can be restored.

## Remaining findings (in priority order)

**Clarity: stray terms on visible paths (P2)**
- `web/gallery/detail.js:891-893` is the banner recipients see: "This tool tried to reach {origin}. Its owner has not allowed that." / "…{n} origins its owner has not allowed." Change to: **"This artifact tried to reach {site}. The owner hasn't allowed it."** / **"This artifact tried to reach {n} sites the owner hasn't allowed."**
- `web/gallery/edit.js:467` and `:576` read "✓ Saved — but X is not on the allowlist and will be blocked." Change to **"Saved. {X} isn't allowed yet and will be blocked. Review it under Permissions."** For the second one, use "Widget generated." as the opening.
- `web/gallery/edit.js:369` and `web/gallery/new.js:262` read "✗ Failed to update allowlist". Change to **"Couldn't update allowed sites."**
- `web/gallery/edit.js:340` reads "Saved, but could not forget {origin}", but the button is now called "Ask again". Change to **"Saved, but couldn't reset the block on {site}."**
- `web/gallery/detail.js:957-981` (refetch status) still uses "✗ Error: …" and "✓ Updated — reloading…". Change to **"Couldn't update: {msg}"**, "Already up to date.", **"Updated. Reloading…"**. This was flagged in round 1 and is not yet fixed.
- `internal/api/templates/connections.tmpl:28` reads "Your tools keep their existing permissions." Change "tools" to **"artifacts"**. Line `:93` reads "…what tools can read or change." Change to **"…what artifacts can read or change."**
- `internal/api/templates/edit.tmpl:65`: the subheading is "Capabilities", but the summary right above it says "0 features allowed". Rename the subheading to **"Features"** so the two match.

**Meaning preserved, but a warning was lost (P2)**
- `web/gallery/storage-import.js`: the replace confirmation is now "Replace this artifact's current saved data with the file's contents?" It no longer says that the current data is lost. Import creates no version, so the current data is gone. Add: **"The current data is permanently replaced; older versions keep only their own copies."** For assets, add **"This can't be undone."** unless assets are restorable through versions.

**Necessity: repetition (P2)**
- In the share modal, the "can't use your accounts" point appears three times: in the public-link paragraph, in the People intro ("Sharing this artifact never lets someone use your connected accounts…"), and in the shared-data help. In share-phone, the People intro pushes the form below the fold. Remove the People intro (`partials.tmpl`, share-help above "Add people"). The public-link sentence and the shared-data note already cover it.
- On the Connections empty page (connections-phone), "choose which artifacts can use your accounts" appears in the lede and again in the empty state. When the instance has no services, the empty state still says "Connect an available service". When no services are configured, change the empty-state body to **"No accounts connected."** (or drop it), and keep only one of the lede and the "Open an artifact and choose Connections…" line.
- `partials.tmpl` Shared data help: "Shared data includes results saved from connected accounts. Review it before sharing." This repeats the link paragraph above it. It is acceptable, but can go if the People intro stays.

**Low (P3)**
- `state.js:342-383`: "property" for nested fields and "key" for top-level entries. The distinction is defensible; keep both, but don't call a top-level entry a property anywhere.
- The share modal "Add people" label is styled the same as the body text below it (share-phone). Use the same subheading style as "People" or "Public link", or make it a field label.
- The share link label "Anyone with the link can open this artifact" works well with the paragraph below it. No change needed.

## Acceptance blockers
- None of the round-1 P1 blockers remain.
- Clarity and Necessity are both at 7. To reach 8: clear the stray "tool"/"origin"/"allowlist"/"Error:" strings in visible JS (detail.js recipient banner, edit.js save warnings, refetch status), restore the permanence warning in the import confirmation, and remove the repeated account-custody text in the share modal and on the Connections empty state.
- When all three reach 8, that counts as one qualifying round only. A second independent round is still required.
