#!/usr/bin/env python3
"""Load the dev/samples artifacts (and their widgets) into a running Exhibit.

    make run                       # in one terminal
    python3 scripts/seed-samples.py

Each sample is a directory under dev/samples/ holding:

    artifact.html   the tool itself                    (required)
    widget.html     its gallery tile                   (optional)
    state.json      demo state, {storage-key: value}   (optional)

Everything goes through the HTTP API, the way any other client would — this
script is a demo fixture loader, not a back door into the datastore.

Samples are upserted by title (read from the artifact's <title>), so re-running
refreshes the bodies, widgets, and demo state in place instead of piling up
duplicates. Artifact ids therefore stay stable across runs, and so do the URLs
you may have open.

state.json values may contain two date tokens, so demo data stays live instead
of ageing into "nothing in the last 30 days" the week after it was written:

    {{date:N}}      the ISO date N days from today (negative for the past)
    {{monday:N}}    the Monday of the week N weeks from this one — for samples
                    whose schedule only makes sense anchored to a week start
"""

import argparse
import datetime
import json
import os
import re
import sys
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SAMPLES = os.path.join(ROOT, "dev", "samples")

DATE_TOKEN = re.compile(r"\{\{(date|monday):(-?\d+)\}\}")


def expand_dates(text):
    """Replace {{date:N}} / {{monday:N}} with concrete ISO dates."""
    today = datetime.date.today()
    this_monday = today - datetime.timedelta(days=today.weekday())

    def sub(m):
        unit, n = m.group(1), int(m.group(2))
        if unit == "monday":
            return (this_monday + datetime.timedelta(weeks=n)).isoformat()
        return (today + datetime.timedelta(days=n)).isoformat()

    return DATE_TOKEN.sub(sub, text)


def title_of(html, fallback):
    m = re.search(r"<title[^>]*>(.*?)</title>", html, re.S | re.I)
    return m.group(1).strip() if m else fallback


class Exhibit:
    def __init__(self, base, token):
        self.base, self.token = base.rstrip("/"), token

    def call(self, method, path, payload=None):
        data = json.dumps(payload).encode() if payload is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method)
        req.add_header("Authorization", "Bearer " + self.token)
        if data:
            req.add_header("Content-Type", "application/json")
        with urllib.request.urlopen(req) as resp:
            body = resp.read().decode()
        return json.loads(body) if body.strip() else None


def seed(ex, name, path):
    with open(os.path.join(path, "artifact.html")) as f:
        artifact = f.read()
    title = title_of(artifact, name)

    existing = {a["title"]: a["id"] for a in ex.call("GET", "/api/artifacts?limit=200")}
    if title in existing:
        artifact_id = existing[title]
        ex.call("PATCH", "/api/artifacts/" + artifact_id, {"body": artifact})
        action = "updated"
    else:
        created = ex.call(
            "POST",
            "/api/artifacts",
            # No allowlist: samples are self-contained, and seeding one would
            # be exactly the scan-seeds-approval move the spec forbids.
            {"title": title, "body": artifact, "network_allowlist": []},
        )
        artifact_id = created["artifact"]["id"]
        action = "created"

    widget_path = os.path.join(path, "widget.html")
    widget_note = "no widget (default tile)"
    if os.path.exists(widget_path):
        with open(widget_path) as f:
            resp = ex.call(
                "PUT", "/api/artifacts/" + artifact_id + "/widget", {"body": f.read()}
            )
        widget_note = "widget"
        if resp.get("unapproved_origins"):
            widget_note += " (blocked origins: %s)" % ", ".join(resp["unapproved_origins"])

    state_path = os.path.join(path, "state.json")
    state_note = ""
    if os.path.exists(state_path):
        with open(state_path) as f:
            state = json.loads(expand_dates(f.read()))
        for key, value in state.items():
            # The API stores opaque strings, exactly as the storage shim does;
            # a JSON object here is serialized the way the artifact would have.
            if not isinstance(value, str):
                value = json.dumps(value, separators=(",", ":"))
            ex.call(
                "PUT",
                "/api/artifacts/" + artifact_id + "/state",
                {"key": key, "value": value},
            )
        state_note = ", %d state key(s)" % len(state)

    print("  %-22s %-7s  %s%s" % (name, action, widget_note, state_note))
    return artifact_id


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--api", default=os.environ.get("APP_ORIGIN", "http://localhost:8080"))
    p.add_argument("--token", default=os.environ.get("AUTH_TOKEN", "dev-token"))
    p.add_argument("samples", nargs="*", help="sample names (default: all)")
    args = p.parse_args()

    names = args.samples or sorted(
        d for d in os.listdir(SAMPLES)
        if os.path.exists(os.path.join(SAMPLES, d, "artifact.html"))
    )

    ex = Exhibit(args.api, args.token)
    print("Seeding %d sample(s) into %s" % (len(names), args.api))
    for name in names:
        path = os.path.join(SAMPLES, name)
        if not os.path.isdir(path):
            sys.exit("no such sample: " + name)
        try:
            seed(ex, name, path)
        except urllib.error.URLError as e:
            sys.exit("  %s FAILED: %s" % (name, e))
    print("Done — open %s to see the cards." % args.api)


if __name__ == "__main__":
    main()
