# Browser apps with relay and connections

These are complete, self-contained HTML apps for the experimental relay. They
contain no credential, proxy URL, server script or third-party JavaScript.
Their Phosphor icons are embedded from the project's vendored MIT-licensed set;
the license is included here. The apps support narrow screens and dark mode.

- `arxiv.html` — Fieldnotes, a research reader with live arXiv search, date and
  relevance sorting, paging, expandable abstracts and a saved reading list.
  The code calls `https://export.arxiv.org/api/query` directly with `fetch`.
- `connection-check.html` — a small bearer connection demonstration against
  HTTPbin, showing authentication and the redacted response.
- `connection-widget.html` — reads the last connection check or its scheduled
  `connection-check-auto` snapshot without making a request.

## Load into Exhibit

Start an isolated instance with `RELAY_ENABLED=true` and
`CONNECTORS_ENABLED=true`. Enable `RELAY_SCHEDULES_ENABLED=true` to demonstrate
updates while the page is closed. The full configuration and boundaries are in
[relay-connectors.md](../../docs/relay-connectors.md).

Upload the HTML files through the normal Add artifact flow, or use the helper:

```bash
EXHIBIT_URL=http://t3code:18080 EXHIBIT_TOKEN=your-service-token \
  python3 examples/connectors/seed.py
```

The helper uses `POST /api/artifacts` and `PUT /api/artifacts/:id/widget`. It
prints the artifact and configuration links. It deliberately leaves all origin,
relay and connection decisions to the person using Exhibit. Do not put provider
credentials in this command or in an artifact.

To upload a single app without the helper:

```bash
jq -n --rawfile body examples/connectors/arxiv.html \
  '{title:"Fieldnotes",body:$body}' | \
  curl --fail-with-body http://t3code:18080/api/artifacts \
    -H "Authorization: Bearer $EXHIBIT_TOKEN" \
    -H 'Content-Type: application/json' --data-binary @-
```

## Fieldnotes: unauthenticated relay

Open Fieldnotes in Exhibit's artifact frame. Review and approve
`https://export.arxiv.org`, allow relay requests in **Connections & relay**, and
search for a topic. Existing origin approvals alone do not enable the relay.
The app waits for a search action before making any request and spaces repeated
requests by at least three seconds, as the
[official arXiv API manual](https://info.arxiv.org/help/api/user-manual.html)
requests. A failed search keeps saved papers and the previous results available.

## Connection check: live bearer connection

Create a **Bearer token** connection named HTTPbin example with origin
`https://httpbin.org` and a disposable demonstration token. HTTPbin accepts any
bearer value. Grant it to **Connection check**, approve its origin, and open the
artifact. Select **Check connection** and allow relay requests. The result shows
`authenticated: true`; the echoed token is redacted before it reaches the page.
This demonstrates the complete connection form, grant, consent and authenticated
request without needing an external account.
Its widget reads saved data. To keep it fresh while the artifact is closed,
schedule `https://httpbin.org/bearer` into `connection-check-auto` on the
artifact's connection page.

## Generic Basic connections

To connect an API that supplies a URL containing a username and password, choose
**HTTP Basic** in Exhibit's connection form and paste the full HTTPS URL there.
Exhibit separates its origin, credentials and base path. The artifact only needs
the public origin and endpoint shown in the connection metadata; it never sees
the pasted URL or the Basic header. Redeem any provider setup token yourself
before using the form. Exhibit performs no token claim or redemption.

Choose optional daily request and response reuse limits for the connection.
They apply across every artifact and device using it, with reusable responses
served without spending another provider request. Each connection supplies its
credential only at its matching API origin. Other approved APIs and CDNs remain
available, including when several connections are attached to the same artifact.
Approve the API origin, grant the connection to your artifact and allow relay
reads before fetching. Credential setup belongs in Exhibit's interface.

## Take either app off Exhibit

Choose **Download app** in **Connections & relay**, unpack the ZIP, and run:

```bash
node serve.mjs
# or, built from this repository:
go build -o exhibit-run ./cmd/exhibit-run
./exhibit-run /path/to/downloaded-app.zip
```

Open the address printed by the runner. Enter local credentials using the
runner's **Connections** link; original Exhibit credentials never enter the
bundle. The local runner accepts generic API key, Bearer and Basic credentials;
use its trusted connection form to reconnect. Fieldnotes only needs its allowed
origin, and the Connection check example can be reconnected with a disposable
Bearer token.

Only the app, its assets, saved state and nonsecret connection requirements are
exported. Saved provider data is part of the artifact's state, so decide
whether to keep or erase it before sharing the downloaded ZIP. A plain HTML
export lists the origins needing a relay; opening HTML alone does not create
an Exhibit connection or a local relay.
