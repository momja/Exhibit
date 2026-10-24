#!/usr/bin/env python3
"""Upload the connector examples through Exhibit's normal authenticated API."""
import argparse
import json
import os
from pathlib import Path
import sys
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default=os.environ.get("EXHIBIT_URL", "http://t3code:18080"))
    parser.add_argument("--only", choices=["arxiv", "connection-check"])
    args = parser.parse_args()
    token = os.environ.get("EXHIBIT_TOKEN")
    if not token:
        parser.error("set EXHIBIT_TOKEN to your Exhibit service token")
    base = args.url.rstrip("/")
    directory = Path(__file__).resolve().parent

    def api(method, route, payload):
        request = Request(base + route, data=json.dumps(payload).encode(), method=method,
                          headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        with urlopen(request, timeout=30) as response:
            return json.load(response)

    examples = [("arxiv", "Fieldnotes · arXiv research reader", "arxiv.html", None),
                ("connection-check", "Connection check", "connection-check.html", "connection-widget.html")]
    for name, title, filename, widget in examples:
        if args.only and name != args.only:
            continue
        # Uploading does not approve an origin, enable the relay, attach a
        # connection or acquire a credential. Those decisions stay in Exhibit.
        result = api("POST", "/api/artifacts", {"title": title, "body": (directory / filename).read_text()})
        artifact_id = result["artifact"]["id"]
        if widget:
            api("PUT", f"/api/artifacts/{artifact_id}/widget", {"body": (directory / widget).read_text()})
        print(f"{title}: {base}/artifacts/{artifact_id}")
        print(f"  Configure access: {base}/connections?artifact={artifact_id}")
        print("  Origins to review: " + ", ".join(result.get("network_footprint", [])))


if __name__ == "__main__":
    try:
        main()
    except HTTPError as failure:
        print(f"Exhibit returned HTTP {failure.code}; check the URL, service token and API permissions.", file=sys.stderr)
        sys.exit(1)
    except (URLError, OSError, ValueError, KeyError):
        print("Could not upload the examples; check the server and its API response.", file=sys.stderr)
        sys.exit(1)
