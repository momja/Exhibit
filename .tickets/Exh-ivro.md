---
id: Exh-ivro
status: open
deps: []
links: []
created: 2026-11-05T03:46:08Z
type: epic
priority: 1
assignee: Max Omdal
tags: [bazarre]
---
# Bazarre: publishing, descriptions and gallery imports

Build the Bazarre as a standalone sibling repository, reusing Exhibit's design language and live artifact embeds. Only the operator can admit a vetted creator identified by a public username and exact instance. RSS distributes explicit immutable public releases; creators track publishing, issue new versions and withdraw. Artifacts gain descriptions. Visitors copy into their own self-hosted Exhibit or import and open the agent for remixing. Imported copies retain attribution and offer explicit updates while preserving local changes. Include owner security/moderation tools, documentation, realistic demo, integration passes and UI reviews with minimal copy. No main merges or deployment required.

Trust model: centralized creator approval; distributed content. No open registration, instance-wide trust, transitive federation, automatic updates or automatic agent execution. Proof of instance account control complements out-of-band human vetting; it does not establish content safety. Exact HTTPS origin + immutable creator UUID + lowercase public handle are pinned. A pending creator receives a random expiring challenge bound to this Bazarre origin; the authenticated creator publishes it on their Exhibit profile; only the operator can verify and activate. Any identity migration needs renewed vetting. Suspension immediately suppresses public browse/detail/RSS/preview/package/handoff responses and prevents sync; existing private copies cannot be remotely erased.

Distribution: RSS 2.0 with namespace urn:exhibit:bazarre:1. One release item per artifact/version, stable non-permalink GUID, plain text metadata, enclosure for JSON package and SHA-256 of exact package bytes. Feed is complete bounded history (fail explicitly if limits exceeded; never silently truncate withdrawals); omission never means deletion. Withdrawals are explicit permanent artifact tombstones. Releases have increasing integer versions and immutable metadata/content; same identity/version with changed digest is rejected. Feeds and enclosures stay on the approved creator instance. Bazarre retains immutable verified packages and local live renders; it never embeds mutable creator HTML URLs. No screenshots required.

HTTP contracts (final schema documented before implementation): GET /bazarre/creators/{username}.json ; GET /bazarre/creators/{username}/rss.xml ; GET /bazarre/releases/{release_id}.json . Discovery identifies protocol, UUID, username, instance, feed_url, audience and challenge. JSON package identifies schema, creator_id, artifact_id, release_id, version, title, description, notes, license, tier, body, optional widget_body and published_at. Public description is bounded plain text. Package allowlist excludes saved state, conversations, secrets/connections, network/capability approvals, shares and private owner identity. Publication explicitly acknowledges source/assets themselves may contain sensitive data. Assets are materialized; missing assets fail publishing. License/redistribution consent is explicit. Publication/withdrawal races must serialize and leave no half-release.

Networking: HTTPS production only; no credentials/query/fragment on instance origin; no redirects or environment proxies; exact-origin endpoints; validate all resolved IPv4/IPv6 at dial time and connect only validated public addresses; reject private/link-local/loopback/reserved/IPv4-mapped addresses. Bound DNS/dial/read times, response size after decompression, XML depth/items/strings and JSON/content sizes; reject malformed/duplicate identity/version/hash/replay. Go test-only transports may use local fixtures, never runtime env bypass.

Embeds: serve only verified immutable package content with response-header CSP sandbox allow-scripts (no allow-same-origin), connect/frame/object/form/base disabled, restrictive asset CSP and Permissions-Policy. No app bridges or third-party tracking. App HTML never interpolates raw feed HTML. Direct navigation to preview is sandboxed by response headers too. Tier compatibility must be explicit and tested.

Recipient flow: store asks for gallery origin, links to GET /bazarre/import?source=<Bazarre package URL>&sha256=<digest>&mode=copy|remix . GET only shows an authenticated review, performs no import. Explicit same-origin POST confirms exact source and digest; guarded fetch, integrity/identity validation, quota enforcement, atomic blob/metadata/provenance and replay safety. Never carry credentials across hosts. Public and private gallery auth remain unchanged. Remix opens /agent?artifact=<new id>, with no automatic prompt or model run. Update checks known feed creator identity/version/hash and offers explicit review; changed local source/widget/assets/metadata blocks overwrite (offer importing a fresh copy instead). Updates preserve saved state/history, reset all approvals/connections and check ownership, quota, stale comparison and replay. Provenance shows creator/instance/license/imported and available versions.

Integration gates: contract fixtures first; publish+RSS+index next; clone/remix/update next; adversarial revocation/race/SSRF/privacy next; desktop/mobile/dark/accessibility/copy review last. All work ticketed; review ticket plan at least three rounds with findings resolved before application code. Run review skill's parallel standards/spec agents on fixed commits, address findings and rerun affected suites. Keep both main worktrees clean; use ticketed feature/integration worktrees.

Sibling Bazarre epic: /home/maxomdal/Bazarre/.tickets/Baz-alsi.md

## Acceptance Criteria

All Exhibit child work complete and cross-repo contract/integration gates pass.

