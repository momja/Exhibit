# Browser apps with relay and connections

These are complete, self-contained HTML apps for the experimental relay. They
contain no credential, proxy URL, server script or third-party JavaScript.
Their Phosphor icons are embedded from the project's vendored MIT-licensed set;
the license is included here. The apps support narrow screens and dark mode.

- `arxiv.html` — Fieldnotes, a research reader with live arXiv search, date and
  relevance sorting, paging, expandable abstracts and a saved reading list.
  The code calls `https://export.arxiv.org/api/query` directly with `fetch`.
- `simplefin-budget.html` — Commonplace, an account overview with balances,
  currencies kept separate, monthly inflow/outflow, a saved outflow target,
  transaction filters and provider error messages. It discovers the viewer's
  granted SimpleFIN connection and requests its public `/accounts` alias.
- `connection-check.html` — a small bearer connection demonstration against
  HTTPbin, showing authentication and the redacted response.
- `connection-widget.html` — reads the last connection check or its scheduled
  `connection-check-auto` snapshot without making a request.
- `simplefin-widget.html` — a gallery widget that reads the saved `accounts`
  snapshot. It never makes a request or obtains a connection.

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
`https://export.arxiv.org`, allow relay reads in **Connections & relay**, and
search for a topic. Existing origin approvals alone do not enable the relay.
The app waits for a search action before making any request and spaces repeated
requests by at least three seconds, as the
[official arXiv API manual](https://info.arxiv.org/help/api/user-manual.html)
requests. A failed search keeps saved papers and the previous results available.

## Connection check: live bearer connection

Create a **Bearer token** connection named HTTPbin example with origin
`https://httpbin.org` and a disposable demonstration token. HTTPbin accepts any
bearer value. Grant it to **Connection check**, approve its origin, and open the
artifact. Select **Check connection** and allow relay reads. The result shows
`authenticated: true`; the echoed token is redacted before it reaches the page.
This demonstrates the complete connection form, grant, consent and authenticated
request without needing an external account.
Its widget reads saved data. To keep it fresh while the artifact is closed,
schedule `https://httpbin.org/bearer` into `connection-check-auto` on the
artifact's connection page.

## Commonplace: SimpleFIN

Open **Connections & relay** for Commonplace. Add a SimpleFIN connection by
pasting a fresh setup token into Exhibit's connection form, then grant that
connection to this artifact. Leave additional data destinations empty, approve
its API origin, and enable relay reads. Return to Commonplace and refresh.
An ordinary SimpleFIN Bridge connection uses `https://bridge.simplefin.org`;
the app discovers another origin from granted public metadata when needed.
Its optional origin setting accepts only a public origin, never an access URL.

The request is an ordinary, credential-free `fetch(origin + '/accounts?...')`.
Exhibit maps that public alias to the connection's encrypted private endpoint
and injects Basic authentication. The app requests a 30-day transaction window
and understands both v2 `errlist` and legacy `errors`; the
[official SimpleFIN protocol](https://www.simplefin.org/protocol.html) describes
the source fields. Balances and transactions remain separated by currency, and
inflow/outflow includes transfers rather than guessing which records are
spending. Raw snapshots are saved under `localStorage['accounts']`.

**Explore with sample accounts** gives a usable, clearly labeled view without
an account. Sample snapshots carry `__exhibit_example: true`; the widget labels
them too. The sample never claims that a bank is connected. Live provider
accounts require the user's connection and were not available for development;
protocol behavior is covered by TLS-backed fixture tests.

For a scheduled widget refresh, use Commonplace's **Keep data fresh** form:

```text
API URL:    https://bridge.simplefin.org/accounts?version=2
State key:  accounts
Every:      24 hours
```

Use the connection's actual public origin if different. The scheduler only
fetches and saves the response; it executes no artifact JavaScript. The same
grants, encrypted cache, daily request quota and destination policy apply.
The first run is spread across the interval. Provider failures preserve the
last saved snapshot. The widget reads that snapshot when Exhibit renders it.

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
bundle. The local SimpleFIN form claims a new setup token and keeps its access
credentials in runner memory. The public `/accounts` alias stays the same even
when the locally claimed path changes. Fieldnotes needs only its allowed origin;
Commonplace can use sample data before reconnecting.

Only the app, its assets, saved state and nonsecret connection requirements are
exported. Saved financial data is part of the artifact's state, so decide
whether to keep or erase it before sharing the downloaded ZIP. A plain HTML
export lists the origins needing a relay; opening HTML alone does not create
an Exhibit connection or a local relay.
