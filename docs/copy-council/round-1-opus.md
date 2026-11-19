# Exhibit interface copy review, round 1 (Claude Opus, source-only)

I reviewed the source in `/home/maxomdal/Exhibit-Exh-iust` without opening a browser. I covered the templates, the Go view-model strings and the user-facing strings in `web/gallery/*.js`. Paths below are relative to that worktree.

## Scores

| Category | Score | Main reasons |
|---|---|---|
| Necessity | **5/10** | Three explanation-heavy areas: permission dialogs and capability banners, connected-account warnings repeated across many surfaces, and edit-page panel intros. Several lines just repeat the button or the heading next to them. |
| Clarity | **5/10** | One copy error: the refetch confirm says the opposite of what the product does. Core nouns are inconsistent: gallery/library, artifact/tool, state/saved data, widget/tile, security/allowlist settings. Jargon appears in end-user copy (origin, allowlist, sandbox posture, `localStorage`, snapshot). |
| Hierarchy | **6/10** | Most help text does sit next to its control. Problems: the edit Security panel has no static heading, only a computed run-on summary; Title Case headings sit beside sentence-case ones; on the Add page, a key safety note is in the footer while the toggle carries a 25-word label; the delete-account copy is split across two blocks. |

## Acceptance blockers (fix before any score can reach 8)

1. **The refetch confirmation contradicts the product.** `web/gallery/detail.js:982-985` says "It is NOT versioned and cannot be undone." In fact refetch records a version (`internal/api/artifacts.go:1053`, `store.VersionRefetch`) and the previous page is one restore away (PRD §8.1). It also says "saved state/data may break". Rewrite:
   > "Update from the source URL?\n\n{URL}\n\nExhibit fetches the page again and saves it as a new version, with its images, styles and scripts. The current version stays in Versions, so you can restore it. Network approvals don't change; any new sites will ask before they load. Saved data is kept and may not fit the new version."
2. **Library naming is inconsistent.** The same place is called "Gallery" (`appheader.go:40,58`; the sr-only h1 at `gallery.tmpl:51`; `notfound.tmpl:44` "Back to the gallery"), "Library" (`galleryspaces.go:17`) and "My library" (`editnavigation.go:30,45`; `components.tmpl:47`). Use **"Library"** for the space and **"Back to library"** for back links. Use "My library" nowhere, or everywhere.
3. **The product noun switches between "artifact" and "tool".** Examples: "This tool's saved data changed" (`detail.tmpl:115`) next to "This artifact uses unsupported…" (`detail.tmpl:54`), and "This tool tried to reach…" (`detail.js:929`). Pick one user-facing noun. "Artifact" already appears in the headers, buttons and Add page, so I recommend it. Apply it across the detail page, share panel, connections and the direct-open dialog.
4. **Saved data has three names.** The edit page uses "Stored state" (`edit.tmpl:125`, `state.js:476`), the share panel uses "Saved Data" (`partials.tmpl:367`), and elsewhere it is "saved data" or "your saved artifact data". The status line uses "state" too, e.g. "Erasing all state" (`agent.js:618`). Use **"Saved data"** everywhere, including button labels: "Save data", "Export data (JSON)", "Erase saved data".

## Findings by surface (in priority order within each)

### Library, search and empty states
- `sharedgallery.go:25` "Nothing shared yet. Share an artifact from its share panel." → "You haven't shared anything yet. Open an artifact and choose Share." (There is no "share panel" label anywhere in the UI.)
- `searchgallery.go:13` "Search your library and artifacts shared with you." is fine as an empty state. Keep it.
- `gallerygrid.go:34` uses the welcome text "Welcome to Exhibit" as the h2, which tells a new user nothing → heading "Your library is empty", CTA "Create your first artifact" (keep).
- `gallery.tmpl:19` "Couldn’t switch spaces. Try again." In this sentence "spaces" is internal jargon → "Couldn’t open that page. Try again."
- `gallerygrid.tmpl:8` (search field) has placeholder "Search" with aria-label "Search artifacts". Fine. Search covers title, text and tags, so you could use the placeholder "Search titles, text, tags".
- `gallerygrid.tmpl:46` and `:13`: there are two different "Couldn’t load artifacts" patterns, one with an inline button and one with the button below. Make them the same.
- Capability popover (`partials.tmpl:248`): the header "Sandbox posture" → "Permissions". "Fully contained — no network, download, clipboard, external link, camera, microphone, or location access" → "No permissions. Can’t use the network, files, clipboard, links, camera, microphone or location." "Manage security settings →" → "Edit permissions" (it must match the edit panel heading; see the Edit section).
- `partials.tmpl:185` aria-label "Sandbox posture and sharing" → "Permissions and sharing".
- `gallery.go:921` "everyone using it reads and writes one shared copy of its data" → "Everyone uses one shared copy of its data." `gallery.go:926` "…read the owner's saved data" → "…see your saved data" (the reader is the owner).

### Add artifact (`new.tmpl`)
- `:32-33` "Three ways in." / "However it arrives, it lands in your library as one self-contained file you own." The heading counts the tiles directly below it. → heading "Add an artifact" (it also matches the button and the page title), and remove the paragraph or cut it to "Every artifact is saved as one file you own."
- `:53` "Fetch a page once and keep your own copy. Assets inlined." → "Save a copy of a web page, including its images and scripts." ("Assets inlined" is jargon and partly duplicates `:111`.)
- `:111` 25-word checkbox label → label "Save images, styles and scripts into the file (recommended)", help text "Keeps the artifact working if the original site changes."
- `:124` "Network origins are surfaced for approval before anything runs." This sits in the footer, far from the scan result → move it beside `#network-approval` as "You’ll choose which websites it can contact before it runs." Or drop it, because the approval UI explains itself.
- `:98` placeholder "Paste HTML artifact source here…" → "Paste HTML…" (the label already says Source).
- `new.js:145,155,163` "Error: …" prefixes, and "(err.message)" exposed to the user → "Couldn’t reach the server. Check your connection and try again." / "Couldn’t add the artifact: {message}".
- `new.js:127` "Fetching page and snapshotting assets…" → "Fetching the page and its files…". `new.js:213,219` "Snapshot failed/Snapshot report" → "Couldn’t save the page’s files" / "What was saved".

### Network approval (`network.tmpl`, `editorigins.tmpl`, `partials.tmpl:421-434`)
- `network.tmpl:8` "This artifact wants to contact these origins." `:9` "Origins stay blocked until you allow them. This static scan may miss addresses built while the artifact runs." → "This artifact may contact these sites. They stay blocked until you allow them." Drop the static-scan sentence, or shorten it to "It may try others later; you’ll be asked then."
- `network.tmpl:12` "Approve selected & enable" ("enable" what?) → "Allow selected". `:13` "Keep remaining origins blocked" → "Block all" (or "Skip for now"). `:16` "Continue to artifact" is OK.
- `network.tmpl:2` "Review this artifact's requests" is fine.
- `editorigins.tmpl:4,12,24` "Network allowlist" / "Referenced, not approved" / "Blocked" → "Allowed sites" / "Found in the code, not allowed" / "Blocked". Also remove the redundant "blocked" tag at `:28` under the "Blocked" heading.
- `partials.tmpl:429` "…You can revoke it later in allowlist settings." There is no screen called "allowlist settings" → "You can remove it later under Edit → Permissions." The same fix applies to `detail.js:303` ("add it in allowlist settings").
- `partials.tmpl:434` "Allow origin" → "Allow site". Keep "Block once" / "Don't ask again" (the security meaning is preserved).

### Viewer and detail (`detail.tmpl`, `components.tmpl`, `detail.js`)
- **The open-outside-the-preview action has four labels:** "Open in new tab" (`components.tmpl:92`, `detail.tmpl:62`), "Open it directly" (`detail.tmpl:54`), "Open directly" plus the heading "Open outside the preview sandbox?" (`direct_open.tmpl:3,10`), and "Open" (`agent.tmpl:183`). → Use "Open in new tab" everywhere. Dialog heading "Open in a new tab?", confirm button "Open in new tab".
- `detail.tmpl:54` → "This artifact needs a feature the preview can’t provide. Open it in a new tab to use it."
- `detail.js:252-330` CAPABILITY_COPY: each detail runs 40–60 words of origin theory. → Cut each to one sentence, e.g. camera: "The preview can’t use your camera. Open in a new tab, where your approval applies." module-worker: "The preview can’t run this artifact’s background worker. Open in a new tab to run it."
- `detail.tmpl:94` → "Only the owner can allow new sites for this artifact. Ask them to allow it if you need it."
- `detail.tmpl:115` "…It is running with what it loaded." → "This artifact’s saved data changed on another device. Reload to see the latest." `:117` "Reload the tool" → "Reload".
- Permission dialogs `detail.tmpl:146,159,171,195,216` all end "…revoke this at any time from its security settings" → "You can change this later under Edit → Permissions."
- `:171` The link dialog asks nothing about the artifact and runs on → "Open a link to {host}? Exhibit can’t check external sites. Allowing lets this artifact open links from now on." Keep the trust warning.
- `:195`/`:216` (media and location) → "The preview can’t use your {camera}. Allowing opens this artifact in a new tab, where your browser will ask too. You can change this later under Edit → Permissions."
- `components.tmpl:114` The tooltip "Downloads this tool and its assets, without its saved data or connected accounts" is hidden in a title attribute, but the fact matters → show it as menu subtext "HTML file, without saved data", or keep it as is and fix "tool".
- `components.tmpl:115` "Update from source" is fine. The status lines at `detail.js:989-1017` use "✓"/"✗ Error:" prefixes → plain sentences: "Updated.", "Already up to date.", "Couldn’t update: {message}".
- `components.tmpl:49` The aria-label "Past the left or right edge puts them away." is confusing → "Move controls. Drag, or use the arrow keys."
- `components.tmpl:109` "Modify with agent" vs "Build with agent" (`new.tmpl`) vs "Edit with agent". Pick "Edit with agent".

### Edit page (`edit.tmpl`, `edit.js`, `state.js`, versions)
- **Hierarchy:** the Security panel summary `edit.tmpl:51` is just the computed string from `edit.js:192-199` ("3 origins · downloads: ask first · clipboard: ask first · …", 7 clauses). → Static heading "Permissions", with a short summary such as "3 sites · 2 allowed features".
- `edit.tmpl:22` "Editing artifact" eyebrow above the title repeats the page title "Edit: …". Remove it.
- `edit.tmpl:27-28` label "Artifact title" with placeholder "Title". Remove the placeholder.
- `edit.tmpl:128-130` "Data this artifact saved through `localStorage`… never as raw text." → "Data this artifact has saved. It syncs across your devices."
- `edit.tmpl:154,156` (export and import help) → Keep the warning about connected-account results, but tighten it: "Export downloads all saved data, including results from connected accounts (never credentials). Unsaved edits aren’t included." / "Import replaces all saved data with the file’s contents." Also, `storage-import.js:33-37` restates this, and adds "Continue only if you understand…", which is patronizing → "Replace all saved data with this file? This can’t be undone."
- `edit.tmpl:169-176` (assets intro, 4 sentences) → "Files this artifact loads while running, saved so it keeps working if the source site changes. Deleting one can break the artifact." `edit.tmpl:186-187` → "Download all assets as a ZIP." / "Replace all assets with a ZIP exported from this artifact."
- `edit.tmpl:217` "Gallery widget — custom tile/default tile", `:224`, and `partials.tmpl:326` "Embed tile": decide on **widget** or **tile**. Recommendation: "Card widget"; the embed becomes "Embed widget".
- `edit.tmpl:253-255` → "Every change to the source or widget is saved as a version, with the data it had. Restoring adds a new version, so nothing is lost."
- `edit.js:395` "Are you sure you want to delete this artifact? The action cannot be reversed and all data will be lost." → "Delete “{title}”? Its versions, saved data and share links are deleted. This can’t be undone." (This names what goes and keeps the irreversibility.)
- `edit.js:308,319` "Saved. Review network access below before leaving." is good. Keep it.
- `state.js:371` vs `:597`: one says "property", the other "key" → "Name the new key." in both.
- `editnavigation.tmpl:3` "…Changes already saved stay saved." → Remove the second sentence.

### Agent (`agent.tmpl`, `agent.js`)
- `agent.tmpl:29` "Agent support is disabled: the `pi` binary was not found on this server." This is operator diagnostics shown to end users → "The agent isn’t available on this server."
- `agent.tmpl:65` placeholder "Ask Exhibit..." → "Describe a change…" (and use … rather than ...).
- `agent.tmpl:69` The hint line packs in three shortcuts, and the Snippet button repeats one of them (`:186`) → Remove the Ctrl+Shift+S clause from the hint.
- `agent.tmpl:62` "Preview updated — tap Preview to view" → "Preview updated. View".
- `agent.tmpl:97` "…this page never sees it again." → "Your key is encrypted on the server and never shown again." The same idea repeats at `agent.js:238`. Keep one of the two.
- `agent.tmpl:216` → "No earlier conversations about this artifact."
- `agent.tmpl:256` "This conversation was kept before conversations could be continued…" → "This older conversation can be read but not continued."
- `agent.tmpl:268,270` The rollback explanations are good. Keep them.
- `agent.js:571,574,589` "Prepared artifact source." / "Extension error: …" / "Provider hiccup — retrying" are internal → remove the first; "Something went wrong in the agent. Try again."; "The AI provider didn’t respond. Retrying ({n}/{max})…"
- `agent.js:551` → "Widget saved. It now appears on this artifact’s card."

### Share and duplicate (`partials.tmpl:270-395`, `duplicate.tmpl`)
- **Case:** "Share Settings", "Link Access", "Saved Data for People with Access", "Separate Data per Person", "Shared Data for Everyone", "What Happens When I Switch?" are Title Case, while the rest of the app uses sentence case. Use sentence case. Heading `:272` → "Share".
- `:306`+`:308`: the toggle "Anyone with the link can open this artifact" plus 4 sentences → keep the toggle label. Help text: "Visitors see your saved data, including results from connected accounts. They can’t change it or use your accounts."
- The connected-account custody message appears at `:308`, `:340`, `:389`, connections `:8`, `:14`, `:36`, `:37`, `:72` and `network.tmpl:24`, about 9 times. Keep it once per surface: in the share panel only under People (`:340`), and remove it from `:389`.
- `:319` "The current link stops working." beside "Replace link" is good (it preserves the warning).
- `:369` legend "How people share saved app data" → "Saved data for people you add". `:375`/`:383` → "Separate data for each person" / "One shared copy for everyone".
- `:394` duplicates `:308` → remove it.
- `:362` "Nobody else has been given access." → "Not shared with anyone yet."
- `duplicate.tmpl:9` "Give the same people access and create a new public link if enabled." → "Shares with the same people, and creates a new public link if this one has one." `:10` "Copy data / Include your saved artifact data." → "Copy saved data".
- `duplicate.tmpl` heading "Duplicate artifact" and the submit "Duplicate artifact" are fine; the menu item "Duplicate" is consistent.

### Settings and connected agents (`profile.tmpl`)
- `:23-24` "Settings." (with a full stop) plus "Manage your storage, connected agents and Exhibit account." → "Settings"; remove the sub-line.
- `:41` "…subject identifier it does send. It is the string an administrator would use to find you." → "Your identity provider didn’t share an email, so this is your account ID."
- `:48` → "Your password is managed by your identity provider."
- `:55` "…guarded by the service token alone." is operator-facing → "This server has no sign-in, so there’s no account to show."
- `:71` "Total storage used by…" repeats the "Storage used" row → "Includes files, widgets, versions and assets."
- `:81-84` The connections Advanced disclosure shows counts ("Service accounts / Managed accounts") that users can’t act on. Remove it from Settings; `/connections` already links to Advanced.
- Delete account `:139-179`: two blocks state "cannot be undone" twice and there are 4 variants of the "new user" line → first block: "Permanently deletes your account, library, saved data and share links."; the confirm step keeps one "This can’t be undone." plus the counts. Keep the meaning of the share-breakage and provider-unaffected lines.
- `:203` and `:214-215`: good. `:222` "For clients without browser sign-in" → "Use a token instead". `:233` is good.

### Connections (`connections.tmpl`)
- `:8` "Your account access always stays personal." This is also stated at `:14`, `:36` and `:37` → keep `:14` only.
- `:47-50` "Relay permissions" / "Allow relay requests" / "sent from this server's address" → "Server requests" / "Let this artifact call APIs through Exhibit". Keep the read/change warning.
- `:53` "Fetch an approved API into a localStorage key while the artifact is closed." → "Update saved data from an API on a schedule, even when the artifact is closed." `:55` label "Save to localStorage key" → "Saved data key".
- `:67-69` "Saved results are separate": 3 paragraphs that restate the share panel, and they use "Shared data"/"Own data", which don’t match the share panel’s mode names → shorten and use the share panel’s exact mode names.
- `:58` "Take the app with you." / "Download a…" is fine. `:61` "Open a tool’s Connections menu" refers to a menu that doesn’t exist in the pill (`components.tmpl` has no Connections item) → check this, or reword to "Open an artifact and choose Connections."
- `connections-controller.js:164` → "Clear this artifact’s saved data, including older versions’ data? Connections and permissions aren’t changed."

### Sign-in, OAuth and errors
- `login.tmpl:58` "Continue with your identity provider" → "Sign in with SSO" (or the provider name).
- `clerk.tmpl:32` "Sign out to switch accounts" is fine. `clerk.tmpl:26` "Your library, wherever you work." is OK.
- `oauth.tmpl:20-22` is good: it is accurate and keeps the security meaning. Make one small change: `:21` "It cannot share, delete, or change saved data." → "It can’t share or delete artifacts, or change saved data." (That is more precise.)
- `notfound.tmpl:32-35`: the museum-label joke ("Deaccessioned…") is fine as brand. `:41` "or the link arrived slightly bent" → "or the link is wrong." `:44` → "Back to library".
- `gallery.go:459` "Artifact not found. It may have been deleted." is good.

## What I'd check in the round-2 screenshots
- Whether the edit Security summary wraps, and the detail pill's banner stack on a phone.
- Share panel length on a phone, since it carries the repeated custody copy.
- The Add page footer note placement.

**Bottom line:** the copy is accurate and safety-conscious almost everywhere. The refetch confirmation is the one copy error. What keeps the scores at 5–6 is repetition (the account-custody message in roughly nine places, and the "revoke from security settings" pointer to a screen that doesn't exist), the four interchangeable noun pairs, and implementation vocabulary (origin, allowlist, `localStorage`, snapshot, posture, binary) on end-user surfaces.
