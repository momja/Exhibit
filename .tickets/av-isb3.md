---
id: av-isb3
status: in_progress
deps: []
links: [av-p0a1, av-41se, av-hwx2]
created: 2026-07-15T01:34:21Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, gallery]
---
# Gallery cards: capability posture badge (capability row)

The gallery card footer shows the artifact's sandbox posture as a compact, neutral glyph cluster, opposite the created date. This supersedes the original green/amber network-only badge: the allowlist is transparency, not a good/bad verdict (spec 6.2), so both states are informational and the badge is neutral. Direction chosen in Paper (option B); the rationale and the two rejected options live on the "av-isb3 — Badge options (review)" artboard.

## Mockups

Paper file 01KWQV7Y6G82HDZJ5CKPJTSJ7Y:
- "av-isb3 — B · Capability row + tooltip" — the final badge states, plus a SPEC NOTES table mapping each glyph to the stored field it reads.
- "av-isb3 — Badge options (review)" — why B was chosen over the green/amber "grade" and the network-only monochrome option.

## Design

Neutral monochrome glyph cluster, self-hosted Phosphor (no CDN), ~13px icons, in the card footer opposite the date (hairline top border, justify-content: space-between):
- No grants (empty allowlist AND downloads/clipboard not approved): a single muted `ph-shield-check` + "Sandboxed" (#6A7180).
- Network: `ph-globe` + count = len(NetworkAllowlist), count in ink (#111318) weight 600.
- Downloads granted: `ph-download-simple` (shown iff downloads_approved).
- Clipboard granted: `ph-clipboard` (shown iff clipboard_approved).

Glyphs in mid-slate #2B303B; the sandboxed state muted. Retire the old #12A150 green / #B45309 amber — colour must not read as a verdict. Data: store.Artifact.NetworkAllowlist (already populated by list scans) plus the downloads_approved / clipboard_approved booleans. Origins/counts render through html/template (contextual auto-escaping — supersedes any manual escape pass).

## Acceptance Criteria

- A fully-sandboxed card (empty allowlist, both capability flags false) shows exactly one muted `ph-shield-check` "Sandboxed" mark.
- Network present shows `ph-globe` + count equal to len(NetworkAllowlist).
- `ph-download-simple` appears iff downloads_approved; `ph-clipboard` appears iff clipboard_approved.
- No green/amber anywhere; the cluster is neutral.
- Embedded Phosphor assets only, no CDN. Existing card click-through and tag-row behavior unchanged.
- Go tests cover: sandboxed, network count, and each capability glyph toggling with its flag.

Note: the hover/focus detail popover (singular "1 origin" phrasing, origin list, etc.) is a separate ticket. This ticket is the card cluster only.
