---
id: av-p3gj
status: closed
deps: []
links: [av-tux9]
created: 2026-07-17T17:48:48Z
type: bug
priority: 3
assignee: Max Omdal
tags: [ui, gallery, security]
---
# Detail page: allowlist origins inlined into inline script can break out via </script>

Surfaced while fixing av-tux9. renderDetailPage (internal/api/gallery.go) inlines the network allowlist into the detail page's inline <script> as a JS array built with fmt.Sprintf("%q", o) per origin (allowlistJSON, ~line 792-799; 'let allowlist = ...' at ~line 943). Go's %q escapes quotes and control characters but NOT '<', '>', or '/'. An origin containing the literal sequence </script> therefore terminates the inline script block early, and any markup after it (e.g. an origin like https://evil</script><img src=x onerror=alert(1)>) is parsed as app-origin HTML and executes when the detail page loads. Same self-XSS class as av-tux9 (origins are user/scanner-controlled via the 'Add origin' input or PATCH), but a fuller breakout: it escapes JavaScript string context into raw HTML parsing rather than injecting via innerHTML.

## Design

Build the inlined array with encoding/json instead of %q: b, _ := json.Marshal(a.NetworkAllowlist); allowlistJSON = string(b). encoding/json escapes '<', '>', and '&' as <, >, & by default, which is simultaneously valid JS string content and inert inside an HTML <script> block (the script-data state only ends on the literal bytes </script). While here, audit the other %q interpolations into inline scripts on this page (TOKEN, ID) — both are server-generated and not user-controlled today, so the allowlist is the only required change; note any others found in the PR.

## Acceptance Criteria

An artifact whose network_allowlist contains https://evil</script><img src=x onerror=alert(1)> renders the detail page with the payload inert inside the JS array: no early </script> termination, no DOM injection, and the page's own scripts (bridges, renderBadges, toolbar) still run. A Go test asserts the served detail page contains the escaped form (</script>) for such an origin and never the raw sequence inside the allowlist literal. Existing allowlist add/save/revoke behavior and badge rendering are unchanged.

