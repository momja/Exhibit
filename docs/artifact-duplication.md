# Duplicating an artifact

The gallery card action menu offers **Duplicate**. Its modal starts with the
original title followed by ` (copy)` and two unchecked options:

- **Copy share settings** preserves the data sharing mode, recipient grants and
  whether an anonymous public link exists. Every copied grant and link receives
  a fresh identity; the original link never opens the copy.
- **Copy data** copies only the owner's current saved state. It excludes other
  viewers' private state. In shared-data mode, the shared board is the owner's
  state and is therefore included.

The copy always keeps the current source, optional gallery widget, current
stored assets, source URL, tier, capability approvals, allowed and blocked
origin decisions, tags and collection membership. It starts with its own
initial version. Previous versions, historical state snapshots, agent
conversations, usage records and credentials do not travel with it.

After success, the modal closes and a bubbling `exhibit:artifacts-changed` event
asks its gallery to clear the active search and reset paging. The existing
htmx search form refreshes `/partials/gallery-grid`, using the same card partial
as the initial page. Clearing the search ensures a renamed copy remains visible.
The page does not navigate or reload. A failed refresh can be retried without
repeating the mutation.

## API and persistence

`POST /api/artifacts/:id/duplicate` is owner-only and unavailable to scoped
agent credentials. Its JSON body contains `title` (required, trimmed),
`copy_share_settings`, `copy_data`, and a nonzero UUID `request_id`. Optional
booleans default to false. It returns the new artifact with status 201, or the
already-created artifact with status 200 when the same request is retried.

The destination ID is derived from the authenticated owner, source ID and
request UUID. The destination row acts as the receipt: an uncertain response
can be retried even after the original changes or is deleted. The modal keeps
its request ID and original options after a server/network failure; a definite
client refusal permits editing the options before a fresh attempt.

Source and widget bytes are staged under new blob IDs. Asset identities and
generation IDs are fresh. Embedded asset URLs in source, widget and textual
asset content are rewritten for the copy. Unchanged binary payloads retain the
existing owner-scoped content addresses; reference-aware deletion keeps them
alive while either artifact needs them.

Staging passes through the existing storage quota, accounting and blob locks.
A single store transaction then checks ownership and that the source version
and asset set still match, and publishes all artifact metadata, asset rows,
optional state and optional shares together. A changed source answers 409.
Failed staging/transactions publish nothing, and unused staged bytes use the
durable deletion queue. Cleanup failures after commit do not report the
successful duplication as failed. No schema migration is needed.

Gallery ordering compares creation timestamps as instants, accommodating both
SQLite timestamps and precise RFC3339 creation times so a new copy appears on
the first page.
