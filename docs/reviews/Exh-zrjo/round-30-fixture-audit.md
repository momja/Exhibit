# Shared fixture observation, bounded audit

Opus30 observed varying RunLog tag displays and made no persistent change by its
report. Parent read-only SQLite confirms current attached tags Health,Reading,
Test,Utilities; pre-PR backup had Health only. Timing/actor are NOT established.
No attribution to user/agent/capture is claimed.

Audited e2e/capture-ui.mjs: only explicit request mutation is scratchauth/local;
UI actions open/close/read and empty creation validation, no tag select/attach/
detach/save action. Inspected Sol30 public activity through position600: visible
Tag actions are opening EditHealth and Escape plus local title restore, with no
observed attach/detach/tag API/SQL write. This is bounded evidence, not proof that
no other actor changed fixture. No sample associations were deleted/restored by
parent, avoiding destruction of potentially intentional user/test fixture data.
Record four-tag set as current baseline for confirmation and require reviewers
readonly. If it changes again, record start/end distinction; don't claim immutable
fixture or certify unseen states. Tag content variation does not change production
runtime42c94 or assetaa7; component behavior was sampled with the visible data.

The corpus basename filters-empty is not an assertion of emptiness; Health+Reading
can now match RunLog. Route/manifest is authoritative, not filename. Actual query
and filter misses are separately verified by the40-case isolated native matrix
and current32 T3 state views. No arbitrary user-authored tile design is scored.
