---
id: av-isb3
status: in_progress
deps: []
links: []
created: 2026-07-15T01:34:21Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, gallery]
---
# Gallery cards: network-security badge (shield-check / globe)

The Paper redesign mockup (Exhibit — Library, file 01KWQV7Y6G82HDZJ5CKPJTSJ7Y) puts a network-security badge on every artifact card footer. Adopt it in the current gallery: each card shows, opposite the created date, the artifact's network posture derived from its allowlist. Empty allowlist -> green Phosphor shield-check icon + 'No network'. Non-empty -> amber Phosphor globe icon + 'N origins' (singular '1 origin'). This surfaces the security model (per-artifact CSP allowlist) at a glance in the library, matching spec §6.2's transparency goal.

## Design

From the Paper mockup: badge is a flex row, 5px gap, 13px icon, 12px text weight 500, line-height 16px. Colors: #12A150 (green, no network), #B45309 (amber, has origins). Icons via self-hosted Phosphor webfont already loaded on gallery pages: <i class='ph ph-shield-check'></i> / <i class='ph ph-globe'></i>. Card footer: date left, badge right (justify-content:space-between), hairline top border. Data: store.Artifact.NetworkAllowlist is already populated by list scans; len()==0 -> no-network variant.

## Acceptance Criteria

Every gallery card shows exactly one badge: 'No network' (green shield-check) when the allowlist is empty, or 'N origins' (amber globe) when not. Badge count matches len(NetworkAllowlist). Uses embedded Phosphor assets, no CDN. Existing card click-through and tag row behavior unchanged. Go tests cover both variants and the singular/plural label.

