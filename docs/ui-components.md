# Reusable UI components

A reusable gallery component consists of a Go template partial, shared CSS, and
a Stimulus controller. Go owns markup and data. Stimulus owns browser-only
interaction and lifecycle. htmx swaps server-rendered regions after mutations.

## Loading and registration

Include `{{template "componentScripts"}}` once in a full app page using shared
controllers. It loads `/assets/gallery/controllers.js` as a native ES module.
That entry point starts one application and explicitly registers controllers.
Relative imports resolve on the app origin. Fragments contain markup only;
inserting `data-controller` markup connects its controller automatically.

Stimulus is pinned in `web/stimulus/package.json`. `make assets` copies its ES
module and MIT license into the embedded assets. The gallery remains unbundled,
and production needs no Node process or external asset service.

## App header

Every full-document app page renders `{{template "appHeader" .Header}}` from
`templates/components.tmpl`: gallery and shared-library views, Settings,
artifact editor and viewer, account administration, agent, add-artifact,
not-found, and both local and hosted authentication pages.
`appHeaderView` in `internal/api/appheader.go` supplies page-owned content.
The default variant provides a title, optional logo/back link/search, and
native link or button actions. Route guards enforce permission independently.
The gallery and hosted Settings retain the shared `accountControl` menu,
including its recovery link. Authentication pages supply branding and their
page title without signed-in navigation or account controls; local sign-in
remains scriptless.

For a title and return to the gallery, populate the page's `Header` with
`subpageHeader("Page title")`. It keeps the Library back label visible, with
the page title in the accessible header and the visible page heading below.
For a different destination, supply
`Back: &appHeaderBack{URL: destination, Label: destinationName}`. Set
`CollapseLabel` when the return label should become visually hidden on phones;
otherwise long labels truncate. `HideTitle` keeps a gallery heading available
to assistive technology without drawing it beside the logo.

Each `appHeaderAction` has a `Label` and self-hosted Phosphor `Icon`. `Href`
makes an anchor; otherwise it renders a button with an optional `ID`.
`LabelID` identifies a label that a feature updates, such as the agent key
status. An optional `TitleIcon` decorates the page title.
`IconOnly` hides the visible label at every width; other action labels become
visually hidden at 600px and below. Accessible names and tooltips remain.
`Primary` selects the primary button style. The editor's `SaveEditor` flag
keeps the visible Save label at every width and delegates `data-save-editor`
to the unsaved-changes controller. Its Saved/Unsaved/Saving status stays in the
56px header, with Save disabled while clean or pending. A section-provided save
issue appears in a persistent in-view explanation, and successful Save stays in
the editor; View artifact is the separate navigation action. The shared editor
session still owns in-flight deduplication and per-section persistence.
Search lives on `/search` at every width, using the shared `gallerySearch`
partial and gallery controller. The filter trigger lives in `galleryHeaderSearch`.
Starred has no visible search or filters; Activity owns its context feed.
The four destinations are Starred, Search, Shared, and Activity. The registered
`artifact-stars` controller handles personal Star/Unstar menu events, then asks
htmx to refresh the server-rendered cards through `exhibit:artifacts-changed`.

`.app-header` and its child classes in `components.css` own the layout, including
when the bar is nested inside a gallery controller. The baseline is the
rendered gallery: 56px tall with 44px controls at every width, plus any phone top safe-area inset. Horizontal
padding is 16px (8px at 360px and below), with 12px desktop gaps and 8px phone gaps. On phones the filter label is visually
hidden, Search-page clearing occupies a separate 44px box, and the Accounts shortcut
moves to Settings (the desktop header retains it). Back links and the hosted
account trigger also have a 44px minimum hit area. Card metadata grows to fit
44px title, posture and overflow targets without overlapping their neighbors.
Page styles only extend feature layout, not the bar's dimensions.
`--header-height` supplies the shared minimum height. Account-service errors
appear in an anchored panel below the account slot without changing the bar or
its neighbors. On phones the feedback offset follows the full header height,
keeping the panel clear of the bar's lower edge. Hidden recovery actions remain
hidden until their controller reveals them.

The artifact viewer uses the `Floating` variant, populated by
`artifactHeader`. Its `floatingHeaderView` carries the title, created date,
posture, and which owner actions are available. `floatingAppHeader` owns the
pill, both popups, restore tab, and accessibility announcements; its styles
live in `components.css`. The `floating-header` Stimulus controller connects
`connectFloatingHeader` from `pip.js` and releases all listeners, observers,
timers, and animation frames on disconnect. Dragging, docking, hiding,
restoring, remembered placement, popup dismissal, and keyboard behavior are
preserved. The artifact still fills the viewport beneath these controls.

Standard headers need no new browser controller. Features compose their
existing controllers and native events. A template audit guards every full
app document against bypassing this component. Browser coverage includes the
standard pages at six widths, authentication pages without signed-in controls,
and floating-header interactions and disconnect/reconnect.

[Desktop gallery](screenshots/Exh-1og2/gallery-desktop.png) ·
[320px gallery](screenshots/Exh-1og2/gallery-320.png) ·
[Phone gallery](screenshots/Exh-1og2/gallery-390.png) ·
[Dark phone gallery](screenshots/Exh-1og2/gallery-390-dark.png) ·
[Account](screenshots/Exh-arf2/account-desktop.png) ·
[Editor](screenshots/Exh-arf2/editor-desktop.png) ·
[Accounts](screenshots/Exh-arf2/admin-desktop.png) ·
[Add artifact](screenshots/Exh-arf2/new-desktop.png) ·
[Agent](screenshots/Exh-arf2/agent-desktop.png) ·
[Not found](screenshots/Exh-arf2/notfound-desktop.png) ·
[Viewer](screenshots/Exh-arf2/viewer-desktop.png) ·
[Sign in](screenshots/Exh-arf2/login-desktop.png)

## Defining action menus

A feature supplies an `actionMenuView`. Two different menus can be defined as:

```go
artifactActions := actionMenuView{
    ID: "artifact-actions-" + artifact.ID,
    Label: "Actions for " + artifact.Title,
    Items: []actionMenuItem{
        menuLink("Edit", "ph-pencil-simple", "/artifacts/"+artifact.ID+"/edit"),
        menuEvent("Duplicate", "ph-copy", "exhibit:duplicate",
            map[string]string{"artifactID": artifact.ID, "title": artifact.Title}),
    },
}

collectionActions := actionMenuView{
    ID: "collection-actions-" + collection.ID,
    Label: "Actions for " + collection.Name,
    Items: []actionMenuItem{
        menuLink("View collection", "ph-folder", "/collections/"+collection.ID),
        menuEvent("Rename", "ph-pencil-simple", "exhibit:rename-collection",
            map[string]string{"collectionID": collection.ID}),
    },
}
```

The collection routes and event illustrate a future consumer. Each feature
implements the routes and events it offers. Pass these values in the page view
model and render them using the same partial:

```html
{{template "actionMenu" .ArtifactActions}}
{{template "actionMenu" .CollectionActions}}
```

IDs must be unique in the document, including across paged fragments. `Label`
names the trigger and menu for assistive technology. `Icon` is an optional
self-hosted Phosphor class. Optional `Class`, `TriggerClass`, and `PanelClass`
adapt appearance and placement; the gallery retains its compact upward-opening
card menu. The default menu opens below its trigger.

`menuLink` renders an ordinary anchor. `menuEvent` renders a button dispatching
its named, bubbling event with the string payload in `event.detail`.
`detail.trigger` is the visible trigger for returning focus after a modal.
Payload JSON is escaped by `html/template` and decoded by Stimulus.

Feature code listens on its stable region:

```js
root.addEventListener('exhibit:rename-collection', event => {
  // Open the feature dialog using event.detail.collectionID.
  // Return focus to event.detail.trigger when it closes.
});
```

The controller handles toggling, dismissal, one open menu per document, arrow
keys, Home/End, typeahead, Tab exit, and Escape focus restoration. Features own
API calls and refresh events.

A press outside an open menu only dismisses it (Exh-bb37): it does not follow
the link or run the control it landed on, as with native menus. That rule
lives in `web/gallery/outside-press.js`, which decides at `pointerdown` while
the menu is still open and consumes the click that follows; a touch that
becomes a scroll consumes nothing. Any popup with the same need registers
`consumeOutsidePress({ isOpen, contains, dismiss })` on connect and calls the
returned release on disconnect. The Clerk account menu is registered this way
by `clerk-controller.js`; Clerk closes its own popover on that same press. Gallery duplication listens for
`exhibit:duplicate`; its form still uses `apiFetch` and requests its htmx refresh
with `exhibit:artifacts-changed`.

![Gallery action menu](screenshots/Exh-16ds/01-gallery-action-menu.png)

[Phone layout](screenshots/Exh-16ds/02-gallery-action-menu-mobile.png)

## Adding a controller

1. Add a native module under `web/gallery/` importing `Controller` from
   `../stimulus/stimulus.js`. Register it in `controllers.js` and add its file
   to `web/gallery/build.mjs`.
2. Define markup in a named Go partial. Use scoped targets for elements, values
   for configuration, and actions for events. Shared styles use design tokens.
3. Keep browser-only state on the controller. Keep its root stable when an
   htmx update must preserve that state. Replacing the root creates a new
   instance and does not automatically preserve an unsaved working copy.
4. Use `connect()` for per-connection setup and `disconnect()` to release timers,
   observers, and subscriptions. Stimulus manages `data-action` listeners.
5. Features perform authenticated API calls and emit the existing change events
   that htmx regions listen for. Controllers do not rebuild server markup.

## Native modal dialogs

The registered `modal` controller connects the dialog module in `modal.js`.
Native `<dialog>` supplies the top layer and inert background. Keep the root
outside any htmx region whose refresh must preserve its working copy. Feature
Go partials own native fields and labels; `.modal-dialog` supplies visual defaults,
including associated `label[for]` captions (also inside native forms and
fieldsets), `.modal-error`, and `.modal-actions` footers.
Migrated dialogs use the component defaults for the frame, 18px heading,
13px body copy, field captions/controls/focus ring, and wrapping action footer
with an 8px gap and 18px top spacing. Do not copy these into page styles.
Use `label[for]` for captions and `.modal-check` for a wrapping checkbox label;
an optional `small` inside its text holds a description. The two roles must
not share caption styling. Read-only fields and disabled controls have shared
states. Footer buttons and checkbox rows have a 44px minimum height on phones.
Errors use `.modal-error`; a progress status uses ordinary body typography.

`modal-sheet` adds fixed `.modal-header` and `.modal-footer` regions around one
scrolling `.modal-body` regular block, with a `modal-fields` fieldset inside
when settings must lock. Do not make the fieldset itself the flex scroll
container: native fieldset layout can report overflow while refusing to scroll.
The regular block clips positioned controls with their labels and cards.
Duplicate, Edit tag and Share use this shell. `modalClose` supplies the same
44px close control and existing modal dismissal behavior. Desktop actions trail
the content; `.modal-primary` is rightmost, `.modal-secondary` precedes it and
`.modal-destructive` sits at the far left in a quiet outline. At400px and below,
actions stack full width in primary/secondary/destructive order. The phone sheet
uses the bottom safe area. Long Share content never scrolls Done away.

Field captions use `--text-label`, sentence case,600 weight and `--color-label`.
Descriptions use `--text-helper` / `--line-helper`; `.form-help` distinguishes
secondary explanations from body copy. `modal-check` option titles lead their
helpers. App-owned explanations inside details panels use `p.form-help`; the
shared panel supplies a12px paragraph gap, including nested partials and
export/import notes. `panel-transfer` pairs a transfer action with its described
help: a shared desktop action column and an8px phone gap. Task Save leads local
Add and transfer actions; resting editor destructive entries use the shared
quiet outline. Inspector utility icons are36px squares,44px at phone widths,
and every phone editor-panel action keeps the44px minimum. Buttons attached to
fields (including Add key/property) match the44px field height. Boolean value
labels retain a44px target. Editable tags keep a compact dot/label capsule and place Edit/Remove controls
beside it in32px desktop/44px phone slots, without stretching the capsule around
those hit areas. Long names wrap without squeezing controls; phone Create and
add tag fills its field column. Native-semantic checkboxes share an18px box and the self-hosted Phosphor
check glyph. Placeholders stay upright. Standard controls are44px; compact
secondary row controls are36px on desktop,44px in phone workflows.

Network & permissions uses the editor's14px group headings and shared field
captions. Local Add/Allow actions remain secondary to page Save. The origin
field has a persistent caption and a short example; the server-rendered origin
partial explains an empty allowlist and replaces that note as draft rows change.
On phones, an origin occupies its own line above the decision actions. Wrapped
security summaries keep the disclosure chevron aligned with the first line.

`checkModalVisuals` compares rendered dialogs with the matching shared shell
under the same page styles. It checks heading/body/helper/caption/field/checkbox/
footer/error roles and horizontal containment without equating content geometry.

The `color-picker` controller owns local preset/picker/hex synchronization and
hex validation. The shared `tagColorFields` partial supplies the `picker`, `hex`,
`swatch`, `selection` and `error` targets. Presets are named radios with one
tab stop, `aria-checked` selection and arrow/Home/End navigation. Four columns of
56px cells keep a complete22px sample clear of its inset check even at320px.
The selected color name (or Custom) is a caption, not another focus outline.
`input->color-picker#input` updates both inputs and announces invalid hex near
the field. `color-picker:change` reports working-copy validity to the tag feature.
Send `color-picker:refresh` to the controller after filling a working copy.
Tag saving remains in `tags.js`; htmx owns replacement and Stimulus reconnection.
The Tags panel explains immediate persistence. Its label/action rows align to
the form column, with quiet32px desktop/44px phone utilities. Attach/removal
errors live beside their controls; the feature retains their text across swaps.
Edit tag explains library-wide scope and enables Save tag only for a valid,
changed working copy. Its two-action footer keeps DOM and visual order aligned;
the body has a separate Delete entry and an in-sheet Keep/Delete confirmation.
`TestTagModalBrowser` covers preset/custom synchronization, isolation from the
creation fields behind the modal, and use after a real server-rendered swap.

[Duplicate desktop](screenshots/Exh-p3ah/duplicate/1280.png) ·
[Duplicate phone](screenshots/Exh-p3ah/duplicate/390.png)

No lifecycle action list is needed:

```html
<dialog class="modal-dialog" aria-labelledby="example-title" data-controller="modal">
  <h2 id="example-title">Edit example</h2>
  <input aria-label="Name" autofocus>
  <button type="button" data-modal-dismiss>Cancel</button>
</dialog>
```

Import `modalFor` from `./modal.js` and keep its stable handle:

```js
const editor = modalFor(dialog);
// Populate feature fields, then open with the visible return-focus element.
editor.show(trigger);
const saved = await editor.run(async () => {
  // Validate/send the feature request and display feature errors here.
  return saveFeature();
});
if (saved) editor.close();
```

The interface has three methods and one read/write value:

- `show(trigger)` opens synchronously and returns whether it opened. Native
  `autofocus` chooses initial focus. Omit the trigger to use the active element.
  Opening does not depend on Stimulus startup or a document-wide element ID.
- `close()` returns whether dismissal was accepted. Escape, backdrop, and
  `data-modal-dismiss` use this same path, including synchronous return focus.
  Padding remains content. Tab and Shift+Tab wrap between enabled fields.
- `run(operation)` immediately calls the operation once, blocks duplicate runs
  and dismissal until its promise settles, and restores each button/fieldset's
  original disabled state. A native fieldset locks a whole form when needed;
  ordinary inputs remain editable. No values are reset. Errors propagate to
  the feature. It returns the operation's result, or `undefined` for a duplicate
  run or an operation invalidated by removal of the dialog.
- `pending` reads the shared lock. Set it to `true` when event-driven work
  starts and `false` from its existing completion/failure handlers. `run` owns
  the same lock for promise-based work.

Removing the dialog releases native modal state, listeners, pending controls,
and return focus. One `modal:disconnected` lifecycle event lets permission
features deny or drop their exact request and close unused placeholder tabs;
it performs no mutation and never restores a detached trigger. Reconnecting reuses the same handle;
an old completion cannot unlock a new operation. The feature still owns any
network request and its effects; removal does not cancel an API mutation.

`pending = true` / `pending = false` applies the same lock for event-driven
work whose completion already arrives through htmx. Do not wrap those events
in a synthetic Promise.

Optional feature escape hatches are cancellable `modal:before-close` and
`modal:closed` events. The before-close event includes `detail.reason`;
backdrop-only domain decisions can veto `backdrop` without adding a configuration
flag or duplicating native listeners. Use these hooks for domain decisions or cleanup, not generic
busy handling. Duplicate uses `run` for in-flight work and retains only its uncertain-retry
payload lock: title/options stay fixed until the server confirms or refuses the
request. Buttons and dismissal use the shared pending lock. No old
`exhibit:modal-open`/`exhibit:modal-close` event interface is retained.

Features own errors, API calls, refresh events, and focus selection after an
htmx replacement. They never rebuild server markup. All thirteen application-owned dialog implementations and Duplicate use this
handle. Native top-layer ordering owns Escape; features add neither a document
keyboard trap nor manual background inertness. Browser-owned prompts and Clerk
dialogs retain their own behavior.

## Gallery lifecycle and navigation

The stable `#gallery-shell` uses the registered `gallery` controller. It owns
space request generations, history restoration and the current search
connection. Its `space` target is `#gallery-content`, which gets replaced when
switching Library, Search, Shared with me or Shared by me. Search is the second
phone tab; desktop retains its current per-space header search. These destinations remain
available with zero sharing counts; phones show the same full labels, wrapping
within each tab. The server sends the
`gallerySpaceContent` partial plus an out-of-band `gallerySearchRegion` update.
Keep the header and Clerk controller outside both regions.

`/search` and `/partials/gallery-grid/search` share one selection and card
partial. The search covers owned artifacts and incoming account grants, using
the existing title, visible source and tag index with one stable paginated order.
Shared cards retain recipient-scoped widget URLs and omit owner actions. A blank
query shows a prompt. Search does not inherit the source space’s filters.
Owned cards carry `/search?q=…` as their editor return destination. Tab clicks
and the `/` shortcut focus the Search field after the successful htmx swap;
history restoration keeps its existing focus behavior.

Each content root uses `gallery-space`. Its connection initializes widget
visibility, duplication and edit-return behavior; disconnect releases their
observers, listeners and timers. `event-scope.js` groups explicit listener
cleanup, including listeners on a stable ancestor. Pending mutations may
finish on the server after disconnect, but their callbacks cannot refresh or
focus a different space. The new destination always reloads its data.

The `filter-sheet` controller lives on the shell so header triggers and the
content dialog share a scope. Its targets may disappear when a space has no
filters. Search/filter htmx swaps keep the dialog itself while updating its
options; a full space switch replaces it along with the content.

Run the actual navigation and lifecycle checks with:

```sh
make assets
go test ./internal/api -run 'Test(SearchTabBrowser|SearchGallery.*|GallerySpace(Navigation.*Browser|Fragments|FragmentExpiredSession))$' -count=1
go test -tags screenshot ./internal/api -run '^TestCapture(GallerySpaceNavigation|SearchTab)$' -count=1
```

Empty sharing destinations retain navigation, including after the last share is
revoked. `TestGalleryEmptySpacesBrowser` exercises both themes at 320, 375, 390,
430 and 1280px, plus enlarged and long labels. Set `GALLERY_SCREENSHOT_DIR` to
a directory outside the checkout, such as `/tmp/exhibit-review/gallery`, to
capture these empty states while running the test.

[Empty sharing space at 320px](screenshots/Exh-1l70/empty-320-light.png) ·
[Dark mode](screenshots/Exh-1l70/empty-320-dark.png)

The browser suite uses the real htmx, Stimulus and Clerk loader/controller
against local SDK doubles. It checks document/header/account node identity,
load/reload/mount counts, history, permissions, duplication, widget remounts,
request races, failures, modified clicks and no-JS links. Screenshot capture
uses `shot-scraper` and verifies header identity after a real space click.

On phones, the library rail becomes tabs fixed to the bottom of the viewport
([Exh-t9bv](../.tickets/Exh-t9bv.md)). They are not sticky: a sticky bar
cannot leave its container, so when iOS rubber-bands past the end of the page
it rides up with the content and leaves an empty band beneath it. The
registered `space-tabs` controller observes the bar's border box and publishes
`--space-tabs-height` on the layout, which reserves that much clearance above
the last card or Load more. Wrapped labels, enlarged text and the bottom
safe-area inset (padding, hence the border box) are therefore all measured,
not estimated; the compact bar plus inset is the fallback until the first
measurement. At normal text size the bar is about 55px before the inset
when labels fit one line, with at least 44px link targets. Full labels can wrap
on a 320px screen; the bar grows to fit them. Desktop uses a 240px side rail,
leaving room for the bold active destination and counts without shifting rows.
`TestGalleryMobileTabsBrowser` checks 320/375/390/430px in light and
dark, 200% tab text with full labels, a simulated 34px inset, pagination, short
pages, and a screen extending past the end of the content. `TestGallerySpacesBrowser`
retains keyboard/focus checks and verifies single-line desktop labels and stable
row positions through space changes at 768px and 1280px, including two-digit counts.

While a phone user scrolls down, the header and these tabs slide off screen
and return on the way back up ([Exh-se4n](../.tickets/Exh-se4n.md)). The
registered `scroll-chrome` controller on `#gallery-shell` only sets
`data-chrome-hidden`; `index.css` applies it inside the phone breakpoint as a
transform, so layout and the tabs' reserved clearance never move, and the
shown state carries no transform that would capture fixed popups in the header.
The attribute sits on the shell, which outlives space switches, so swapped-in
tabs inherit it. The direction rules are the pure `scroll-chrome.js`: a 12px
tolerance measured from the furthest point reached, the bars always shown
within a header's height of the top and at the end of the list, and positions
clamped to the scrollable range so iOS rubber-banding reads as no movement.
Focus entering a hidden bar reveals it, and restores the scroll position the
browser already changed to bring the sticky header's field into view. A
`:focus-visible` control in a bar (a search field being typed in) keeps them
shown. Reduced motion drops the slide. `scroll-chrome.test.mjs` covers the
rules; `TestGalleryPhoneChromeBrowser` covers positions, focus, a space switch
and desktop. Set `GALLERY_CHROME_SCREENSHOT_DIR` to a directory outside the
checkout to capture these states for PR evidence. Historical captures show
[scrolling down](screenshots/Exh-se4n/01-scrolling-down-390.png) and
[scrolling back up](screenshots/Exh-se4n/02-scrolling-up-390.png) at 390px.

[Before at 390px](screenshots/Exh-0n90/before-390.png) ·
[Compact tabs at 390px](screenshots/Exh-0n90/after-390.png) ·
[Desktop rail](screenshots/Exh-0n90/desktop-1280.png) ·
[Single-label compatibility fixture at 200% text](screenshots/Exh-0n90/single-labels-200-320-dark.png).
The last capture uses the full single labels with a simulated 34px bottom inset.
The combined navigation regression checks the same markup at 200% text. These are
Chromium captures, not physical iPhone, VoiceOver or browser-zoom checks.

## Unsaved changes

The editor's body uses the registered `unsaved-changes` controller with the
`editorLeaveDialog` partial. Its `dialog` target composes the shared modal for focus
containment; its `status` target reports whether there are unsaved edits.
Escape and Keep editing close the dialog and return focus to the link.
Browser reload, Back, and tab closure use the native beforeunload prompt.
Protection resets on pageshow when browser Back restores a cached editor.
Confirmed deletion and version restoration emit `exhibit:editor-exit` before
navigating, so they do not ask for the same discard decision twice.

Sections register `isDirty`, asynchronous `save`, and `discard` operations with
`editor-session.js`. A successful save returns `true`; a failed or incomplete
save leaves its error on the page and returns `false`. The coordinator shares
in-flight saves, stops on failure, and checks for further edits before allowing
navigation. It preserves server-confirmed changes after partial saves.
The source and widget section tracks the submitted snapshot, so text entered
while a request is pending remains unsaved. Section code emits
`exhibit:editor-changed` when its internal working copy changes without a form
input event; CodeMirror emits input on its underlying textarea.

## Testing

The existing Node suite tests feature logic in its small DOM. Stimulus
integration uses a real browser through the existing `chromiumtest` helper:

```sh
make assets
go test ./internal/api -run 'Test(AppHeaderBrowser|ActionMenuBrowser|GalleryActionMenuDuplicationBrowser)$' -count=1
```

The helper finds a locally installed Playwright Chromium headless shell, or
uses `CHROMIUM_TEST_BINARY`. It skips browser tests when neither is available.
Cases cover two different menus, escaped payloads, keyboard/focus, presses
outside that dismiss without activating what they land on, actual htmx
replacements and appends, disconnect/reconnect, and the existing duplication
dialog, including modal removal/reconnection. The full application journeys in
`e2e/` cover persisted state and Duplicate's save/htmx replacement, cancellation,
keyboard focus, pending-save guards, and retries after a committed response is
lost. These run in CI; local Playwright execution is explicit for relevant
frontend work or failure investigation. See [browser test setup](../e2e/README.md).
Add browser integration cases for new controllers; keep the miniature Node DOM
focused on feature logic.

## Account menu and Settings

The gallery and Settings header render the same `accountControl` template partial.
Hosted accounts have one Clerk user button. `clerk-menu.js` adds a Settings link
using Clerk's supported custom menu API. Clerk's Profile and Security screens
retain their standard content.

The server renders a non-interactive circular placeholder and reserves the
`--header-control-size` square (32px) for the account slot, matching the trigger's
appearance options. The same theme tokens style the placeholder in light and
dark mode. The mount remains hidden, inert, and excluded from the accessibility
tree through deferred SDK startup, the permission reload, and asynchronous UI
rendering. The `clerk` controller observes Clerk's supported
`button.cl-userButtonTrigger` class; an enabled trigger reveals the control and
removes the placeholder. A live status announces loading and readiness.

SDK errors, a missing session, or a permission reload/button render that takes
over 15 seconds show an anchored error panel with Try again and Settings actions.
Try again reloads the page. The slot and surrounding controls retain their
geometry. Disconnect cancels observation and the timeout, unmounts provider UI,
and prevents a stale reload from mounting into a new connection. Non-Clerk
accounts retain their ordinary Settings link with no placeholder slot. Library
space navigation preserves the existing header and account controller.

`TestClerkPlaceholderBrowser` compares header and neighboring control positions
and sizes through manually held idle, reload, wrapper-only, disabled-trigger,
and ready stages at 1280, 768, 767, 390, and 320px in both themes on gallery and
Settings. The failure and reconnect suites exercise recovery by keyboard,
timeouts, cancellation, and unmounting. These use the real app and loader with a
local SDK double, and are required to execute in browser CI. They do not validate
the live hosted Clerk UI. After upgrading Clerk, verify the supported trigger
class, appearance sizing, and the hosted Profile/Security/Settings menu.

Screenshots use that same SDK double (the ready account icon is simulated),
captured with `go test -tags screenshot ./internal/api -run '^TestCaptureClerkPlaceholder$' -count=1`
and `shot-scraper`:

[Loading desktop](screenshots/Exh-j6wa/loading-desktop.png) ·
[Loading phone](screenshots/Exh-j6wa/loading-phone.png) ·
[Ready desktop](screenshots/Exh-j6wa/ready-desktop.png) ·
[Ready phone](screenshots/Exh-j6wa/ready-phone.png) ·
[Error desktop](screenshots/Exh-j6wa/error-desktop.png) ·
[Error phone](screenshots/Exh-j6wa/error-phone.png).

Settings lives at `/settings` (`/profile` is a
compatibility alias) and offers storage usage, sign-out, and account deletion.
Its Manage profile button opens the unmodified Clerk profile modal. The Settings
header keeps that same account menu beside the Exhibit / Settings breadcrumb,
matching the approved interactive demo. Storage and artifact counts come from
the session owner, and the demo's sample data and simulated actions are confined
to `designs/settings-demo.html`.

The `account-deletion` controller is shared by hosted and self-hosted settings.
It reveals the server-rendered confirmation, resets the phrase on cancellation,
prevents duplicate requests, and displays the server's pending-deletion message
when provider cleanup needs retrying. The server enforces the confirmation and
last-administrator guard independently. `TestClerkAccountBrowser` covers the menu,
profile launch, Settings navigation, real artifact byte accounting and erasure,
confirmation, mobile layout, and complete or
pending deletion, with the provider UI boundary stubbed.

The screenshots show the real Settings page rendered with that provider stub:

![Settings](screenshots/Exh-r6a0/settings.png)

[Phone layout](screenshots/Exh-r6a0/settings-mobile.png)

## Shared native modal adoption

The tag editor composes `modal` on a stable dialog inside the Tags panel,
outside `#tags-body`. It populates fields, calls `show(pencil)`, and wraps only
the tag mutation in `run`. A successful result closes the dialog and emits the
existing refresh event. After the server-rendered body swaps, the feature
focuses the replacement pencil or the existing-tag selector after deletion.
Fields are resolved inside the owning panel rather than globally.

The stable `#tags-body` fieldset locks its controls during a mutation and the
following htmx refresh. A failed refresh exposes the template's Refresh tags
button, which retries only the partial GET. A transport failure keeps stale
controls locked; a server failure first swaps in its server-rendered error
notice. Session-recovery headers and missing-artifact fragments use htmx's
normal behavior. The feature handles its own swap/error events; it does not wrap
htmx in a Promise or restore references to controls that a swap has replaced.

Run `go test ./internal/api -run 'Test(ModalInterfaceBrowser|TagModalBrowser)$' -count=1` for native
focus/Tab, Escape/backdrop, validation, save/htmx, and disconnect/reconnect
checks. Set `MODAL_SCREENSHOT_DIR="/tmp/exhibit-review/modals"` to capture PR evidence.

[Tag editor desktop](screenshots/Exh-p3ah/1280.png) ·
[Tag editor phone](screenshots/Exh-p3ah/390.png)

Sharing uses the same handle on `#share-panel`; htmx replaces only its body.
Its native `#share-body` fieldset locks all settings as one working copy.
Sharing sets `pending` before the mutation and clears it after the htmx swap or
refresh failure. Its existing event handlers retain responsibility for
reconciling server state and selecting focus after the fragment is replaced.
There is no second busy flag, control-disable loop, or promise wrapping the swap.
After a failed refresh, the fieldset stays disabled because the settings are
stale; Done takes focus and reopening retries only the fragment. Input/IME,
disclosure and list scroll state remain feature-owned. Native dialog scrolling keeps Done reachable in short
windows. `TestShareModalBrowser` checks both viewport sizes and real swaps.
`TestShareModalTransactionsBrowser` covers partial success, pending guards,
failed/lost refresh recovery, stale link IDs, and reconnecting the feature root.

Sharing retains a 560px content width: the recipient entry/action and the two
saved-data choice cards need more room than a small form. It still obeys the
shared viewport cap and scrolls as one surface on short windows. The guest list
alone is capped at 30dvh so a long list does not bury link/data settings. The
status can wrap beside Done. Monospace read-only link/embed fields distinguish
copyable code; the data-mode radio cards keep their explanatory illustrations.
Heading, body, checkbox and footer styling use shared defaults.

[Sharing desktop](screenshots/Exh-9xfp/1280.png) ·
[Sharing phone](screenshots/Exh-9xfp/390.png)

[Long content at 390×400](screenshots/Exh-9xfp/short-content/390.png) ·
[Expanded explanation and Done at 390×400](screenshots/Exh-9xfp/short-actions/390.png)

The BYOK API-key dialog uses `modal`, initially focuses the secret field, and
forgets its editable secret on close. Saving/deleting uses `run` and a native fieldset to lock the working copy
until the request and key-status refresh settle; server errors retain the form. Platform mode renders no
key dialog and never starts key operations. The native module binds the key
controls; compatibility exports remain for other existing page integrations.
`TestAgentKeyModalBrowser` exercises pending Escape and a server failure with
an empty-field screenshot (no credentials).

Key explanatory paragraphs use shared body typography. The current-key summary
keeps its bordered metadata box so an existing masked credential is distinct
from editable fields. Read-only fields use the shared state styling.

[API key desktop](screenshots/Exh-uxyy/1280.png) ·
[API key phone](screenshots/Exh-uxyy/390.png)

Gallery filters compose `filter-sheet` (trigger-to-instance routing) with
`modal` on the stable sheet. Only options swap; bottom-sheet sizing and safe
area padding stay in gallery CSS. The close button is the initial focus.
`TestGalleryFilterBrowser` covers actual htmx filters; `TestFilterModalBrowser`
covers two independently addressed instances and shared keyboard dismissal.

The sheet keeps its bottom anchor, full width up to 560px, 85dvh scroll cap,
bottom-only square corners, and bottom safe-area inset. These make it a reachable
phone filter surface rather than a centered form. Done spans the sheet for a
clear exit after selecting filters; its footer spacing and phone touch height,
heading typography, and base padding come from the shared dialog.

[Filters desktop](screenshots/Exh-gynk/1280.png) ·
[Filters phone](screenshots/Exh-gynk/390.png)

Admin entitlements use the shared modal with the invoking account button as
return focus and Plan as initial focus. The feature parses raw text as exact,
safe whole bytes (blank is `null`); it retains fields on authorization/network
failure. `run` locks buttons and the native fieldset while saving. The form submits through the native
ES module. `TestAdminEntitlementModalBrowser` checks malformed and blank input,
server refusal, pending Escape, and account-specific return focus.

The entitlement editor uses the standard 420px frame, heading, associated
captions, body explanations, fields, error and action footer. Its dialog heading
has a unique ID separate from the page's Entitlements section. Long account
identifiers wrap through the shared heading rule; no private width or type
scale is needed. Add-account page controls retain their page styles.

[Entitlement desktop](screenshots/Exh-b98w/1280.png) ·
[Entitlement phone](screenshots/Exh-b98w/390.png)

[Long account desktop](screenshots/Exh-b98w/long-account/1280.png) ·
[Long account phone](screenshots/Exh-b98w/long-account/390.png)

The editor's leave dialog composes `unsaved-changes` with `modal`. Keep editing
is initial focus. Escape returns to the attempted navigation link; backdrop
clicks intentionally leave this decision open. The close hook vetoes only backdrop dismissal. `run` owns the
save/discard lock and the handle closes after the existing coordinator settles. Browser beforeunload and pageshow restoration remain with
the editor coordinator. `TestEditorLeaveModalBrowser` covers this policy;
`TestEditNavigationBrowser` and `TestEditNavigationPermissionSavesBrowser`
exercise saves, discard, Back, permission edits and partial page refreshes.

Unsaved changes uses the existing `.modal-wide` 480px content size for its three
explicit decisions. Heading, body, padding and wrapping footer use the defaults;
phone controls share the same 44px minimum as every dialog. No private button
sizing or mobile padding remains.

[Unsaved changes desktop](screenshots/Exh-yrgf/1280.png) ·
[Unsaved changes phone](screenshots/Exh-yrgf/390.png)

The browser measures that the three labels need more than the default 374px
content area and fit the wide frame's 434px content area.
[Actions at 390×240](screenshots/Exh-yrgf/short-actions/390.png)

Runtime network prompts are one imported ES module used by detail and agent.
`modal:closed` advances exactly one queued report, so a late block reply cannot
consume the next origin. Allow clears the old queue and refreshes capabilities
before reloading the current frame. Reports retain their artifact and frame
identity; replies from a displaced preview cannot reload its replacement.
Block once is initial focus; transient dismissal stays available while a block
write is pending. `modal:disconnected` lets features discard transient requests
without performing mutations or restoring a detached trigger.

`TestNetworkModalBrowser` runs native keyboard/backdrop and delayed block
settlement on both pages at both widths. Node feature fixtures import the actual page and dialog modules and drive
the public handle/native cancellation; browser suites cover native focus,
Escape, top-layer behavior, and Stimulus removal.

Network uses the shared 480px wide frame for its three distinct decisions;
its heading, explanation, status and wrapping footer use the same defaults.

[Network prompt desktop](screenshots/Exh-1grc/1280.png) ·
[Network prompt phone](screenshots/Exh-1grc/390.png)

Download permission uses `modal` with Block as initial focus and the live
artifact frame as return focus. Dismissal discards its bytes; delayed approval
checks request identity and the frame's document generation before downloading.
An iframe navigation clears the pending request even when its WindowProxy is
unchanged. Approval failures are shown inside the dialog and allow retry.
`TestDownloadModalBrowser` checks native dismissal, failed/late grants, and a
successful download's filename, MIME type and byte count.

[Download desktop](screenshots/Exh-fvzb/1280.png) ·
[Download phone](screenshots/Exh-fvzb/390.png)

Clipboard permission uses the same safe Block focus and native dismissal.
The feature settles displaced and denied request IDs once, ignores late replies,
and invalidates requests on frame navigation. Clipboard access starts in the
Allow gesture; its result is delivered only after the grant persists and the
request is still current. Native paste remains browser-owned. The browser test
uses a clipboard adapter to assert real transient activation and relays replies
through the sandbox frame; it never reads the operator's clipboard.

[Clipboard desktop](screenshots/Exh-3917/1280.png) ·
[Clipboard phone](screenshots/Exh-3917/390.png)

Clipboard retains a request-specific Allow guard through both the grant write
and the browser operation. Block/Escape stay live; a late operation cannot send
a second reply after denial. This differs deliberately from shared `pending`,
which locks every dismissal.

External-link permission uses `run` to lock buttons and every dismissal while
approval is pending, with no feature busy flag or control loop. The selected destination is immutable during
that request. Allow claims a blank tab in the click gesture, severs its opener,
and navigates only after persistence; failures and stale requests close the
placeholder. Block/Escape/backdrop record no denial. Initial focus is Block;
closing restores the artifact frame. `TestLinkModalBrowser` checks delayed
approval, refusal/retry, destination stability and opener isolation.

[External link desktop](screenshots/Exh-d0np/1280.png) ·
[External link phone](screenshots/Exh-d0np/390.png)

Camera and microphone share one `modal` consumer. The feature retains the
requested device set, per-device PATCHes, denial/unsupported replies, and the
direct-open continuation. Dismissal or displacement settles the matching call;
frame navigation invalidates it. Cancelling direct-open leaves device consent
available. Direct-open is also a shared native dialog, so its confirmation
sits above device consent in the browser top layer.
`TestMediaModalBrowser` covers camera-only persistence, microphone copy/denial,
and nested Escape/focus without acquiring real devices.

[Device permission desktop](screenshots/Exh-w432/1280.png) ·
[Device permission phone](screenshots/Exh-w432/390.png)

Location consent is another shared-modal consumer. The feature owns
`geolocation_approved`, direct-open continuation, matching denial/unsupported
replies and frame-generation guards. Cancelling the nested direct-open dialog
returns to location consent; replacing a pending location call settles the old
one. No browser geolocation grant is spent by these tests.
`TestLocationModalBrowser` checks both widths, native dismissal, displacement,
and the successful continuation through direct-open.

[Location desktop](screenshots/Exh-7hh4/1280.png) ·
[Location phone](screenshots/Exh-7hh4/390.png)

Direct-open uses the shared handle for both viewer and agent. The feature
retains viewer/artifact-scoped preferences, synchronous blank-tab claiming,
opener severance, blocked-popup retry, approval callbacks and exactly-once
promise settlement. Cancellation/disconnection closes unused placeholders. Its
fallback resolves the current preview or More actions trigger after htmx
replaces/hides the original. Manual background inertness and the document
keyboard trap have been removed, along with obsolete overlay styles.
`TestDirectOpenModalBrowser` runs both pages at both widths;
`TestPermissionModalStackBrowser` verifies topmost Escape across independent
and nested prompts. Pending-approval and actual agent htmx-swap checks cover
late settlement and the newly rendered preview URL.

Every consumer tested through `dialogCycle` also removes/reconnects the same
open dialog and verifies native modality is released. The editor separately
checks removal/reconnection, navigation and pageshow because its backdrop policy
is intentionally different.
CI's `e2e/check-browser-tests.mjs` requires all migration browser suites to run,
so skipped native-dialog coverage cannot silently pass CI.

Direct-open uses the shared checkbox row and error presentation. Its explanation
and remember choice wrap within the same default frame as the other small
permission forms; it needs no private label/field styling.

[Direct-open desktop](screenshots/Exh-sqny/1280.png) ·
[Direct-open phone](screenshots/Exh-sqny/390.png)


Run all modal integrations and capture screenshots outside the checkout with:

```sh
MODAL_SCREENSHOT_DIR="/tmp/exhibit-review/modals" go test ./internal/api -run 'Test(.*Modal.*Browser|DirectOpenPendingBrowser|DirectOpenPreviewSwapBrowser|GalleryActionMenuDuplicationBrowser|GalleryFilterBrowser)$' -count=1
```

Direct-open keeps Cancel live while its placeholder tab awaits approval. The tab
itself guards duplicate confirmation; there is no second generic busy flag.
Network, download, and clipboard likewise keep cancellation live and retain
request-specific write guards. Use shared `pending` for non-dismissible work
(tags, Duplicate, sharing, keys, entitlements, editor saves, and link approval);
do not apply it to cancellable permission transactions.

## UI council refinements

The library owns a visible page heading and introduction inside its swapped
content region. Gallery card menus remain visible with stable 32px desktop
and 44px phone targets; hovering never shifts the title. Empty states span the
grid, describe the next step and use ordinary/htmx navigation for recovery.
The stable polite results status is refreshed out of band by the same server
partial as filters. Styling and wording are shared across library spaces.

The add page is a native ES module. Its feature-owned validation marks and
focuses the actual field, including the CodeMirror contenteditable, and locks
creation while an API request is pending. The editor island transfers each
textarea's accessible name and description to CodeMirror. Source and widget
editors have distinct names. Field errors are associated inline; progress and
server errors use the page's live status region.

`ArtifactEditor.mount(textarea)` returns a small handle with `focus`,
`setInvalid`, `setValue`, and `setReadOnly`. Page code uses this handle rather
than changing CodeMirror's DOM or dispatching editor transactions itself.
Read-only mode configures both CodeMirror's state and content editability, so
commands such as Replace all respect a pending import. A completed import
restored by a persisted `pageshow` reloads the creation page; unfinished network
approval retains its working state.

The floating viewer keeps its positioning lifecycle, with 36px desktop and
44px phone/coarse controls and restore target. Its menu supports arrows,
Home/End, Tab, Escape and focus-exit dismissal. A visible tool name supplies
context; editing is directly available on desktop and in the menu on phones.

Review evidence and capture instructions are in
[the UI council brief](reviews/Exh-zrjo/brief.md).


Visual controls share the app font family, with 36px default and 32px compact
heights; fields retain 44px phone targets. Secondary boundaries use the quiet
`--color-border`, while editable fields use `--color-field-border` (at least
3:1 on the adjacent panel) and a recessed `--color-field` in dark mode. Editor
surfaces, active lines, selections, placeholders and subdued syntax use shared
palette tokens. The CodeMirror theme compartment still follows live media
changes without replacing the draft. Keep active-line fills translucent:
CodeMirror paints its selection layer behind the line content. Verify an actual
word selection on the active line, including the composited background, rather
than checking each color separately. Mobile share choices stack; fixed phone
tabs reserve a two-line label area so their icons align.

The source editor's native find/replace and line-navigation panels use the same
field, border, radius and focus tokens. They use compact 32px desktop controls
and 44px phone controls; the search-open editor keeps a visible source viewport.
Completion menus, folding markers and match hints share that palette too.
Secondary occurrences stay quieter than the actual selection/current find
result. Retain the library's keyboard behavior and verify these states in both
color schemes when changing the editor theme.


## Posture receipt placement

The server-owned `capabilityPopover` receipt shares helper typography and
sentence-case section headers on cards and inside the floating viewer's About
surface. The existing explicit activation behavior publishes `capability:changed`.
The registered `capability-popover` Stimulus controller fits the card receipt
inside the visual viewport, flips it above the anchor when needed, and limits
its height with internal scrolling. It adds resize/scroll hooks only while open
and releases them on close/removal. Shared outside-press handling dismisses
without activating a card underneath. Viewer placement remains owned by its
existing floating-header popup, with the same receipt content and type roles.
The browser journey covers keyboard activation, viewport collision/no document
growth, resizing, server-rendered replacement, Safari null-relatedTarget and
Manage navigation. No client-side markup is generated.

Selected color presets inset their check badge3px inside the1px cell border.
Its4px radius shares the8px outer corner centre, so badge and cell contours do
not collide or leave a fringe. The color-picker working-copy behavior is unchanged.
