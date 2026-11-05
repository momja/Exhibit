# Interface copy changes

Work: [Exh-iust](../../.tickets/Exh-iust.md), branch `feature/Exh-iust/interface-copy`.
This inventory describes the final changes from baseline `a6682df4`, grouped by where users encounter them. Repeated instances of the same shared copy are grouped together. Review rounds and validation are recorded in [review.md](review.md).

## Navigation and library

Sources: [appheader.go](../../internal/api/appheader.go), [editnavigation.go](../../internal/api/editnavigation.go), [gallery.go](../../internal/api/gallery.go), [gallerygrid.go](../../internal/api/gallerygrid.go), [sharedgallery.go](../../internal/api/sharedgallery.go), [gallery.tmpl](../../internal/api/templates/gallery.tmpl), [components.tmpl](../../internal/api/templates/components.tmpl).

- Standardized the root destination as **Library**, replacing **Gallery** and **My library** in headers, return links, the viewer's accessible navigation label, the accessible library heading, and the editor's fallback destination.
- Library navigation failure: **“Couldn’t open that page. Try again.”** replaces the internal “switch spaces” wording.
- Empty library heading: **“Your library is empty”** replaces “Welcome to Exhibit”; the existing creation action stays beside it.
- Shared-by-me empty state now tells users to **open an artifact and choose Share**, instead of referring to an unnamed “share panel.”
- Sharing receipts now state that named recipients can view and update the owner's shared data. The public-link receipt explicitly adds **“but cannot save changes”**; it no longer says everyone using the artifact can write.
- Used **artifact** consistently in the viewer, sharing, connections, permission notices, settings, and agent preview instead of switching between artifact and tool. Generic examples and user-authored artifact content are unaffected by this terminology pass.

## Add artifact — `/new`

Sources: [new.tmpl](../../internal/api/templates/new.tmpl), [new.js](../../web/gallery/new.js).

- Replaced **“Three ways in.”** with **“Add to your library”** and removed the subtitle promising every import is one self-contained file.
- Route descriptions are now **“Describe what you want to build.”**, **“Save HTML you already have.”**, and **“Save a copy of a web page.”** The selected route's phone description uses the same text.
- Renamed **Description** to **“What should it do?”** The long placeholder became persistent help below the field: **“Describe what you want on screen and what it should remember between visits.”** The placeholder is now a short timer example.
- Renamed **Source** to **HTML source** and shortened its placeholder to **“Paste HTML…”**.
- Shortened the URL checkbox to **“Save images, styles, and scripts with the page (recommended)”**. A nearby second line explains **“Some files may still require network access.”**
- Removed the footer's duplicate network-approval explanation; the actual approval controls explain the decision when it is needed.
- Import progress is now **“Adding artifact…”** or **“Fetching the page and its files…”**, replacing Uploading/snapshotting terminology.
- Empty paste validation now asks for **HTML**, rather than “an artifact.”
- Connection failure now gives an action: **“Couldn’t reach the server. Check your connection and try again.”** Server rejection says **“Couldn’t add the artifact: …”**; an unreadable response says **“Couldn’t read the server’s response. Try again.”**
- URL report headings are **Saved files** and **Saved assets**. “Vendored into the file” became “saved with the artifact.”
- URL-file failure now explains that the original page was saved but its files still need network access, instead of describing a `<base href>` fallback.
- Individual file failures say the files **could not be saved** and still need network access, replacing “inlined” and “origins below.”
- Replaced the absolute “fully self-contained” claim with **“The scan found no remaining external references.”**
- Approval-update failures now say **“Couldn’t update allowed sites.”**
- Post-save network guidance is **“Saved. Review network access before continuing.”**, so it no longer incorrectly points below a review region positioned above it.

## Network permissions — import, editor, runtime prompt, card popover

Sources: [network.tmpl](../../internal/api/templates/network.tmpl), [editorigins.tmpl](../../internal/api/templates/editorigins.tmpl), [edit.tmpl](../../internal/api/templates/edit.tmpl), [partials.tmpl](../../internal/api/templates/partials.tmpl), [edit.js](../../web/gallery/edit.js).

- Network review says the artifact **may contact these sites**. Its help explains that sites stay blocked until allowed and that other sites may be requested later.
- Renamed the review actions **Allow selected sites** and **Continue without allowing**. The latter accurately describes ignoring the checked destinations.
- Replaced **Network allowlist** with **Allowed sites** and **Referenced, not approved** with **Found in source, not allowed**.
- Removed the repeated “blocked” badge from every row under the Blocked heading.
- Renamed **Forget** to **Ask again** and explained that it removes the saved block decision while requests remain blocked until allowed.
- Gave the editor a static **Permissions** heading. Its seven-clause inventory became a short count of sites and allowed features; the individual controls remain inside.
- Renamed the Permissions subheading **Capabilities** to **Features** so it matches the summary.
- The add-site input now has a plain URL example and an accessible label specifying the scheme, host, and optional port.
- Card popover headings and accessible names use **Permissions** / **Permissions and sharing**, replacing “Sandbox posture.” Network counts say **site/sites** instead of origin/origins.
- Renamed the popover settings link **Edit permissions →**.
- Runtime approval explains the lasting site permission, the reload needed to retry, and the actual **Edit → Permissions** revocation path.
- Renamed the lasting runtime decisions **Always block site** and **Allow site**; kept Block once distinct.
- Editor approval errors say **“Couldn’t update allowed sites.”** Resetting a block can report **“Saved, but couldn’t reset the block on …”**, matching Ask again.

## Editor — `/artifacts/{id}/edit`

Sources: [edit.tmpl](../../internal/api/templates/edit.tmpl), [editnavigation.tmpl](../../internal/api/templates/editnavigation.tmpl), [edit.js](../../web/gallery/edit.js), [widget.go](../../internal/api/widget.go), [state.js](../../web/gallery/state.js), [state-export.js](../../web/gallery/state-export.js), [storage-import.js](../../web/gallery/storage-import.js).

- Removed the **Editing artifact** eyebrow and the redundant Title placeholder.
- Empty source validation says **“HTML source cannot be empty.”**
- Delete confirmation names the artifact and explicitly lists **versions, saved data, and share links**, retaining **“This cannot be undone.”** Deletion failures now name deletion instead of a generic Error or save failure.
- The leave-editor dialog no longer adds the tautology “Changes already saved stay saved.”
- Save, remove, and generate status messages use plain words rather than redundant checkmark/Error prefixes. Widget generation progress is **“Generating widget…”**.
- Post-save network guidance is **“Saved. Review network access before leaving.”**, removing the incorrect “below” pointer to a review region above the status.

### Saved data panel

- Renamed **Stored state** to **Saved data** in its heading, dynamic count, and accessible controls; **Save state** became **Save data**.
- Replaced the localStorage/type-control explanation with **“Data saved by this artifact syncs across your devices.”**
- Renamed the panel's Cancel button **Discard data edits**, distinguishing it from navigation cancellation.
- Grouped **Export data (JSON)** and **Import data (JSON)** separately, with each explanation directly below and programmatically associated with its button. On phones these groups stack.
- Export help states that connected-account results are included but credentials and unsaved edits are excluded, and asks users to review the file before sharing.
- Import help states that an exported JSON file replaces saved data and discards unsaved data edits.
- Renamed **Erase all data** to **Erase current data** and separated it from edit/transfer controls.
- Erase confirmation no longer falsely denies version history. It states that current data is permanently deleted, older versions and open pages may retain copies, and the artifact and permissions remain.
- Erase success now says **“Current saved data erased.”**
- Empty state: **“No saved data yet. Add a key to enter data manually.”**
- Uneditable data shapes: **“This value can be viewed or deleted, but not edited here.”**
- Load/export errors use **saved data**, not state. Save success says **“Saved. Reload the artifact to use this data.”**

### Stored assets panel

- Replaced the explanation of binary payloads, WebAssembly heaps, separate storage, and fetch calls with a short description of files saved with the artifact.
- Kept a concise warning beside the list: deleting a file is permanent, may break the artifact, and should follow checking its URL in the source.
- Grouped **Export assets (ZIP)** and **Import assets (ZIP)** with their own help. Export describes files plus the URL/content-type manifest; import describes replacing saved files with a ZIP exported from this artifact.

### Data and asset import confirmations

- Use **saved data** / **stored assets**, replacing “stored state.”
- Ask directly about replacing the current contents. Explicitly state that current contents will be permanently replaced and older versions may retain earlier copies. Data import separately discloses discarded unsaved edits.
- Removed the repeated “Continue only if you understand…” admonition.
- Failure messages identify **saved data** or **assets**, rather than exposing the internal `state` name.

### Card widget and versions

- Renamed **Gallery widget** to **Card widget**. Its summary now says **custom/default**, replacing custom tile/default tile.
- Widget explanation now describes a read-only preview on the library card that can show saved data; clicking opens the artifact.
- Added a persistent **Widget HTML** label and help below the source field. The helper correctly directs users to **Remove** to use the default widget; it no longer promises that saving an empty custom widget will do that.
- Empty-widget validation and removal confirmation use **widget** consistently and explain the default-widget outcome. The disabled generation tooltip now says the agent is unavailable and offers entering Widget HTML manually, instead of exposing a missing binary diagnostic.
- Widget save/generation warnings now say network access is blocked and point to **Permissions**, replacing “not on the allowlist.”
- Shortened the Versions introduction to what a version contains and the fact that restoring adds a version while retaining the current one.
- Version-preview guidance consistently says **Restore**, replacing the competing “Roll back” name. Shortened its details while preserving the fact that preview edits are discarded.

## Viewer — `/artifacts/{id}`

Sources: [detail.tmpl](../../internal/api/templates/detail.tmpl), [direct_open.tmpl](../../internal/api/templates/direct_open.tmpl), [components.tmpl](../../internal/api/templates/components.tmpl), [detail.js](../../web/gallery/detail.js).

- Unsupported-feature headline now explains that the preview cannot provide a feature and offers a new tab. For protected connected previews, it instead says the feature is unavailable there.
- Shortened separate camera, microphone, combined-device, location, background-worker, and fallback details to useful explanations instead of browser-origin theory.
- Protected-preview details no longer recommend the disabled new-tab remedy. The redirected-request case remains separate and explains that another site needs permission and a new tab does not bypass the restriction. Shared recipients are told to ask the owner to review the request, rather than use owner-only Edit → Permissions controls.
- Redirect warnings hide **Open in new tab**, since it cannot resolve the permission block. The action remains available for preview limitations it can resolve.
- Shared-recipient blocked-network guidance now plainly says only the owner can allow new sites. Runtime recipient banners use **artifact** and **sites**, replacing tool/origins.
- Saved-data-change banner now gives the remedy and its cost: **reload to use the latest data; unsaved input may be lost**. Its button is **Reload artifact**.
- Download and clipboard consent retain their enduring scope but point to the actual **Edit → Permissions** controls.
- External-link consent now explicitly covers **future external links, including other sites**, while keeping the destination and trust warning.
- Camera/microphone and location consent now explain the new tab, browser permission, remembered Exhibit approval, and revocation path without the long browser-mechanics explanation.
- Direct-open heading is **“Open in a new tab?”** and its confirm action is **Open in new tab**. The body explicitly retains that the artifact runs **outside the preview sandbox** as a regular web page, with downloads, navigation, and browser features under normal browser controls.
- Viewer agent action is **Edit with agent**, replacing Modify with agent.
- Toolbar movement instructions use direct language for dragging, arrow keys, and moving beyond either side to hide controls.
- Renamed **Download file** to **Download HTML**. Added visible secondary text beside it: **“Includes assets. No saved data or account access.”** Updated the tooltip to the same artifact terminology.
- **Update from source** confirmation no longer says the action is unversioned and irreversible. It explains the new version, restoration through Edit → Versions, unchanged network permissions, and retained data that may no longer fit the updated page.
- Refetch progress and outcomes are plain **Already up to date**, **Updated. Reloading…**, **Updated**, and **Couldn’t update: …**, replacing symbol/Error-prefixed messages.

## Share and duplicate dialogs

Sources: [partials.tmpl](../../internal/api/templates/partials.tmpl), [duplicate.tmpl](../../internal/api/templates/duplicate.tmpl), [gallery.go](../../internal/api/gallery.go).

- Renamed **Share Settings** to **Share artifact**, **Link Access** to **Public link**, and **Saved Data for People with Access** to **Saved data for people you add**. The data-mode legend uses the same scope.
- Consolidated repeated account-access warnings into one introduction: **“Sharing can expose saved results from connected accounts, but never grants account access.”** Removed the duplicate People paragraph and repeated public-link rule from the switching disclosure.
- Public-link help states that visitors can view but cannot change saved data **in either data mode**.
- Added the **Add people** label, persistent instruction to use existing Exhibit usernames/emails separated by commas, a shorter field placeholder, and an explicit **Give access** submit button.
- Empty recipients list now says **“Not shared with anyone yet.”**
- Mode names are **Separate data** and **Shared data**. Their descriptions distinguish each person's own data from the owner's data that named recipients can view and update; “everyone” no longer incorrectly includes public writers.
- Shortened the nearby review reminder to **“Review your saved data before sharing.”**
- Renamed the disclosure **“What happens when I switch?”** and replaced “personal grant” with **people you add**, retaining the explanation of preserved separate data.
- Renamed **Embed tile** to **Embed widget** and described the embedded read-only card widget.
- Duplicate options are **Copy sharing settings** and **Copy saved data**. Sharing help explains access for the same people and a new public link; data help discloses connected-account results and asks users to review the data before also copying sharing settings.
- The card sharing receipt separately identifies named-person writers and public-link readers, including the public no-save restriction.

## Agent — `/agent`

Sources: [agent.tmpl](../../internal/api/templates/agent.tmpl), [agent.js](../../web/gallery/agent.js), [transcript.go](../../internal/agent/transcript.go).

- Unavailable-agent banner now states the outcome without exposing a missing `pi` binary diagnostic.
- Composer placeholder is **“Describe what you want to build or change…”**.
- Keyboard hint retains Enter/Shift+Enter but removes the repeated element-selection shortcut; that shortcut remains on its own control.
- Preview nudge is **View updated preview**. The preview link is **View artifact**, replacing Details. Element-selection action is **Select an element**, replacing Snippet an element.
- Preview and widget labels consistently use **artifact** and **Card widget**.
- API-key explanation is shorter while retaining encryption, server use, and no redisplay. Existing-key help explains keeping the mask or entering a replacement. Save success is just **API key saved**, instead of repeating the encryption explanation.
- Added both visible and accessible **Remove API key** text to the trash control.
- History empty states are **“No earlier conversations about this artifact.”** and **“No messages in this conversation.”**
- Older non-resumable conversations now say they can be read but not continued. When the agent is unavailable, the note explicitly says past conversations remain readable.
- Version mismatch says the conversation **last used** a version. Actions are **Preview vN**, **Restore vN and continue**, and **Continue with current version**, replacing longer preview and rollback wording. Restore success, failure, and uncertain-result messages also say **restore** rather than roll back.
- Artifact-created/updated messages no longer repeatedly tell users which side or tab contains the preview. New network messages explain that access stays blocked until approval.
- Widget save notices are shorter and direct blocked-site remediation to Edit → Permissions.
- Removed the internal **Prepared artifact source** notice from both live chat and recorded conversation history.
- Extension failure is a plain retry instruction; provider retry status says the AI provider did not respond, retaining attempt counts.
- Live and recorded data activities consistently say **saved data** or **saved data key**, replacing state. Data-clear result specifies current saved data.

## Settings — `/settings` (also `/profile`)

Source: [profile.tmpl](../../internal/api/templates/profile.tmpl).

- Removed the heading's full stop and redundant “Manage your storage…” subtitle.
- Shortened the missing-email explanation to identify the displayed account ID and its use by administrators.
- External sign-in help directs users to manage their password and sign-in methods with their identity provider.
- No-login copy plainly explains that anyone who can reach the instance can access its library, replacing service-token/origin implementation details.
- Storage helper is now **“Includes artifact files, widgets, versions, and assets.”**
- Removed the long Advanced/Experimental connection explanation and duplicate advanced-navigation link. Useful account allowances are retained under a concise **Connection limits** disclosure.
- Sign-out helper is **“Sign out on this device.”**
- Account-deletion summary names the account, library, saved data, and share links; the irreversible warning remains in the actual confirmation.
- Connection-disconnection failure copy now plainly explains that deletion pauses for retry and that some services may already be disconnected.
- Share-count consequences, deletion confirmation, and share-breakage sentences are shorter while retaining counts and permanent deletion. Hosted/provider-account distinctions remain intact.
- Connected-agent token fallback heading is **Use a token instead**.
- Added full token scope before issuance: library-wide artifact/widget search, creation, reading/editing, and reading saved data; no artifact sharing/deletion or saved-data changes; access lasts until revoked. Associated this warning with the form.
- One-time token instruction now also says **keep it private**.

## Connections — `/connections`, artifact connections, Advanced

Sources: [connections.tmpl](../../internal/api/templates/connections.tmpl), [connections_page.go](../../internal/api/connections_page.go), [relay.tmpl](../../internal/api/templates/relay.tmpl), [connections-controller.js](../../web/gallery/connections-controller.js).

- Shortened the page introduction and removed the repeated per-account custody suffix. Retained the shared account-privacy explanation.
- Artifact access introduction asks users to select an account to review what the artifact can read or change. Grant preview retains the trust/scope warning and concisely discloses that saved results may be shared/exported.
- Reconnect copy uses **artifacts**, as do custom-credential scope explanations, disconnect confirmation, account-connected notices, artifact-return navigation, and the account-grant action.
- Empty-account guidance only appears when setup providers exist. The artifact next step only appears when there is an account to grant; an instance without services no longer suggests impossible setup. The artifact-specific access section also hides choose-account instructions when no accounts exist and says **“No accounts are available for this artifact.”** when setup is unavailable.
- Renamed **Relay permissions** / **Allow relay requests** to **Server requests** / **Allow server requests**. Shortened help while retaining the ability to read/change data within API/account permissions.
- Network labels use **Allowed sites** and **No sites allowed yet**.
- Scheduled refresh explains updating saved data even while the artifact is closed; **Save to localStorage key** became **Saved data key**. Timing help explains random scheduling and retained data on failure.
- **Saved results are separate** became **Saved data**. Disconnect help now says future access stops and previous saved results remain.
- Saved-results help uses the same **Shared data** / **Separate data** names as sharing, and distinguishes data exports/app bundles from HTML-only downloads.
- Runnable download is **Download app bundle (ZIP)**, instead of the ambiguous Download app.
- General next step names the actual **Connections** action. Owner-only shared-data clearing guidance uses artifact terminology.
- Advanced **API origin** label adds **(scheme and host)**. Limits help concisely explains cross-artifact/device scope, reused GET responses, and blank/zero values.
- Grant details say the artifact can send results to its other approved destinations. Connection approval failure uses the account's service rather than “connection origin.”
- Permanent clear confirmation explicitly distinguishes owner history clearing from a recipient's own data, says **This cannot be undone**, and retains unchanged account/network permissions and possible copies in open pages.
- Server-request consent points to the actual **Connections → Advanced connections → Server requests** revocation path.

## Sign-in, OAuth, not-found and render fallback pages

Sources: [auth.go](../../internal/api/auth.go), [login.tmpl](../../internal/api/templates/login.tmpl), [oauth.tmpl](../../internal/api/templates/oauth.tmpl), [notfound.tmpl](../../internal/api/templates/notfound.tmpl), [opengraph.go](../../internal/render/opengraph.go), [render.go](../../internal/render/render.go).

- SSO action is **Sign in with SSO**, replacing Continue with your identity provider.
- SSO verification/expiry failures now explain that the attempt expired or could not be verified. Provider refusal and other failures have plain explanations; disabled-account refusal remains distinct. Technical failure details remain in logs.
- Added a server-rendered **Sign-in failed** page with **Return to sign in** beside the explanation; authentication refusal behavior and HTTP status remain unchanged.
- OAuth scope now precisely says the agent cannot **share or delete artifacts**, or change saved data, instead of leaving the objects of share/delete implicit.
- Removed all decorative 404 captions, including Untitled/n.d./Deaccessioned and **NOT ON VIEW**, plus the “link arrived slightly bent” explanation.
- 404 heading is **Page not found**; body asks users to check the link or search the library. Requested path moved into **Technical details**.
- 404 recovery action is **Back to library**. Removed the second Add artifact action in the body; the header already supplies it.
- Invalid/revoked public-link page adds recovery: **“This link is unavailable. Ask the sender for a new link.”**
- Protected direct-render fallback says **Open this artifact in Exhibit**, explains where connected accounts/private saved data are available, and labels its recovery link **Open in Exhibit**.
