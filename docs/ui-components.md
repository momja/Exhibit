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
API calls and refresh events. Gallery duplication listens for
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

The registered `modal` controller wraps native `<dialog>`, which supplies the
top layer and inert background. The controller wraps Tab/Shift+Tab between its
enabled fields, keeping focus in the dialog at both boundaries. Duplicate is its first
consumer. Keep the dialog root outside a region whose htmx refresh should
preserve its form state. Compose the feature's named Go partial with shared
`.modal-dialog` styling and these actions:

```html
<dialog class="modal-dialog" aria-labelledby="example-title"
  data-controller="modal"
  data-action="exhibit:modal-open->modal#open exhibit:modal-close->modal#dismiss cancel->modal#cancel click->modal#backdrop keydown->modal#wrapFocus">
  <h2 id="example-title">Edit example</h2>
  <input aria-label="Name" data-modal-target="initialFocus">
  <button type="button" data-action="modal#dismiss">Cancel</button>
</dialog>
```

Feature code dispatches `exhibit:modal-open` on that dialog with
`detail.trigger` set to the visible return-focus element. The controller opens
it and focuses `initialFocus` when present. A feature can then select its text.
Dispatch `exhibit:modal-close` after a successful save. Escape, backdrop clicks,
Cancel, and programmatic close requests all emit the same cancellable
`modal:before-close` event. Features call `event.preventDefault()` while a save
must not be dismissed. Dialog padding is part of the dialog, not its backdrop.

After accepted dismissal, the controller closes the dialog, restores focus to
the trigger if it is still connected, and emits `modal:closed`. Features own
error feedback, uncertain-request reconciliation, API calls, and refresh
events. Duplicate focuses the new card title after its successful htmx refresh;
it does not try to refocus the old removed trigger. Focus restoration is
synchronous, so an earlier queued native `close` event cannot steal focus from
a reopened dialog or refreshed result. Removing the component closes its
native modal state without dispatching a feature mutation. Stimulus owns its
action listeners and reconnects it when reinserted.

Existing feature dialogs retain their implementations until explicitly migrated.

## Testing

The existing Node suite tests feature logic in its small DOM. Stimulus
integration uses a real browser through the existing `chromiumtest` helper:

```sh
make assets
go test ./internal/api -run 'Test(ActionMenuBrowser|GalleryActionMenuDuplicationBrowser)$' -count=1
```

The helper finds a locally installed Playwright Chromium headless shell, or
uses `CHROMIUM_TEST_BINARY`. It skips browser tests when neither is available.
Cases cover two different menus, escaped payloads, keyboard/focus, actual htmx
replacements and appends, disconnect/reconnect, and the existing duplication
dialog, including modal removal/reconnection. The full application journeys in
`e2e/` cover persisted state and Duplicate's save/htmx replacement, cancellation,
keyboard focus, pending-save guards, and retries after a committed response is
lost. These run in CI; local Playwright execution is explicit for relevant
frontend work or failure investigation. See [browser test setup](../e2e/README.md).
Keep the miniature Node DOM focused on feature logic.
