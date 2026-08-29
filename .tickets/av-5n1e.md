---
id: av-5n1e
status: closed
deps: []
links: [av-qo0j]
created: 2026-08-17T21:30:45Z
type: bug
priority: 1
assignee: Max Omdal
tags: [ingest, frontend, ux]
---
# Failed URL ingest hangs the new-artifact page forever

Any failed ingest leaves `/new` showing its last progress message — "Fetching page and snapshotting assets…" — with no error, no recovery, and no indication anything went wrong. A dead URL is indistinguishable from a slow snapshot.

`web/gallery/new.js` parses before it checks:

```js
const data = await resp.json();
if (!resp.ok) { status.textContent = 'Error: ' + (data.error || resp.statusText); return; }
```

The API's failure path is `http.Error`, which writes **plain text**. So `resp.json()` throws on every error response, the rejection is unhandled (`ingest()` has no try/catch and its caller does not attach one), and the status element is never updated again.

Found while snapshotting a domain that does not resolve: the server correctly answered 400 in 37ms and logged it, while the page sat on the progress message indefinitely.

This is not specific to snapshot or to URL mode — it is every non-2xx from `POST /api/artifacts`.

## Design

Client-side, in `new.js`:

- Check `resp.ok` **before** parsing.
- Extract the message defensively: JSON `{error}` when the body is JSON, the raw text when it is not (which is what `http.Error` sends), and the status line when the body is empty or unreadable.
- Wrap the request itself so a transport failure — offline, DNS, dropped connection — reports rather than leaving the last progress message on screen. That case never reaches a response at all, so no amount of response handling covers it.

**Not fixed by making the server return JSON errors.** That would be a reasonable separate change, but it would not fix this: a client must not assume an error body is JSON, and the transport-failure case has no body at all. The client is where the bug is.

The same read-then-check shape is worth grepping for elsewhere in `web/gallery/` — this is a pattern bug, not a one-off, and the other pages have their own progress messages to get stuck on.

## Acceptance Criteria

- Ingesting a URL that does not resolve shows the server's message and leaves the form usable.
- A 4xx with a plain-text body, a 4xx with a JSON body, and a 5xx with an empty body each produce a readable message rather than a hang.
- A transport failure (server unreachable) reports rather than leaving the progress text on screen.
- A successful ingest is unchanged.

