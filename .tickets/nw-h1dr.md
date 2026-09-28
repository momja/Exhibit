---
id: nw-h1dr
status: in_progress
deps: []
links: []
created: 2026-09-28T00:00:00Z
type: task
priority: 3
---
# Drop the "Exhibit" wordmark from the gallery header
The gallery header shows the logo followed by a visible `<h1>Exhibit</h1>`. The
word tells nobody anything the tab title and the logo beside it don't already
say, and it breaks the convention every other page follows: its `<h1>` names
the page's subject (the artifact title, "Add artifact", "Edit Artifact"), where
the gallery's names the product.

Remove the visible wordmark and keep the logo. Keep an `<h1>` as well, visually
hidden, so the page still has a top-level heading for screen-reader heading
navigation.

Out of scope: the login and 404 pages keep "Exhibit" — the login page is where
the brand belongs, and the 404 page is reached by mistake, where it helps you
get your bearings. A public instance showing `PUBLIC_INSTANCE_NAME` as the
visible heading is a separate follow-up.
